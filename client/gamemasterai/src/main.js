import { createApp } from 'vue';
import App from './App.vue';
import router from './router';
import store from './store';
import axios from 'axios';

const app = createApp(App);

// Use environment-provided API base URL when available, otherwise use relative paths.
axios.defaults.baseURL = process.env.GM_API_BASE ? process.env.GM_API_BASE.replace(/\/$/, '') : '';

// Add request interceptor
axios.interceptors.request.use(
    function (config) {
        console.log('Request:', config);
        return config;
    },
    function (error) {
        console.log('Request Error:', error);
        return Promise.reject(error);
    }
);

// Add response interceptor
axios.interceptors.response.use(
    function (response) {
        console.log('Response:', response);
        return response;
    },
    function (error) {
        console.log('Response Error:', error);
        return Promise.reject(error);
    }
);

app.use(router);
app.use(store);
app.config.globalProperties.$http = axios;
app.mount('#app');