import path from "path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Vitest 单独配置：不加载 wails vite 插件（jsdom 环境用不到，且避免干扰）。
export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
