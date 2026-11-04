import { defineConfig } from 'vitest/config';

export default defineConfig({
  base: './',
  // theme.test.ts reads the style sheets, which tests get empty by default.
  test: { css: true },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
    sourcemap: false,
    // Served from the binary on loopback, so chunk size does not matter.
    chunkSizeWarningLimit: 4096,
    rolldownOptions: {
      output: {
        // libavoid-js is LGPL: its glue code stays a separate file beside libavoid.wasm, so both
        // can be replaced.
        codeSplitting: { groups: [{ name: 'libavoid', test: /node_modules[\\/]libavoid-js/ }] },
      },
    },
  },
});
