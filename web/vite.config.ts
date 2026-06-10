import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev the React app is served by Vite (port 5173) and API/WebSocket calls are
// proxied to the Go daemon (port 8765). In production the daemon serves the
// built assets directly, so no proxy is involved.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1024,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8765",
        changeOrigin: false,
        ws: true,
      },
    },
  },
});
