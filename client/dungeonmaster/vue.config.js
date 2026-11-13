const path = require('path');
const { defineConfig } = require('@vue/cli-service');

// Load .env before reading vars below (Vue CLI also loads env in Service.init; this covers edge cases and documents intent).
try {
    const dotenv = require('dotenv');
    const root = __dirname;
    dotenv.config({ path: path.join(root, '.env') });
    dotenv.config({ path: path.join(root, '.env.local'), override: true });
} catch (_) {
    /* optional dependency resolution */
}

const devPort = Number(process.env.DM_FRONTEND_PORT) || 8080;

/**
 * Port the **browser** must use for WSS behind NPM. If we omit `port` in client.webSocketURL, webpack-dev-server
 * injects the dev server port (8080) — that yields broken `wss://fqdn:8080/ws`. Always set an explicit port for wss.
 */
function wssClientPort(portFromUrl) {
    const p = portFromUrl != null && portFromUrl !== '' ? Number(portFromUrl) : NaN;
    if (Number.isFinite(p) && p > 0 && p !== devPort) return p;
    const envP = Number(process.env.DM_WSS_PUBLIC_PORT);
    if (Number.isFinite(envP) && envP > 0) return envP;
    return 443;
}

/**
 * webpack-dev-server injects a small dev client that opens **one** WebSocket for live reload.
 * Only override `client.webSocketURL` when the app is served behind HTTPS / a proxy (see env.example).
 * Otherwise omit it so WDS derives host/port/path from `window.location` — avoids `auto://0.0.0.0:0/ws`
 * and reconnect storms that show up as many "ws" rows in DevTools.
 */
function devServerClientOptions() {
    const raw = (process.env.DM_DEV_WEBSOCKET_URL || '').trim();
    if (raw) {
        try {
            const u = new URL(raw);
            const protocol = u.protocol === 'wss:' || u.protocol === 'https:' ? 'wss' : 'ws';
            const pathname = u.pathname && u.pathname !== '/' ? u.pathname : '/ws';
            const spec = { protocol, hostname: u.hostname, pathname };
            if (protocol === 'wss') {
                spec.port = wssClientPort(u.port || NaN);
            } else if (u.port) {
                const n = Number(u.port);
                if (Number.isFinite(n) && n > 0) spec.port = n;
            }
            return { webSocketURL: spec };
        } catch (_) {
            /* fall through */
        }
    }
    const siteHttps = (process.env.DM_PUBLIC_URL || process.env.DM_FRONTEND_URL || '').trim();
    if (/^https:\/\//i.test(siteHttps)) {
        try {
            const u = new URL(siteHttps);
            return {
                webSocketURL: {
                    protocol: 'wss',
                    hostname: u.hostname,
                    pathname: '/ws',
                    port: wssClientPort(u.port || NaN),
                },
            };
        } catch (_) {
            /* fall through */
        }
    }
    return {};
}

/** Backend for /api during `npm run serve` (same pattern as production reverse proxy: browser → same origin, proxy → API). */
const apiProxyTarget = (process.env.DM_API_PROXY_TARGET || 'http://127.0.0.1:5001').replace(/\/$/, '');

module.exports = defineConfig({
    transpileDependencies: true,
    devServer: {
        //https: true,  // Enables HTTPS for the dev server
        port: devPort,
        hot: false,
        /** Full reload when sources change (hot module replacement off — see Vue alias note in chainWebpack). */
        liveReload: true,
        allowedHosts: 'all',
        // HTTPS reverse proxy: set DM_DEV_WEBSOCKET_URL (see env.example). Otherwise omit client.webSocketURL.
        client: {
            ...devServerClientOptions(),
        },
        proxy: {
            '/api': {
                target: apiProxyTarget,
                changeOrigin: true,
            },
        },
    },
    chainWebpack: config => {
        // Vuex is hoisted to the repo root while this app uses ./node_modules/vue.
        // Two Vue copies = store mutations do not trigger component updates until full reload.
        config.resolve.alias.set('vue', path.resolve(__dirname, 'node_modules/vue'));

        // Bake optional API base for split hosting (VUE_APP_DM_API_BASE or DM_API_BASE from .env).
        config.plugin('define').tap((definitions) => {
            const apiBase = process.env.VUE_APP_DM_API_BASE || process.env.DM_API_BASE || '';
            definitions[0]['process.env'].DM_API_BASE = JSON.stringify(apiBase);
            return definitions;
        });
    }
});
