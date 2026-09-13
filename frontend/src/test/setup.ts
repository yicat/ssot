import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

if (typeof window !== "undefined" && typeof window.matchMedia !== "function") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }),
  });
}

// jsdom 缺 PointerEvent/pointer capture/scrollIntoView：radix Select 依赖，测试环境补齐。
if (typeof window !== "undefined") {
  if (!window.PointerEvent) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    window.PointerEvent = MouseEvent as any;
  }
  const proto = window.HTMLElement?.prototype;
  if (proto) {
    if (!proto.setPointerCapture) proto.setPointerCapture = () => {};
    if (!proto.releasePointerCapture) proto.releasePointerCapture = () => {};
    if (!proto.hasPointerCapture) proto.hasPointerCapture = () => false;
    if (!proto.scrollIntoView) proto.scrollIntoView = () => {};
  }
}

afterEach(() => {
  cleanup();
});
