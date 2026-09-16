/**
 * 检索弹窗（命令面板式，形参见 docs/specs/document.spec.md 第一节）。
 *
 * 为什么是弹窗而不是左栏里的一块：检索是**临时动作**——打开、看一眼、点一条、关掉。
 * 挤在左栏里会一直占着导航的位置，也会跟文件树的注意力打架。
 *
 * 键盘：`/` 或 `Ctrl+K` 打开、`Esc` 关闭、`↑/↓` 选、回车打开选中的那条。
 */
import { Search, X } from "lucide-react";
import { useEffect, useRef } from "react";

import type { VaultHit } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { statusStyle } from "../VaultBrowser/useVaultBrowser";

type Props = {
  open: boolean;
  query: string;
  hits: VaultHit[] | null;
  busy: boolean;
  cursor: number;
  onQuery: (q: string) => void;
  onMove: (delta: number) => void;
  onOpen: (path: string) => void;
  onClose: () => void;
};

export function SearchModal({ open, query, hits, busy, cursor, onQuery, onMove, onOpen, onClose }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);

  // 打开就聚焦：检索弹窗不该让你再点一下输入框。
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  if (!open) return null;

  const list = hits ?? [];

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/25 p-4 pt-24"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label="检索"
    >
      <div
        className="w-full max-w-xl overflow-hidden rounded-lg border border-border bg-card shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <Search className="size-4 shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => onQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") onClose();
              if (e.key === "ArrowDown") {
                e.preventDefault();
                onMove(1);
              }
              if (e.key === "ArrowUp") {
                e.preventDefault();
                onMove(-1);
              }
              if (e.key === "Enter" && list[cursor]) onOpen(list[cursor].path);
            }}
            placeholder="检索标题与正文…"
            className="min-w-0 flex-1 bg-transparent text-sm outline-none"
          />
          {busy && <span className="shrink-0 text-[11px] text-muted-foreground">检索中…</span>}
          <button type="button" aria-label="关闭检索" onClick={onClose} className="shrink-0 rounded p-0.5 hover:bg-secondary">
            <X className="size-4" />
          </button>
        </div>

        <div className="max-h-[60vh] overflow-auto py-1">
          {hits === null && <div className="px-3 py-2 text-xs text-muted-foreground">输入关键词（中文直接打）</div>}
          {hits !== null && list.length === 0 && <div className="px-3 py-2 text-xs text-muted-foreground">没有命中</div>}
          {list.map((h, i) => {
            const st = statusStyle(h.status);
            return (
              <button
                key={h.path}
                type="button"
                onMouseEnter={() => onMove(i - cursor)}
                onClick={() => onOpen(h.path)}
                className={"block w-full px-3 py-2 text-left " + (i === cursor ? "bg-secondary" : "hover:bg-secondary/60")}
              >
                <span className="flex items-center gap-2">
                  <span className={`shrink-0 rounded px-1 text-[10px] leading-4 ${st.className}`}>{st.label}</span>
                  <span className="truncate text-sm">{h.title}</span>
                  {h.titleMatch && <span className="shrink-0 text-[10px] text-muted-foreground">标题命中</span>}
                </span>
                <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{h.snippet}</span>
                <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{h.path}</span>
              </button>
            );
          })}
        </div>

        <div className="flex items-center gap-3 border-t border-border px-3 py-1.5 text-[11px] text-muted-foreground">
          <span>↑↓ 选择</span>
          <span>回车打开</span>
          <span>Esc 关闭</span>
        </div>
      </div>
    </div>
  );
}
