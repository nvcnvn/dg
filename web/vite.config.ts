import { defineConfig } from 'vite';

export default defineConfig({
  base: './',
  resolve: {
    alias: [
      {
        find: /^\.\/providers\/WorkerProvider\.mjs$/,
        replacement: decodeURIComponent(new URL('./src/canvas/worker-provider-stub.ts', import.meta.url).pathname),
      },
    ],
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
    sourcemap: false,
  },
});
