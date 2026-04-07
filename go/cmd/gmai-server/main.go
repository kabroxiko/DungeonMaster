package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gamesession"
	"github.com/deckofdmthings/gmai/internal/httpserver"
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
	cmd := exec.Command("npm", "run", "serve")
	cmd.Dir = clientDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		log.Printf("warning: Vue dev server (npm run serve) failed to start: %v", err)
		return nil
	}
	log.Printf("Vue dev server started (child of gmai-server) pid=%d dir=%s", cmd.Process.Pid, clientDir)
	return cmd
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
	if cfg.MongoURI == "" {
		log.Fatal("DM_MONGODB_URI is required (set in the environment or copy env.example to .env in the repository root)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := store.Dial(ctx, cfg.MongoURI)
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
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

	var vueDev *exec.Cmd
	if config.TruthyEnv("DM_SPAWN_VUE_DEV") {
		vueDev = startVueDevServer(repoRoot())
	}

	addr := cfg.BindHost
	if addr == "" {
		addr = "0.0.0.0"
	}
	ln := fmt.Sprintf("%s:%s", addr, cfg.Port)
	httpSrv := &http.Server{
		Addr:              ln,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("gmai-server listening on http://%s", ln)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	// Shutdown waits for active handlers: SSE streams, WebSockets, and in-flight /api calls (e.g. LLM) up to the grace cap.
	// Closing WS first avoids waiting on those handlers; SSE closes when the server closes the connection.
	if vueDev != nil && vueDev.Process != nil {
		log.Printf("stopping Vue dev server (pid %d)", vueDev.Process.Pid)
		_ = vueDev.Process.Kill()
	}
	hub.CloseWebSockets()
	shGrace := time.Duration(cfg.HTTPShutdownGraceSeconds) * time.Second
	shCtx, shCancel := context.WithTimeout(context.Background(), shGrace)
	defer shCancel()
	log.Printf("HTTP shutdown (grace %v); long LLM requests may run until this timeout", shGrace)
	if err := httpSrv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}
