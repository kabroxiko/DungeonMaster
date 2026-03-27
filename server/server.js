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

const mongoose = require('mongoose');
const express = require('express');
const cors = require('cors');
const bodyParser = require('body-parser');

const app = express();

const gameSessionRouter = require('./routes/gameSession');
const gameStateRoutes = require('./routes/gameState'); 

// CORS configuration: allow development origins (use GM_FRONTEND_URL env or default)
const FRONTEND_URL = (process.env.GM_FRONTEND_URL || 'http://localhost:8082').replace(/\/$/, '');
const allowedOrigins = [FRONTEND_URL];

app.use(bodyParser.json());
app.use(bodyParser.urlencoded({ extended: true }));

app.use((req, res, next) => {
  const origin = req.headers.origin;
  // In development, be permissive to simplify LAN testing.
  if (process.env.NODE_ENV === 'development' || process.env.NODE_ENV === 'dev') {
    res.header('Access-Control-Allow-Origin', origin || '*');
  } else {
    if (!origin) {
      // non-browser request (curl, internal) - allow
      res.header('Access-Control-Allow-Origin', '*');
    } else if (allowedOrigins.includes(origin)) {
      res.header('Access-Control-Allow-Origin', origin);
    } else {
      // default to FRONTEND_URL if not an allowed origin
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

// Connection to MongoDB
mongoose.connect(process.env.GM_MONGODB_URI, {
    useNewUrlParser: true,
    useUnifiedTopology: true,
})
.then(() => console.log('Connected to MongoDB'))
.catch((error) => console.error('Error connecting to MongoDB:', error));

// Removed testUser endpoint
// app.get('/api/test', (req, res) => {
//     const testUser = { _id: '123456', email: 'test@example.com' };
//     res.json({ user: testUser });
// });

const PORT = process.env.PORT || 5001;

app.listen(PORT, () => {
    console.log(`Server is running on port ${PORT}`);
    console.log(`FRONTEND_URL=${FRONTEND_URL}  NODE_ENV=${process.env.NODE_ENV || 'undefined'}`);
});
