import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { staticErrorPages } from './scripts/error-pages.ts';

export default defineConfig({
  plugins: [react(), { name: 'standalone-error-pages', generateBundle() { for (const page of staticErrorPages()) this.emitFile(page); } }],
  server: { proxy: { '/api': 'http://127.0.0.1:8082', '/health': 'http://127.0.0.1:8082' } },
  build: { sourcemap: false },
});
