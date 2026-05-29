import { defineConfig, loadEnv } from "vite";
import { resolve } from "node:path";

const env = loadEnv("", resolve(__dirname, "../../.."), "");
const coreProxy = env.CORE_URL || env.VITE_CORE_URL || "http://127.0.0.1:8090";

export default defineConfig({
  root: resolve(__dirname),
  base: "/ui/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    manifest: true,
    rollupOptions: {
      input: resolve(__dirname, "index.html"),
    },
  },
  server: {
    host: true,
    proxy: {
      "/healthz": coreProxy,
      "/v1": coreProxy,
    },
  },
});
