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
 *  - **助手回复按 Markdown 渲染**，用的是文档正文那套渲染器（代码块 / 表格 / 公式 / 列表都在），
 *    外面套 `.md-body` 拿排版，再用几个类把「文档级」的字号压回聊天的尺度。
 *    双链（`[[…]]`）在聊天里没有目标状态可依，所以按普通文字显示，**不标成断链**。
 */
import { Bot, CircleStop, ListTree, Play, Power, Send, Settings2, ShieldAlert, Wrench } from "lucide-react";
import { useEffect, useRef } from "react";

import { renderMarkdown } from "../../../lib/markdown";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../ui/select";

import { useAgent } from "./useAgent";

type Props = {
  /** 打开设置（配置页在 VaultBrowser 那边统一管）。 */
  onOpenSettings: () => void;
};

/**
 * 助手回复的渲染：**复用文档正文那套渲染器**（CommonMark + GFM + KaTeX），
 * 所以代码块、表格、列表、公式都能正常显示，不用在聊天里另造一套。
 *
 * 两处刻意的取舍：
 *  - 外面套 `.md-body` 拿排版，再用几个类把「文档级」的字号/间距压回聊天尺度
 *    （标题不该比气泡还大、段间距不该比消息间距还宽）；
 *  - 双链在聊天里不跳转、也不标成断链（见下面对 `.md-broken` 的覆盖）。
 *
 * ⚠️ 这里**不缓存**渲染结果：markdown-it 解析一条消息很便宜，而 `useMemo` 不能写在
 * 回调/条件里（React 的规矩），写在这儿会踩 hook 顺序。
 */
function Prose({ text }: { text: string }) {
  return (
    <div
      className="md-body text-sm leading-relaxed [&_h1]:mt-0 [&_h1]:text-base [&_h2]:mt-2 [&_h2]:text-[15px] [&_h3]:text-sm [&_p]:my-1.5 [&_pre]:my-2 [&_pre]:text-xs [&_ul]:my-1.5 [&_ol]:my-1.5 [&_table]:my-2 [&_table]:text-xs [&_.md-broken]:text-inherit [&_.md-broken]:no-underline"
      dangerouslySetInnerHTML={{ __html: renderMarkdown(text, { resolve: () => undefined }) }}
    />
  );
}

/**
 * 思考那一行的小预览：收起时给一句人话，让人知道里面大概在想什么（太长就截断）。
 * 换行压成空格，免得 summary 被撑成多行。
 */
function thoughtPreview(text: string): string {
  const one = text.replace(/\s+/g, " ").trim();
  return one.length > 48 ? one.slice(0, 48) + "…" : one;
}

