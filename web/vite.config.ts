import { defineConfig } from 'vite';
export default defineConfig({build:{emptyOutDir:false,rolldownOptions:{input:'src/app.ts',output:{entryFileNames:'app.js',chunkFileNames:'chunks/[name]-[hash].js'}}}});
