import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

const env = loadEnv("", resolve(__dirname, "../../.."), "");
const coreProxy = env.CORE_URL || env.VITE_CORE_URL || "http://127.0.0.1:8090";

export default defineConfig({
  root: resolve(__dirname),
  base: "/ui/",
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    manifest: true,
    rollupOptions: {
      input: resolve(__dirname, "index.html"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: [resolve(__dirname, "src/test/setup.ts")],
  },
  server: {
    host: true,
    proxy: {
      "/healthz": coreProxy,
      "/v1": { target: coreProxy, ws: true },
    },
  },
});
