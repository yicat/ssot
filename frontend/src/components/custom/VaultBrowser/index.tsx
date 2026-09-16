/**
 * VaultBrowser：以文档为中心的浏览界面。
 *
 * 左栏是文档（按两层分组 + 数据表），右栏是当前文档：元信息、**带行号的正文**、
 * 双链、反链与问题链接。
 *
 * 两个刻意做出来的东西：
 *  1. **未核验（draft）永远显眼**——列表里的标记、正文顶上的条，都是这条立场。
 *  2. **正文显示文件行号**——因为块级锚点报的就是文件行号，
 *     点一条 `[[raw/…#^块]]` 说"跳到第 13 行"时，人能当场对上。
 *
 * 渲染只读 store / hook 给的东西，不含别的逻辑（见 store.ts 与 useVaultBrowser.ts）。
 */
import type { ReactNode } from "react";
import { AlertTriangle, Database, Link2, RefreshCw, X } from "lucide-react";

import type {
  VaultBacklink,
  VaultDoc,
  VaultIssue,
  VaultItem,
  VaultLink,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useVaultBrowser, statusStyle } from "./useVaultBrowser";

export function VaultBrowser() {
  const { root, items, tables, selected, doc, links, notice, noticeIsError, busy, set, reload, select, follow, publish } =
    useVaultBrowser();

  const docs = items.filter((it) => it.layer === "docs");
  const raw = items.filter((it) => it.layer === "raw");

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-3 border-b border-border px-4 py-1.5 text-xs text-muted-foreground">
        <span className="truncate" title={root}>
          {root || "（没有打开的项目）"}
        </span>
        <span className="shrink-0">
          {items.length} 篇文档 · {tables.length} 张表
        </span>
        <button
          type="button"
          onClick={() => void reload()}
          disabled={busy}
          className="ml-auto inline-flex items-center gap-1 rounded px-2 py-0.5 hover:bg-secondary disabled:opacity-50"
        >
          <RefreshCw className="size-3" />
          刷新
        </button>
      </div>

      {notice && (
        <div
          className={
            "flex items-start gap-2 border-b px-4 py-1.5 text-xs " +
            (noticeIsError
              ? "border-rose-200 bg-rose-50 text-rose-800"
              : "border-border bg-secondary/60 text-foreground")
          }
        >
          {noticeIsError && <AlertTriangle className="mt-0.5 size-3 shrink-0" />}
          <span className="min-w-0 break-words">{notice}</span>
          <button
            type="button"
            aria-label="关闭提示"
            onClick={() => set({ notice: null, noticeIsError: false })}
            className="ml-auto shrink-0 rounded p-0.5 hover:bg-black/10"
          >
            <X className="size-3" />
          </button>
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <aside className="w-72 shrink-0 overflow-auto border-r border-border py-2">
          <Group title={`整理层 docs/`} count={docs.length} items={docs} selected={selected} onSelect={select} />
          <Group title={`原始层 raw/`} count={raw.length} items={raw} selected={selected} onSelect={select} />
          {tables.length > 0 && (
            <div className="mt-2 px-3">
              <div className="mb-1 text-xs font-semibold text-muted-foreground">数据表</div>
              {tables.map((t) => (
                <div key={t} className="flex items-center gap-2 px-2 py-1 text-xs text-muted-foreground">
                  <Database className="size-3 shrink-0" />
                  <span className="truncate" title={t}>
                    {t}
                  </span>
                </div>
              ))}
            </div>
          )}
        </aside>

        <section className="min-w-0 flex-1 overflow-auto">
          {doc ? (
            <DocPane doc={doc} links={links?.backlinks ?? []} issues={links?.issues ?? []} onFollow={follow} onPublish={publish} busy={busy} />
          ) : (
            <div className="p-8 text-sm text-muted-foreground">
              左栏还没有文档。在项目目录下建 <code>docs/</code> 与 <code>raw/</code>，放几篇 markdown 就有了。
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

function Group({
  title,
  count,
  items,
  selected,
  onSelect,
}: {
  title: string;
  count: number;
  items: VaultItem[];
  selected: string | null;
  onSelect: (path: string) => Promise<void>;
}) {
  return (
    <div className="px-3">
      <div className="mb-1 text-xs font-semibold text-muted-foreground">
        {title}
        <span className="ml-1 font-normal">（{count}）</span>
      </div>
      {items.map((it) => {
        const st = statusStyle(it.status);
        const active = it.path === selected;
        return (
          <button
            key={it.path}
            type="button"
            onClick={() => void onSelect(it.path)}
            className={
              "flex w-full items-start gap-2 rounded px-2 py-1 text-left " +
              (active ? "bg-secondary" : "hover:bg-secondary/60")
            }
          >
            <span className={`mt-0.5 shrink-0 rounded px-1 text-[10px] leading-4 ${st.className}`}>{st.label}</span>
            <span className="min-w-0">
              <span className="block truncate text-sm">{it.title}</span>
              <span className="block truncate text-[11px] text-muted-foreground">{it.path}</span>
            </span>
          </button>
        );
      })}
      {count === 0 && <div className="px-2 py-1 text-xs text-muted-foreground">（空）</div>}
    </div>
  );
}

function DocPane({
  doc,
  links,
  issues,
  onFollow,
  onPublish,
  busy,
}: {
  doc: VaultDoc;
  links: VaultBacklink[];
  issues: VaultIssue[];
  onFollow: (l: VaultLink) => Promise<void>;
  onPublish: (status: string) => Promise<void>;
  busy: boolean;
}) {
  const st = statusStyle(doc.status);
  const bodyLines = (doc.body ?? "").split("\n");

  return (
    <div className="p-6">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h1 className="font-heading text-base font-medium">{doc.title}</h1>
        <span className={`rounded px-1.5 py-0.5 text-xs ${st.className}`}>{st.label}</span>
        <span className="text-xs text-muted-foreground">{doc.path}</span>
      </div>

      {doc.status !== "published" && (
        <div className="mb-3 flex items-center gap-2 rounded border border-amber-200 bg-amber-50 px-3 py-1.5 text-xs text-amber-900">
          <AlertTriangle className="size-3 shrink-0" />
          这篇还没核验（status: {doc.status}）。未核验的内容可以用，但必须看得见。
        </div>
      )}

      <div className="mb-4 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
        {(doc.tags ?? []).length > 0 && <span>标签：{(doc.tags ?? []).join("、")}</span>}
        {doc.source && <span>来源：{doc.source}</span>}
        <span className="ml-auto flex items-center gap-1">
          {["draft", "published", "archived"].map((s) => (
            <button
              key={s}
              type="button"
              disabled={busy || s === doc.status}
              onClick={() => void onPublish(s)}
              className={
                "rounded px-2 py-0.5 disabled:opacity-40 " +
                (s === doc.status ? "bg-foreground text-background" : "hover:bg-secondary")
              }
            >
              {statusStyle(s).label}
            </button>
          ))}
        </span>
      </div>

      {/* 正文带文件行号：块锚点报的就是这个行号，点了能当场对上。 */}
      <div className="rounded border border-border bg-card p-3 text-sm leading-6">
        {bodyLines.map((line, i) => (
          <div key={i} className="flex gap-3">
            <span className="w-8 shrink-0 text-right text-xs tabular-nums text-muted-foreground select-none">
              {(doc.bodyOffset ?? 1) + i}
            </span>
            <span className="min-w-0 whitespace-pre-wrap break-words">{line || " "}</span>
          </div>
        ))}
      </div>

      <Panel icon={<Link2 className="size-3" />} title={`双链（${(doc.links ?? []).length}）`}>
        {(doc.links ?? []).length === 0 && <Empty>这篇没有链出去的引用</Empty>}
        {(doc.links ?? []).map((l, i) => (
          <button
            key={i}
            type="button"
            onClick={() => void onFollow(l)}
            className="block w-full truncate rounded px-2 py-1 text-left text-xs hover:bg-secondary"
            title={l.raw}
          >
            {l.raw}
            {l.block && <span className="ml-2 text-muted-foreground">→ 块 ^{l.block}</span>}
            {l.heading && <span className="ml-2 text-muted-foreground">→ 标题 {l.heading}</span>}
            {l.embed && <span className="ml-2 text-muted-foreground">（嵌入）</span>}
          </button>
        ))}
      </Panel>

      <Panel icon={<Link2 className="size-3" />} title={`谁引了它（${links.length}）`}>
        {links.length === 0 && <Empty>没有文档链到它</Empty>}
        {links.map((b, i) => (
          <div key={i} className="truncate px-2 py-1 text-xs" title={`${b.from} → ${b.raw}`}>
            <span className="text-muted-foreground">←</span> {b.from}
            {b.block && <span className="ml-2 text-muted-foreground">引的是块 ^{b.block}</span>}
          </div>
        ))}
      </Panel>

      {issues.length > 0 && (
        <Panel icon={<AlertTriangle className="size-3" />} title={`问题链接（${issues.length}）`}>
          {issues.map((is, i) => (
            <div key={i} className="px-2 py-1 text-xs text-rose-800">
              <span className="mr-1 rounded bg-rose-100 px-1">{is.kind === "ambiguous" ? "指不清" : "断链"}</span>
              {is.raw}
              <div className="text-muted-foreground">{is.reason}</div>
            </div>
          ))}
        </Panel>
      )}
    </div>
  );
}

function Panel({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return (
    <div className="mt-5">
      <div className="mb-1 flex items-center gap-1 text-xs font-semibold text-muted-foreground">
        {icon}
        {title}
      </div>
      <div className="rounded border border-border">{children}</div>
    </div>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return <div className="px-2 py-1 text-xs text-muted-foreground">{children}</div>;
}
