import path from "path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Vitest 单独配置：不加载 wails vite 插件（jsdom 环境用不到，且避免干扰）。
export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
      // jsdom 里没有真实的 Wails 宿主，用替身替换 runtime。
      // 生成的 bindings 直接 import 它，替换后测试可精确控制后端返回值。
      "@wailsio/runtime": path.resolve(import.meta.dirname, "./src/test/mock-wails-runtime.ts"),
    },
  },
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
