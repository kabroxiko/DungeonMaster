const { defineConfig } = require('@vue/cli-service');

const devPort = Number(process.env.DM_FRONTEND_PORT) || 8080;

module.exports = defineConfig({
    transpileDependencies: true,
    devServer: {
        //https: true,  // Enables HTTPS for the dev server
        port: devPort,
        hot: false,
        liveReload: false,
        allowedHosts: 'all',
    },
    chainWebpack: config => {
        config.module
            .rule('txt')
            .test(/\.txt$/)
            .use('raw-loader')
            .loader('raw-loader')
            .end();
    }
});
