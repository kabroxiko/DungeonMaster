package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gamesession"
	"github.com/deckofdmthings/gmai/internal/httpserver"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/realtime"
	"github.com/deckofdmthings/gmai/internal/store"
)

func databaseNameFromMongoURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Path == "" || u.Path == "/" {
		return "dungeonmaster"
	}
	name := strings.TrimPrefix(u.Path, "/")
	if idx := strings.Index(name, "?"); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		return "dungeonmaster"
	}
	return name
}

// repoRoot finds the repo root whether the process was started from the repo root or from go/.
func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if fi, err := os.Stat(filepath.Join(wd, "client", "dungeonmaster")); err == nil && fi.IsDir() {
		return wd
	}
	up := filepath.Join(wd, "..")
	if fi, err := os.Stat(filepath.Join(up, "client", "dungeonmaster")); err == nil && fi.IsDir() {
		abs, err := filepath.Abs(up)
		if err != nil {
			return up
		}
		return abs
	}
	return wd
}

// startVueDevServer runs `npm run serve` in client/dungeonmaster (Node). Parent process is gmai-server.
func startVueDevServer(root string) *exec.Cmd {
	clientDir := filepath.Join(root, "client", "dungeonmaster")
	if fi, err := os.Stat(clientDir); err != nil || !fi.IsDir() {
		log.Printf("Vue dev: skip — not found: %s", clientDir)
		return nil
	}
	// A dev server left behind by an earlier run keeps the port. The new one then silently moves to the next free
	// port while the browser keeps talking to the orphan, which still proxies /api to this process — the app looks
	// alive and no frontend change since that run ever reaches the browser. Refuse and name the port instead.
	port := frontendPort()
	if portTaken(port) {
		log.Printf("Vue dev: NOT started — port %s is already in use, probably a dev server left by an earlier run "+
			"(lsof -nP -iTCP:%s -sTCP:LISTEN). Stop it, or set DM_FRONTEND_PORT to a free port.", port, port)
		return nil
	}
	cmd := exec.Command("npm", "run", "serve")
	cmd.Dir = clientDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	// Own process group: `npm run serve` is a shell that execs `vue-cli-service serve` in turn, so killing npm alone
	// leaves the node grandchild listening on the port - the dev server that outlives `npm start`.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Printf("warning: Vue dev server (npm run serve) failed to start: %v", err)
		return nil
	}
	log.Printf("Vue dev server started (child of gmai-server) pid=%d dir=%s port=%s", cmd.Process.Pid, clientDir, port)
	return cmd
}

// frontendPort is the port vue.config.js binds for the dev server (DM_FRONTEND_PORT, 8080 by default).
func frontendPort() string {
	if p := strings.TrimSpace(os.Getenv("DM_FRONTEND_PORT")); p != "" {
		return p
	}
	return "8080"
}

// portTaken reports whether something already listens on the loopback address of that port.
func portTaken(port string) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

// setupLogging mirrors the process log to a file and says where it went. In development the Vue dev server shares this
// process's terminal and floods it on every rebuild, so the backend's own lines - including the DM_DEBUG_LLM dumps -
// are easy to lose. Returns a function that closes the file.
func setupLogging(cfg *config.Config) func() {
	path := cfg.LogFile
	if path == "" && config.TruthyEnv("DM_SPAWN_VUE_DEV") {
		path = filepath.Join("tmp", "gmai-server.log")
	}
	if path == "" {
		return func() {}
	}
	// `npm start` runs the module with `-C go`, so the process cwd is go/: resolve a relative DM_LOG_FILE against the
	// repository root, which is where a developer expects to find it.
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot(), path)
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("warning: cannot create log directory %s: %v", dir, err)
			return func() {}
		}
	}
	// Truncated per run: this is the log of the process that is running now, not an append-only history.
	f, err := os.Create(path)
	if err != nil {
		log.Printf("warning: cannot write log file %s: %v", path, err)
		return func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.Printf("logging to %s", path)
	return func() { _ = f.Close() }
}

// loadDotEnv loads the same variables as the former Node app (dotenv).
// Tries repo root `.env`, then `../.env` when cwd is `go/`.
func loadDotEnv() {
	paths := []string{".env", "../.env"}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		if err := godotenv.Load(p); err != nil {
			log.Printf("warning: could not load %s: %v", p, err)
			continue
		}
		log.Printf("loaded environment from %s", p)
		return
	}
}

