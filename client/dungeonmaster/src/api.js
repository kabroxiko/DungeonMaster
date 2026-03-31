// api.js
import axios from 'axios';

const api = axios.create({
    baseURL: process.env.DM_API_BASE
        ? `${process.env.DM_API_BASE.replace(/\/$/, '')}/api`
        : typeof window !== 'undefined'
          ? `${window.location.protocol}//${window.location.hostname}:5001/api`
          : '/api',
    headers: {
        'Content-Type': 'application/json',
    },
});

export default api;