export function AgentPane({ onOpenSettings }: Props) {
  const a = useAgent();

  /** 消息区：自动跟随到底部，**但人往上滚了就不跟随**（聊天里的常规约定）。
   *
   * 用 ref 而不是 state 记「是否跟随」：它只是给滚动用的一个开关，
   * 放进 state 会让每次滚动都触发一次整面板重渲染（没必要）。
   */
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  /** 距底部 24px 以内算「在底部」——留点余量，免得小数像素把人判成「滚上去了」。 */
  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };

  // 每次渲染都对一次底部：流式追加时不改数组身份，靠 deps 是抓不住的（踩过这种）。
  useEffect(() => {
    const el = scrollRef.current;
    if (!el || !stick.current) return;
    el.scrollTop = el.scrollHeight;
  });

  /** 发送：人刚发完一句，一定是想看着下方的回答，所以强制恢复跟随。 */
  const submit = () => {
    stick.current = true;
    void a.send();
  };

  // 打开面板就拉状态；**没起后端就自动起**。
  //
  // ⚠️ 这里**不能**加 `if (a.vault)` 这类条件：`vault` 本身就是从 Status 拿的，
  // 首次挂载时它还是空字符串，条件不成立 → 根本不查 → 界面一直显示「后端未启动」，
  // 而后端其实在跑（踩过：切走再切回来、或页面 reload 之后就是这样）。
  useEffect(() => {
    // 打开面板就把后端拉起来：DSH 那样的体感（不用人先点一下）。boot 会先拉状态，没起才起。
    void a.boot();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const canSend = a.running && !a.busy && a.draft.trim().length > 0;

  return (
    <div className="flex h-full min-h-0 flex-1 flex-col overflow-hidden">
      {/* 顶部：后端状态 + 模型 + 会话 */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-4 py-2 text-xs">
        <Bot className="size-3.5 text-muted-foreground" />
        <span className="text-muted-foreground">
          {a.running ? `${a.agent || "后端"}${a.version ? " " + a.version : ""}` : "后端未启动"}
        </span>
        {a.sessionId && <span className="truncate text-[11px] text-muted-foreground/70">会话 {a.sessionId.slice(0, 8)}</span>}

        <div className="ml-auto flex items-center gap-2">
          {a.running && (
            <button
              type="button"
              onClick={() => void a.stop()}
              disabled={a.busy}
              className="inline-flex items-center gap-1 rounded px-2 py-0.5 hover:bg-secondary disabled:opacity-40"
              title={a.busy ? "正在跑，等这一轮结束再关" : "关掉后端进程（下次打开面板会自动起来）"}
            >
              <Power className="size-3" />
              关掉后端
            </button>
          )}
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
          {/* 会话：**标准下拉**（跟模型选择一个样式），不是自己搓的按钮+列表。
              打开时才去拉历史（onOpenChange），省得每次切进来都查一遍。 */}
          <Select
            value={a.sessionId || "new"}
            onValueChange={(v) => {
              if (v === "new") void a.newSession();
              else void a.resume(v);
            }}
            onOpenChange={(open) => {
              if (open) void a.loadSessions();
            }}
          >
            <SelectTrigger className="h-6 w-52 gap-1 text-xs" title="切换会话 / 新开一个（不会串到别的项目）">
              <ListTree className="size-3 shrink-0" />
              <SelectValue placeholder="会话" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="new">＋ 新会话（清空的）</SelectItem>
              {a.sessions.map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  {(s.title || s.id) + (s.current ? "（当前）" : "")}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
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

      {/* 消息流 */}
      <div ref={scrollRef} onScroll={onScroll} className="min-h-0 flex-1 overflow-auto px-4 py-3">
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

        <div className="flex flex-col gap-3">
          {a.items.map((it) => {
            if (it.kind === "user") {
              return (
                <div
                  key={it.id}
                  className="max-w-[85%] self-end rounded-2xl rounded-br-md bg-secondary px-3.5 py-2 text-sm leading-relaxed whitespace-pre-wrap"
                >
                  {it.text}
                </div>
              );
            }
            if (it.kind === "assistant") {
              return (
                <div key={it.id} className="max-w-[92%] self-start">
                  <Prose text={it.text} />
                </div>
              );
            }
            if (it.kind === "thought") {
              // 推理是**过程**：默认收起（跟 DSH 一样），需要时点开看。
              // 用原生 <details> 而不是 state：不用管展开状态，也不会被流式更新冲掉。
              return (
                <details key={it.id} className="max-w-[92%] self-start">
                  <summary className="cursor-pointer list-none text-[10px] tracking-wide text-muted-foreground/60 hover:text-muted-foreground">
                    思考 <span className="opacity-70">{thoughtPreview(it.text)}</span>
                  </summary>
                  <div className="mt-1 border-l border-border/70 pl-2 text-xs leading-relaxed whitespace-pre-wrap text-muted-foreground/80 italic">
                    {it.text}
                  </div>
                </details>
              );
            }
            if (it.kind === "tool") {
              return (
                <div key={it.id} className="self-start">
                  <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-border/70 bg-secondary/40 px-2 py-0.5 text-[11px] text-muted-foreground">
                    <Wrench className="size-3 shrink-0" />
                    <span className="truncate">{it.title}</span>
                    <span className="shrink-0 text-[10px] opacity-70">
                      {it.status === "in_progress" ? "进行中" : it.status === "completed" ? "完成" : it.status}
                    </span>
                  </span>
                </div>
              );
            }
            return (
              <div
                key={it.id}
                className={
                  "flex max-w-[92%] items-start gap-1.5 self-start rounded-lg px-2.5 py-1.5 text-xs leading-relaxed " +
                  (it.isError ? "bg-rose-50 text-rose-800" : "bg-secondary/50 text-muted-foreground")
                }
              >
                {it.isError && <ShieldAlert className="mt-0.5 size-3 shrink-0" />}
                <span className="min-w-0 break-words">{it.text}</span>
              </div>
            );
          })}
          {a.busy && (
            <div className="self-start text-xs text-muted-foreground/70" role="status" aria-live="polite">
              正在回答…
            </div>
          )}
        </div>
      </div>

      {/* 输入区 */}
      <div className="shrink-0 border-t border-border bg-background px-4 py-2">
        <div className="flex items-end gap-2">
          <textarea
            value={a.draft}
            onChange={(e) => a.set({ draft: e.target.value })}
            onKeyDown={(e) => {
              // 回车发送、Shift+回车换行（聊天里的常规约定）。
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                if (canSend) submit();
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
              onClick={submit}
              disabled={!canSend}
              className="inline-flex shrink-0 items-center gap-1.5 rounded border border-border px-3 py-1.5 text-sm hover:bg-secondary disabled:opacity-40"
            >
              <Send className="size-3.5" />
              发送
            </button>
          )}
        </div>
        {/* 忙碌提示：只在真忙的时候出现，不占一行空位。
            这里**不再**写「agent 不能发布」那句提示——门在能力层（agent.spec.md §1），
            界面上少摆一句口号，规则照样成立。 */}
        {a.busyMessage && <div className="mt-1 text-[11px] text-muted-foreground">{a.busyMessage}</div>}
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