func main() {
	loadDotEnv()
	cfg := config.Load()
	closeLog := setupLogging(cfg)
	defer closeLog()
	if cfg.MongoURI == "" {
		log.Fatal("DM_MONGODB_URI is required (set in the environment or copy env.example to .env in the repository root)")
	}
	// Say which AI backends this process will use, and refuse to start when a selector is wrong: a misconfigured
	// provider should not look like a broken model on a player's first request.
	playProvider, playURL, playModel, err := llm.BackendFor(cfg, false)
	if err != nil {
		log.Fatalf("AI provider (light): %v", err)
	}
	log.Printf("AI provider (light): %s at %s, model %s (DM_AI_LIGHT_*)", playProvider, playURL, playModel)
	heavyProvider, heavyURL, heavyModel, err := llm.BackendFor(cfg, true)
	if err != nil {
		log.Fatalf("AI provider (heavy): %v", err)
	}
	if cfg.AIHeavyProvider != "" || cfg.AIHeavyModel != "" {
		log.Printf("AI provider (heavy): %s at %s, model %s (DM_AI_HEAVY_*)", heavyProvider, heavyURL, heavyModel)
	} else {
		log.Printf("AI provider (heavy): same as light, model %s (DM_AI_HEAVY_PROVIDER is unset)", heavyModel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := store.Dial(ctx, cfg.MongoURI)
	if err != nil {
		// The URI holds the database password and the driver echoes a malformed one back, so the message is redacted
		// before it is printed.
		log.Fatalf("mongo connect: %s", store.RedactSecrets(err.Error(), cfg.MongoURI))
	}
	// mongo.Connect is lazy: it validates the URI and returns, so without this ping a server whose database is
	// unreachable starts cleanly and then answers every account lookup with a 500. Fail at boot with the real reason
	// instead, the same way a bad AI provider selector does.
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("mongo ping: %s", store.RedactSecrets(err.Error(), cfg.MongoURI))
	}
	defer func() {
		dcCtx, dcCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dcCancel()
		if err := client.Disconnect(dcCtx); err != nil {
			log.Printf("mongo disconnect: %v", err)
		}
	}()

	dbName := databaseNameFromMongoURI(cfg.MongoURI)
	db := client.Database(dbName)
	hub := realtime.NewHub()
	deps := &gamesession.Deps{Cfg: cfg, Coll: store.GameStates(db), Hub: hub}
	srv := &httpserver.Server{Cfg: cfg, DB: db, Hub: hub, GS: deps}

	// A restart kills the background work a table is waiting on (a start, a world step). Put those tables back before
	// they are served again, so nobody waits on a process that no longer exists.
	go func() {
		rctx, rcancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer rcancel()
		gamesession.RecoverInterruptedStarts(rctx, deps, hub)
	}()

	addr := cfg.BindHost
	if addr == "" {
		addr = "0.0.0.0"
	}
	// Bind before anything else is started. A second instance used to spawn the Vue dev server first and then die on the
	// port, leaving a frontend serving on 8080 with no API behind it - the "app still running in the background" that a
	// stopped npm start left behind.
	ln, err := net.Listen("tcp", net.JoinHostPort(addr, cfg.Port))
	if err != nil {
		log.Fatalf("listen on %s:%s: %v (another gmai-server may already be running)", addr, cfg.Port, err)
	}
	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}

	var vueDev *exec.Cmd
	if config.TruthyEnv("DM_SPAWN_VUE_DEV") {
		vueDev = startVueDevServer(repoRoot())
	}
	// The dev server is this process's child: it goes when this process goes, however this process ends.
	var stopVueOnce sync.Once
	stopVueDev := func() {
		stopVueOnce.Do(func() {
			if vueDev != nil && vueDev.Process != nil {
				log.Printf("stopping Vue dev server (pid %d)", vueDev.Process.Pid)
				// The negative pid signals the whole group, so npm's children (sh, node) go with it.
				if err := syscall.Kill(-vueDev.Process.Pid, syscall.SIGTERM); err != nil {
					_ = vueDev.Process.Kill()
				}
			}
		})
	}
	defer stopVueDev()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("gmai-server listening on http://%s", ln.Addr())
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case err := <-serveErr:
		log.Printf("listen: %v", err)
	}
	// Shutdown waits for active handlers: SSE streams, WebSockets, and in-flight /api calls (e.g. LLM) up to the grace cap.
	// Closing WS first avoids waiting on those handlers; SSE closes when the server closes the connection.
	stopVueDev()
	hub.CloseWebSockets()
	shGrace := time.Duration(cfg.HTTPShutdownGraceSeconds) * time.Second
	shCtx, shCancel := context.WithTimeout(context.Background(), shGrace)
	defer shCancel()
	log.Printf("HTTP shutdown (grace %v); long LLM requests may run until this timeout", shGrace)
	if err := httpSrv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}
