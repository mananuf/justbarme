import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

// A function config (not a plain object) so loadEnv can read VITE_API_PROXY_TARGET
// below -- the backend's actual port (JBM_HTTP_ADDR) lives in the *root* .env,
// a separate file this one has no automatic way to see. Hardcoding this proxy
// target to match whatever the backend happened to use once already caused a
// real outage: every API call silently 502'd (logged in the UI as "could not
// reach the API", easy to mistake for a login failure) after the backend's
// port was changed in the root .env without updating this file to match.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_');
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || 'http://localhost:8080';

  return {
    plugins: [
      react(),
      tailwindcss(),
      VitePWA({
        registerType: 'prompt',
        includeAssets: ['icon.svg', 'icon-192.png', 'icon-512.png'],
        manifest: {
          name: 'justbarme',
          short_name: 'justbarme',
          description: 'Run your bar. Simply.',
          theme_color: '#121a14',
          background_color: '#f4f1e8',
          display: 'standalone',
          orientation: 'any',
          start_url: '/',
          scope: '/',
          icons: [
            {
              src: '/icon.svg',
              sizes: 'any',
              type: 'image/svg+xml',
              purpose: 'any',
            },
            {
              src: '/icon-192.png',
              sizes: '192x192',
              type: 'image/png',
              purpose: 'maskable',
            },
            {
              src: '/icon-512.png',
              sizes: '512x512',
              type: 'image/png',
              purpose: 'maskable',
            },
          ],
        },
        workbox: {
          navigateFallback: '/index.html',
          navigateFallbackDenylist: [/^\/api\//],
          runtimeCaching: [],
          cleanupOutdatedCaches: true,
        },
        devOptions: { enabled: false },
      }),
    ],
    server: {
      port: 5173,
      proxy: {
        // The backend's CSRF check compares the request's Origin against its
        // Host header, which only line up when they share one origin -- true
        // in production, but not for this proxy, which otherwise rewrites
        // Host to the target while the browser's real Origin stays
        // localhost:5173. Preserving the original Host keeps the two in sync
        // for local dev.
        '/api': {
          target: apiProxyTarget,
          configure: (proxy) => {
            proxy.on('proxyReq', (proxyReq, req) => {
              if (req.headers.host) proxyReq.setHeader('host', req.headers.host);
            });
          },
        },
      },
    },
    test: {
      environment: 'jsdom',
      setupFiles: './src/test/setup.ts',
      exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
      css: true,
      restoreMocks: true,
    },
  };
});
