import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// El panel se sirve como archivos estaticos desde nginx en el contenedor.
export default defineConfig({
  plugins: [react()],
  server: { host: '0.0.0.0', port: 3000 },
  build: { outDir: 'dist', sourcemap: false },
})
