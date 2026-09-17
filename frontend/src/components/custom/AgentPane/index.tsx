/**
 * Agent 面板：主体区的第三个模式（文档 / 数据表 / **Agent**）。
 *
 * 为什么长在主体区而不是常驻侧栏：文档是主角（docs/specs/document.spec.md）——
 * 聊天切过去才占屏幕，切回来文档还在原处；常驻会把正文挤窄。
 *
 * 界面口径：
 *  - 工具调用**单独一行**、灰色小字：它是过程，不是回答（回答才是要读的东西）。
 *  - 权限提示是**弹窗**、必须人点：agent 不能自己批准自己。
 *  - 模型选择放在这里（会话级），不是配置页——换一次会话就换一次选择。
 */
import { Bot, CircleStop, ListTree, Play, Send, Settings2, ShieldAlert, Wrench } from "lucide-react";
import { useEffect } from "react";

import { useAgent } from "./useAgent";

type Props = {
  /** 打开设置（配置页在 VaultBrowser 那边统一管）。 */
  onOpenSettings: () => void;
};

export function AgentPane({ onOpenSettings }: Props) {
  const a = useAgent();

  // 打开面板就把状态拉一次：后端可能早就起着，切走再切回来不该显示成「没起」。
  //
  // ⚠️ 这里**不能**加 `if (a.vault)` 这类条件：`vault` 本身就是从 Status 拿的，
  // 首次挂载时它还是空字符串，条件不成立 → 根本不查 → 界面一直显示「后端未启动」，
  // 而后端其实在跑（踩过：切走再切回来、或页面 reload 之后就是这样）。
  useEffect(() => {
    void a.refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const canSend = a.running && !a.busy && a.draft.trim().length > 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* 顶部：后端状态 + 模型 + 会话 */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2 text-xs">
        <Bot className="size-3.5 text-muted-foreground" />
        <span className="text-muted-foreground">
          {a.running ? `${a.agent || "后端"}${a.version ? " " + a.version : ""}` : "后端未启动"}
        </span>
        {a.sessionId && <span className="truncate text-[11px] text-muted-foreground/70">会话 {a.sessionId.slice(0, 8)}</span>}

        <div className="ml-auto flex items-center gap-2">
          {a.models.length > 0 && (
            <select
              value={a.model}
              onChange={(e) => void a.setModel(e.target.value)}
              className="max-w-56 rounded border border-border bg-transparent px-1.5 py-0.5 text-xs"
              title="这一次会话用哪个模型（来自后端的会话配置选项）"
            >
              {a.models.map((m) => (
                <option key={m.value} value={m.value}>
                  {m.group ? `${m.group} · ${m.name}` : m.name}
                </option>
              ))}
            </select>
          )}
          <button
            type="button"
            onClick={() => void a.loadSessions()}
            disabled={!a.running}
            className="inline-flex items-center gap-1 rounded px-2 py-0.5 hover:bg-secondary disabled:opacity-40"
            title="这个 vault 的历史会话（不会串到别的项目）"
          >
            <ListTree className="size-3" />
            会话
          </button>
          <button
            type="button"
            onClick={onOpenSettings}
            className="inline-flex items-center gap-1 rounded px-2 py-0.5 hover:bg-secondary"
            title="配置（项目 / Agent 后端 / 外观）"
          >
            <Settings2 className="size-3" />
            配置
          </button>
        </div>
      </div>

      {/* 会话列表（点「会话」才出来） */}
      {a.sessionsOpen && (
        <div className="border-b border-border bg-secondary/40 px-4 py-2 text-xs">
          {a.sessions.length === 0 ? (
            <span className="text-muted-foreground">这个 vault 还没有历史会话。</span>
          ) : (
            <ul className="flex flex-col gap-1">
              {a.sessions.map((s) => (
                <li key={s.id} className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void a.resume(s.id)}
                    disabled={s.current}
                    className="truncate rounded px-1 py-0.5 text-left hover:bg-secondary disabled:opacity-60"
                    title={s.cwd}
                  >
                    {s.title || s.id}
                  </button>
                  {s.current && <span className="shrink-0 text-[10px] text-muted-foreground">当前</span>}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {/* 消息流 */}
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        {a.items.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <Bot className="size-5" />
            <div>{a.running ? "说点什么吧。" : "还没有起后端。"}</div>
            {!a.running && (
              <button
                type="button"
                onClick={() => void a.start()}
                className="mt-1 inline-flex items-center gap-1.5 rounded border border-border px-3 py-1 hover:bg-secondary"
              >
                <Play className="size-3.5" />
                启动后端
              </button>
            )}
          </div>
        )}

        <div className="flex flex-col gap-2">
          {a.items.map((it) => {
            if (it.kind === "user") {
              return (
                <div key={it.id} className="self-end max-w-[85%] rounded bg-secondary px-3 py-1.5 text-sm whitespace-pre-wrap">
                  {it.text}
                </div>
              );
            }
            if (it.kind === "assistant") {
              return (
                <div key={it.id} className="max-w-[92%] text-sm whitespace-pre-wrap">
                  {it.text}
                </div>
              );
            }
            if (it.kind === "thought") {
              return (
                <div key={it.id} className="max-w-[92%] border-l-2 border-border pl-2 text-xs whitespace-pre-wrap text-muted-foreground">
                  {it.text}
                </div>
              );
            }
            if (it.kind === "tool") {
              return (
                <div key={it.id} className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  <Wrench className="size-3 shrink-0" />
                  <span className="truncate">{it.title}</span>
                  <span className="shrink-0 opacity-70">
                    {it.status === "in_progress" ? "进行中" : it.status === "completed" ? "完成" : it.status}
                  </span>
                </div>
              );
            }
            return (
              <div
                key={it.id}
                className={
                  "flex items-start gap-1.5 rounded px-2 py-1 text-xs " +
                  (it.isError ? "bg-rose-50 text-rose-800" : "bg-secondary/60 text-muted-foreground")
                }
              >
                {it.isError && <ShieldAlert className="mt-0.5 size-3 shrink-0" />}
                <span className="min-w-0 break-words">{it.text}</span>
              </div>
            );
          })}
        </div>
      </div>

      {/* 输入区 */}
      <div className="border-t border-border px-4 py-2">
        <div className="flex items-end gap-2">
          <textarea
            value={a.draft}
            onChange={(e) => a.set({ draft: e.target.value })}
            onKeyDown={(e) => {
              // 回车发送、Shift+回车换行（聊天里的常规约定）。
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                if (canSend) void a.send();
              }
            }}
            rows={2}
            placeholder={a.running ? "让它做什么…（回车发送，Shift+回车换行）" : "先启动后端"}
            className="min-h-0 flex-1 resize-none rounded border border-border bg-transparent px-2 py-1.5 text-sm outline-none focus:border-ring"
          />
          {a.busy ? (
            <button
              type="button"
              onClick={() => void a.cancel()}
              className="inline-flex shrink-0 items-center gap-1.5 rounded border border-border px-3 py-1.5 text-sm hover:bg-secondary"
            >
              <CircleStop className="size-3.5" />
              停
            </button>
          ) : (
            <button
              type="button"
              onClick={() => void a.send()}
              disabled={!canSend}
              className="inline-flex shrink-0 items-center gap-1.5 rounded border border-border px-3 py-1.5 text-sm hover:bg-secondary disabled:opacity-40"
            >
              <Send className="size-3.5" />
              发送
            </button>
          )}
        </div>
        <div className="mt-1 flex items-center gap-2 text-[11px] text-muted-foreground">
          {a.busyMessage && <span>{a.busyMessage}</span>}
          {a.running && !a.busy && (
            <button type="button" onClick={() => void a.stop()} className="hover:underline">
              关掉后端
            </button>
          )}
          <span className="ml-auto">
            agent 不能发布：改完是 draft，发布只能你在文档页点。
          </span>
        </div>
      </div>

      {/* 权限提示：**必须人点** */}
      {a.permission && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/25 p-4" role="dialog" aria-modal="true" aria-label="权限确认">
          <div className="w-full max-w-md rounded border border-border bg-background p-4 shadow-lg">
            <div className="flex items-center gap-2 text-sm font-medium">
              <ShieldAlert className="size-4 text-amber-600" />
              agent 想调用一个需要你批准的工具
            </div>
            <div className="mt-2 text-xs text-muted-foreground">{a.permission.tool}</div>
            <div className="mt-4 flex justify-end gap-2">
              {a.permission.options.map((o) => (
                <button
                  key={o.optionId}
                  type="button"
                  onClick={() => void a.answer(o.optionId)}
                  className={
                    "rounded border border-border px-3 py-1.5 text-sm hover:bg-secondary " +
                    (o.kind.startsWith("allow") ? "" : "text-rose-700")
                  }
                >
                  {o.name}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
