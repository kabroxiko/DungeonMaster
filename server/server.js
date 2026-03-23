require('dotenv').config();

const mongoose = require('mongoose');
const express = require('express');
const cors = require('cors');
const bodyParser = require('body-parser');

const app = express();

const gameSessionRouter = require('./routes/gameSession');
const gameStateRoutes = require('./routes/gameState'); 

// CORS configuration: allow development origins (add more if needed)
const allowedOrigins = [
  'http://localhost:8080', // original client
  'http://localhost:8082', // other dev instance seen in console
];

app.use(bodyParser.json());
app.use(bodyParser.urlencoded({ extended: true }));

app.use((req, res, next) => {
  const origin = req.headers.origin;
  if (!origin) {
    // non-browser request (curl, server-side) - allow
    res.header('Access-Control-Allow-Origin', '*');
  } else if (allowedOrigins.includes(origin)) {
    res.header('Access-Control-Allow-Origin', origin);
  } else {
    // allow other origins if you prefer by uncommenting next line:
    // res.header('Access-Control-Allow-Origin', '*');
    res.header('Access-Control-Allow-Origin', 'http://localhost:8080');
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
mongoose.connect(process.env.MONGODB_URI, {
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
});
