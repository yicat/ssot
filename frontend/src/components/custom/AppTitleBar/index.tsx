/**
 * 自绘标题栏：整窗唯一的一条 bar。
 *
 * 原生标题栏已由 main.go 的 Frameless 去掉，界面上这一条就是那条 titlebar
 * （约定见 docs/specs/shell.spec.md）：
 *   - 应用名在这里出现，整窗只出现一次
 *   - 可拖区与窗口按钮靠 Windows 非客户区机制（useAppTitleBar 里的 nonClientRegion）
 *   - 数据与切换动作由调用方传进来，这里只渲染，不碰 binding
 */
import { Copy, Minus, Square, X } from "lucide-react";

import type { ProjectRef, SessionState } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { Button } from "../../ui/button";
import { nonClientRegion, useAppTitleBar } from "./useAppTitleBar";

type Props = {
  /** 当前会话；还没有项目时是 null。 */
  session: SessionState | null;
  /** 可切换的项目。 */
  projects: ProjectRef[];
  /** 切换项目。失败由调用方处理（会话失败时不变，界面不会塌）。 */
  onOpenProject: (dir: string) => void;
};

/**
 * Windows 原生标题栏按钮的样子：贴边、通高、没有圆角。
 *
 * ⚠️ 关闭键**不要**在这个串后面再拼 `hover:bg-red-*`：两份同优先级的 hover 工具类
 * 谁生效只取决于它们在生成 CSS 里的先后（跟写 class 的顺序无关），叠着写会随机翻车
 * ——本文件第一版就是这么写错的，结果关闭键 hover 出来的是灰底。所以这里给两套完整串。
 *
 * `cursor-default` 是**故意的**：全局给按钮补了手型（style.css 的 @layer base），
 * 但 Windows 原生的窗口按钮是箭头，所以这里显式盖回箭头。见 docs/specs/shell.spec.md。
 */
const windowButtonBase =
  "inline-flex w-11 cursor-default items-center justify-center transition-colors hover:bg-secondary hover:text-foreground";
const windowButtonClose =
  "inline-flex w-11 cursor-default items-center justify-center transition-colors hover:bg-red-600 hover:text-white";

export function AppTitleBar({ session, projects, onOpenProject }: Props) {
  const { maximised, minimise, toggleMaximise, close } = useAppTitleBar();

  return (
    <header className="flex select-none items-stretch border-b border-border bg-card">
      <div className="flex min-w-0 flex-1 items-center gap-x-4 py-1 pl-4">
        {/* 可拖：应用名 */}
        <div {...nonClientRegion("caption")} className="text-sm font-semibold whitespace-nowrap">
          SSOT 工作台
        </div>

        <div className="truncate text-xs text-muted-foreground">
          当前项目：
          {session ? `${session.name}（${session.dir}）` : "（未打开）"}
        </div>

        <div className="flex flex-wrap gap-1.5">
          {projects.map((p) => {
            // 当前项目只做「安静标记」：标题栏里不放实心重色块（本主题 --primary 是近黑），
            // 何况左边那行文字已经在说同一件事了。见 docs/specs/shell.spec.md。
            const current = p.current || session?.dir === p.dir;
            return (
              <Button
                key={p.dir}
                size="xs"
                variant={current ? "secondary" : "ghost"}
                // 传 className 覆盖是安全的：Button 内部走 cn()，在类名层就把 font-medium 换掉
                className={current ? "font-semibold" : undefined}
                onClick={() => onOpenProject(p.dir)}
              >
                {p.name}
              </Button>
            );
          })}
        </div>

        {/*
          可拖：中间这段空白。只有标了变量的元素才是非客户区，
          所以左边那排项目按钮仍是普通内容区，照常收点击。
        */}
        <div {...nonClientRegion("caption")} className="min-w-4 flex-1 self-stretch" />
      </div>

      {/* 窗口按钮：动作在 onClick 里自己调 runtime（原生动作已被 Go 吞掉） */}
      <div className="flex items-stretch">
        <button type="button" aria-label="最小化" onClick={minimise} {...nonClientRegion("minimize")} className={windowButtonBase}>
          <Minus className="size-4" />
        </button>
        <button
          type="button"
          aria-label={maximised ? "还原" : "最大化"}
          onClick={toggleMaximise}
          {...nonClientRegion("maximize")}
          className={windowButtonBase}
        >
          {maximised ? <Copy className="size-4 -scale-x-100" /> : <Square className="size-3.5" />}
        </button>
        <button type="button" aria-label="关闭" onClick={close} {...nonClientRegion("close")} className={windowButtonClose}>
          <X className="size-4" />
        </button>
      </div>
    </header>
  );
}
