import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    // Enable polling so file-system events work inside Docker volume mounts
    // (native inotify events do not propagate from the host on macOS/Windows).
    watch: {
      usePolling: true,
      interval: 300,
    },
    // Tell the browser which port to use for the HMR WebSocket. Must match
    // the host-side port exposed in docker-compose.yml.
    port: 9002,
    hmr: {
      clientPort: 9002,
    },
    proxy: {
      // Forward API calls to the Go server during development.
      "/api": {
        target: process.env.API_URL ?? "http://localhost:9001",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
