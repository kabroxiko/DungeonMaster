// Temporary shim to avoid Node deprecation warnings from old dependencies that call util._extend.
// Place the shim at the very top so any subsequent require() sees the patched util.
try {
  const util = require('util');
  if (util && typeof util._extend === 'function') {
    // Overwrite deprecated util._extend with Object.assign to prevent DeprecationWarning (DEP0060)
    util._extend = Object.assign;
  }
} catch (e) {
  // ignore if util cannot be required for any reason
}

// Load environment early (after the shim)
require('dotenv').config();

const path = require('path');
const fs = require('fs');
const mongoose = require('mongoose');
const express = require('express');
const cors = require('cors');
const bodyParser = require('body-parser');

const app = express();

const gameSessionRouter = require('./routes/gameSession');
const gameStateRoutes = require('./routes/gameState');

// CORS: DM_FRONTEND_URL is the canonical UI origin; also allow same port on localhost/127.0.0.1 when the URL has an explicit port (LAN dev vs localhost browser).
const FRONTEND_URL = (process.env.DM_FRONTEND_URL || 'http://localhost:8080').replace(/\/$/, '');

function buildAllowedOrigins() {
  const list = new Set([FRONTEND_URL]);
  (process.env.DM_CORS_ORIGINS || '')
    .split(',')
    .map((s) => s.trim().replace(/\/$/, ''))
    .filter(Boolean)
    .forEach((o) => list.add(o));
  try {
    const href = /^https?:\/\//i.test(FRONTEND_URL) ? FRONTEND_URL : `http://${FRONTEND_URL}`;
    const u = new URL(href);
    if (u.port) {
      list.add(`http://localhost:${u.port}`);
      list.add(`http://127.0.0.1:${u.port}`);
    }
  } catch (e) {
    /* ignore */
  }
  return [...list];
}

const allowedOrigins = buildAllowedOrigins();

app.use(bodyParser.json());
app.use(bodyParser.urlencoded({ extended: true }));

app.use((req, res, next) => {
  const origin = req.headers.origin;
  // In development, echo request origin so any dev host works.
  if (process.env.NODE_ENV === 'development' || process.env.NODE_ENV === 'dev') {
    res.header('Access-Control-Allow-Origin', origin || '*');
  } else {
    if (!origin) {
      res.header('Access-Control-Allow-Origin', '*');
    } else if (allowedOrigins.includes(origin)) {
      res.header('Access-Control-Allow-Origin', origin);
    } else {
      res.header('Access-Control-Allow-Origin', FRONTEND_URL);
    }
  }
  res.header('Access-Control-Allow-Methods', 'GET,POST,PUT,DELETE,OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  res.header('Access-Control-Allow-Credentials', 'true');
  if (req.method === 'OPTIONS') {
    return res.sendStatus(204);
  }
  next();
});

app.use('/api/game-session', gameSessionRouter);
app.use('/api/game-state', gameStateRoutes);

// Session summaries run only after save (gameState → triggerSummaryForGame). No periodic batch job.

// Connection to MongoDB
mongoose.connect(process.env.DM_MONGODB_URI, {
    useNewUrlParser: true,
    useUnifiedTopology: true,
})
.then(() => console.log('Connected to MongoDB'))
.catch((error) => console.error('Error connecting to MongoDB:', error));

// Optional: serve built Vue app from this process so production can use one port (API + static SPA).
// Build client first: cd client/dungeonmaster && npm run build
// Then set DM_SERVE_SPA_DIST=1 (or an absolute path to dist), or use a reverse proxy instead.
function resolveSpaDistDir() {
  const v = process.env.DM_SERVE_SPA_DIST;
  if (!v || v === '0' || v === 'false') return null;
  if (v === '1' || v === 'true') {
    return path.join(__dirname, '..', 'client', 'dungeonmaster', 'dist');
  }
  return path.isAbsolute(v) ? v : path.join(__dirname, '..', v);
}

const spaDistDir = resolveSpaDistDir();
if (spaDistDir) {
  if (fs.existsSync(spaDistDir)) {
    app.use(express.static(spaDistDir));
    app.get('*', (req, res, next) => {
      if (req.path.startsWith('/api')) return next();
      if (req.method !== 'GET' && req.method !== 'HEAD') return next();
      res.sendFile(path.join(spaDistDir, 'index.html'), (err) => next(err));
    });
    console.log(`Serving SPA static files from ${spaDistDir}`);
  } else {
    console.warn(`DM_SERVE_SPA_DIST is set but dist folder not found: ${spaDistDir}`);
  }
}

const PORT = process.env.PORT || 5001;

app.listen(PORT, () => {
    console.log(`Server is running on port ${PORT}`);
    console.log(`FRONTEND_URL=${FRONTEND_URL}  NODE_ENV=${process.env.NODE_ENV || 'undefined'}`);
    if (process.env.NODE_ENV !== 'development' && process.env.NODE_ENV !== 'dev') {
        console.log('CORS allowed origins:', allowedOrigins.join(', '));
    }
});
