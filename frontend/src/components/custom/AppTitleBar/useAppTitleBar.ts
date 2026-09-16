/**
 * 自绘标题栏的交互逻辑（渲染见 index.tsx，约定见 docs/specs/shell.spec.md）。
 *
 * 只做三件事：记住窗口是否最大化、提供窗口按钮的动作、提供标记「非客户区」用的样式。
 */
import { useEffect, useState, type CSSProperties } from "react";

import { Window } from "@wailsio/runtime";

/**
 * 非客户区的四种标记，取值见 @wailsio/runtime/dist/appregion.js 顶部注释。
 *
 * - `caption`：可拖动。双击最大化、右键系统菜单也是 Windows 原生的，不用我们写。
 * - `minimize` / `maximize` / `close`：命中后 Go 会**吞掉原生动作**，只把按下/抬起
 *   转发回前端（为了按下态样式），所以对应的 onClick 必须自己调 runtime
 *   ——就是 useAppTitleBar 返回的那三个动作。
 */
export type NonClientRegion = "caption" | "minimize" | "maximize" | "close";

/**
 * 把一个元素标成 Windows 的「非客户区」。
 *
 * ⚠️ 这个 CSS 变量是**继承**的：标到父元素上会让整棵子树都变成非客户区。
 * 所以可拖区要标在**专门的空白元素**上，别标到装按钮的那一层。
 */
export function nonClientRegion(kind: NonClientRegion): { style: CSSProperties } {
  return { style: { "--wails-non-client-region": kind } as CSSProperties };
}

/** 窗口是否最大化，以及三个窗口按钮的动作。 */
export function useAppTitleBar() {
  const [maximised, setMaximised] = useState(false);

  useEffect(() => {
    let alive = true;
    const sync = () => {
      void Window.IsMaximised().then((value) => {
        if (alive) setMaximised(value);
      });
    };
    sync();
    // 最大化/还原一定会改变视口尺寸，监听 resize 就够，不必再引窗口事件。
    window.addEventListener("resize", sync);
    return () => {
      alive = false;
      window.removeEventListener("resize", sync);
    };
  }, []);

  return {
    maximised,
    minimise: () => void Window.Minimise(),
    toggleMaximise: () => void Window.ToggleMaximise(),
    close: () => void Window.Close(),
  };
}
