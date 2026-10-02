import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  resolve: { conditions: ['browser'] },
  clearScreen: false,
  server: { host: '127.0.0.1', strictPort: true, port: 5173 },
  build: { target: 'es2022', sourcemap: false },
  test: { environment: 'jsdom', include: ['tests/**/*.test.ts'], restoreMocks: true },
});
