/**
 * 配置弹窗：只管我们自己的三层（项目 / Agent 后端 / 外观）。
 *
 * 口径见 docs/specs/settings.spec.md：
 *  - **不管模型与 API key**——那是会话级与后端自己的事；
 *  - 后端要能**逐条报缺什么**（DSH 入口、profile、CLI 二进制），只报告不自动修；
 *  - 设置读不动就明确说出来，不许静默用默认值。
 */
import { CheckCircle2, ExternalLink, X, XCircle } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import * as AgentService from "../../../../bindings/github.com/ngnl5/ssot/internal/api/agentservice";
import type { AppSettingsView } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

type Props = {
  open: boolean;
  onClose: () => void;
};

export function SettingsModal({ open, onClose }: Props) {
  const [view, setView] = useState<AppSettingsView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setView(await AgentService.Settings());
      setError(null);
    } catch (err) {
      setError(String(err));
    }
  }, []);

  useEffect(() => {
    if (open) void load();
  }, [open, load]);

  if (!open) return null;

  /** 局部改一项（保存前只改内存里的视图）。 */
  const patch = (p: Partial<AppSettingsView>) => setView((v) => (v ? { ...v, ...p } : v));

  const save = async () => {
    if (!view) return;
    setBusy(true);
    try {
      // 存完后端会重新检查一遍，所以这里直接用返回值刷新「缺什么」。
      setView(await AgentService.SaveSettings(view));
      setError(null);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  const field = (
    label: string,
    key: keyof AppSettingsView,
    hint: string,
  ) => (
    <label className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <input
        type="text"
        // data-field 是给测试用的稳定锚点：**不要**让测试按 input 的先后下标去取字段——
        // 踩过：下标取错了一个字段，测试把 "acp" 写进了 DSH 安装目录，而断言因为回读同一个错字段
        // 还「通过」了（表现是点启动后端报 `acp\DSH Desktop.exe` 找不到）。
        data-field={String(key)}
        value={String(view?.[key] ?? "")}
        onChange={(e) => patch({ [key]: e.target.value } as Partial<AppSettingsView>)}
        className="rounded border border-border bg-transparent px-2 py-1 text-sm outline-none focus:border-ring"
      />
      <span className="text-[11px] text-muted-foreground/80">{hint}</span>
    </label>
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-auto bg-black/25 p-4 pt-16"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label="配置"
    >
      <div className="w-full max-w-2xl rounded border border-border bg-background p-5 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-2">
          <h2 className="font-heading text-base font-semibold">配置</h2>
          <button type="button" onClick={onClose} aria-label="关闭配置" className="ml-auto rounded p-1 hover:bg-secondary">
            <X className="size-4" />
          </button>
        </div>

        {error && (
          <div className="mt-3 flex items-start gap-2 rounded bg-rose-50 px-2 py-1.5 text-xs text-rose-800">
            <XCircle className="mt-0.5 size-3.5 shrink-0" />
            <span className="min-w-0 break-words">{error}</span>
          </div>
        )}

        {view?.notes?.map((n) => (
          <div key={n} className="mt-2 rounded bg-amber-50 px-2 py-1 text-xs text-amber-900">
            {n}
          </div>
        ))}

        <div className="mt-4 flex flex-col gap-4">
          {/* 项目 */}
          <section className="flex flex-col gap-2">
            <h3 className="text-sm">
              项目 <span className="text-xs text-muted-foreground">（vault 从哪来）</span>
            </h3>
            {field("项目根目录", "projectsRoot", "相对路径按 App 的工作目录算；只列含 project.yml 的目录")}
            <div className="text-[11px] text-muted-foreground">
              当前实际用的项目根：<code>{view?.projectRoot || "（未知）"}</code>
            </div>
          </section>

          {/* Agent 后端 */}
          <section className="flex flex-col gap-2">
            <h3 className="text-sm">
              Agent 后端 <span className="text-xs text-muted-foreground">（聊天用哪个后端；模型与 key 不在这里）</span>
            </h3>
            {field("DSH 安装目录", "dshInstall", "下面要有「DSH Desktop.exe」与 resources/")}
            {field("profile 名", "profile", "我们建的那个 ACP profile（默认 acp）")}
            {field("ssot CLI 路径", "cliBin", "MCP 服务器就是它：<这个路径> mcp --root <vault>；跑 wails3 task build:cli 生成")}
            {field("actor 名字", "actor", "agent 写入时记进 git trailer 的名字（agent:<名字>）")}
          </section>

          {/* 后端检查 */}
          <section className="flex flex-col gap-1">
            <h3 className="text-sm">后端检查</h3>
            <ul className="flex flex-col gap-1">
              {(view?.checks ?? []).map((c) => (
                <li key={c.name} className="flex items-start gap-2 text-xs">
                  {c.ok ? (
                    <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-emerald-600" />
                  ) : (
                    <XCircle className="mt-0.5 size-3.5 shrink-0 text-rose-600" />
                  )}
                  <span className="shrink-0">{c.name}</span>
                  <code className="min-w-0 break-all text-muted-foreground">{c.path}</code>
                  {!c.ok && <span className="shrink-0 text-rose-700">{c.hint}</span>}
                </li>
              ))}
            </ul>
          </section>

          {/* 外观 */}
          <section className="flex flex-col gap-2">
            <h3 className="text-sm">外观</h3>
            <label className="flex items-center gap-2 text-xs">
              <span className="text-muted-foreground">主题</span>
              <select
                value={view?.theme ?? "system"}
                onChange={(e) => patch({ theme: e.target.value })}
                className="rounded border border-border bg-transparent px-1.5 py-0.5 text-xs"
              >
                <option value="system">跟随系统</option>
                <option value="light">浅色</option>
                <option value="dark">深色</option>
              </select>
              <span className="text-muted-foreground/80">字号那三档是 spec 定死的口径，不给随便拧</span>
            </label>
          </section>

          {/* 路径与保存 */}
          <div className="flex items-center gap-3 border-t border-border pt-3 text-[11px] text-muted-foreground">
            <span className="min-w-0 truncate" title={view?.settingsPath}>
              设置文件：<code>{view?.settingsPath}</code>
            </span>
            <button
              type="button"
              onClick={() => void load()}
              className="ml-auto inline-flex shrink-0 items-center gap-1 rounded px-2 py-0.5 hover:bg-secondary"
            >
              <ExternalLink className="size-3" />
              重新检查
            </button>
            <button
              type="button"
              onClick={() => void save()}
              disabled={busy}
              className="shrink-0 rounded border border-border px-3 py-1 text-xs hover:bg-secondary disabled:opacity-50"
            >
              {busy ? "保存中…" : "保存"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
