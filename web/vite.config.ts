import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";
import { fileURLToPath } from "url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-vite-plugin";
import svgr from "vite-plugin-svgr";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// https://vite.dev/config/
export default defineConfig(({ command, mode }) => {
  const env = loadEnv(mode, __dirname, "");

  return {
    plugins: [
      react(),
      svgr({
        svgrOptions: {
          exportType: "default",
        },
      }),
      tanstackRouter({
        routesDirectory: "./src/routes",
        generatedRouteTree: "./src/routeTree.gen.ts",
        routeFileIgnorePattern: "(page\\.tsx$|\\.test\\.(ts|tsx)$)",
      }),
      tailwindcss(),
    ],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
        "@shared": path.resolve(__dirname, "../shared"),
      },
    },
    json: {
      stringify: false, // Import JSON as parsed objects, not stringified
    },
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
          target: env.API_URL ?? "http://localhost:9001",
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: "dist",
      emptyOutDir: true,
      target: "es2022",
      minify: command === "build" ? "terser" : "esbuild",
      terserOptions:
        command === "build"
          ? ({
              compress: {
                // Remove console.log in production builds only
                // Keeps console.info, console.warn, console.error
                pure_funcs: ["console.log"],
              },
            } as Record<string, unknown>)
          : undefined,
    },
  };
});