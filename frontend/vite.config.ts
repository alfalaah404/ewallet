import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

// Backend origin for the /api proxy. Defaults to :8080 but is overridable via
// API_PROXY_TARGET so dev/e2e can point at a backend on a different port (e.g.
// :8090 when :8080 is taken by another service like a reverse proxy).
const API_TARGET = process.env.API_PROXY_TARGET || 'http://localhost:8080';

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  server: {
    host: true,
    port: 5173,
    // Dev proxy: forward /api/* to the Go backend so the SPA can call same-origin.
    proxy: {
      '/api': { target: API_TARGET, changeOrigin: true },
    },
  },
  preview: {
    host: true,
    port: 4173,
    // Preview proxy mirrors dev so Playwright e2e can hit the live Go backend same-origin.
    proxy: {
      '/api': { target: API_TARGET, changeOrigin: true },
    },
  },
});
