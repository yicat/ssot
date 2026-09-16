/**
 * VaultBrowser：以文档为主的界面（约定见 docs/specs/document.spec.md）。
 *
 * 三栏：**文件树** ｜ **文档 / 数据表（主体）** ｜ **侧栏（反链 / 问题链接）**。
 * 文档是主角：状态、标签、双链、数据表都长在文档里，不做成外挂面板。
 * 检索是**弹窗**（临时动作），不在左栏里占地。
 */
import { AlertTriangle, Database, EyeOff, Link2, MessageSquare, RefreshCw, Search, X } from "lucide-react";
import { useEffect, useMemo, type MouseEvent, type ReactNode } from "react";

import type {
  VaultBacklink,
  VaultDoc,
  VaultIssue,
  VaultItem,
  VaultTableInfo,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { renderMarkdown } from "../../../lib/markdown";
import { DocTree } from "../DocTree";
import { SearchModal } from "../SearchModal";
import { makeResolver, statusStyle, useVaultBrowser } from "./useVaultBrowser";

export function VaultBrowser() {
  const {
    root,
    items,
    tables,
    tableInfos,
    tableData,
    showComments,
    query,
    searchOpen,
    searchCursor,
    hits,
    selected,
    selectedTable,
    doc,
    links,
    notice,
    noticeIsError,
    busy,
    set,
    reload,
    select,
    followLink,
    publish,
    search,
    openSearch,
    closeSearch,
    moveCursor,
    openTable,
    toggleTask,
    toggleComments,
  } = useVaultBrowser();

  // `/` 或 Ctrl+K 打开检索（正在输入框里打字时不抢）。
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const typing = el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable);
      if ((e.key === "/" && !typing) || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k")) {
        e.preventDefault();
        openSearch();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [openSearch]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* 工具条 */}
      <div className="flex items-center gap-3 border-b border-border px-4 py-1.5 text-xs text-muted-foreground">
        <span className="truncate" title={root}>
          {root || "（没有打开的项目）"}
        </span>
        <span className="shrink-0">
          {items.length} 篇 · {tableInfos.length} 表
        </span>
        <button
          type="button"
          onClick={openSearch}
          className="ml-auto inline-flex shrink-0 items-center gap-1.5 rounded border border-border px-2 py-0.5 hover:bg-secondary"
          title="/ 或 Ctrl+K"
        >
          <Search className="size-3" />
          检索
          <span className="text-[10px] opacity-70">/</span>
        </button>
        <button
          type="button"
          onClick={() => void reload()}
          disabled={busy}
          className="inline-flex shrink-0 items-center gap-1 rounded px-2 py-0.5 hover:bg-secondary disabled:opacity-50"
        >
          <RefreshCw className="size-3" />
          刷新
        </button>
      </div>

      {notice && (
        <div
          className={
            "flex items-start gap-2 border-b px-4 py-1.5 text-xs " +
            (noticeIsError ? "border-rose-200 bg-rose-50 text-rose-800" : "border-border bg-secondary/60 text-foreground")
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
        {/* 左栏：文件树 + 数据表 */}
        <aside className="w-72 shrink-0 overflow-auto border-r border-border py-2">
          <DocTree items={items} selected={selected} onSelect={(p) => void select(p)} />
          <TableList tableInfos={tableInfos} tables={tables} selected={selectedTable} onOpen={openTable} />
        </aside>

        {/* 主体：数据表 或 文档 */}
        <section className="min-w-0 flex-1 overflow-auto">
          {selectedTable ? (
            <TablePane file={selectedTable} tableInfos={tableInfos} tableData={tableData} />
          ) : doc ? (
            <DocPane
              doc={doc}
              items={items}
              tableData={tableData}
              showComments={showComments}
              backlinks={links?.backlinks ?? []}
              issues={links?.issues ?? []}
              busy={busy}
              onFollowLink={followLink}
              onSearch={(q) => void search(q)}
              onPublish={publish}
              onToggleTask={toggleTask}
              onToggleComments={toggleComments}
            />
          ) : (
            <div className="p-8 text-sm text-muted-foreground">
              左栏还没有文档。在项目目录下建 <code>docs/</code> 与 <code>raw/</code>，放几篇 markdown 就有了。
            </div>
          )}
        </section>
      </div>

      <SearchModal
        open={searchOpen}
        query={query}
        hits={hits}
        busy={busy}
        cursor={searchCursor}
        onQuery={(q) => void search(q)}
        onMove={moveCursor}
        onOpen={(p) => void (async () => {
          await select(p);
          closeSearch();
        })()}
        onClose={closeSearch}
      />
    </div>
  );
}

function TableList({
  tableInfos,
  tables,
  selected,
  onOpen,
}: {
  tableInfos: VaultTableInfo[];
  tables: string[];
  selected: string | null;
  onOpen: (file: string) => void;
}) {
  if (tableInfos.length === 0 && tables.length === 0) return null;
  return (
    <div className="mt-3 px-3">
      <div className="tree-group mb-1 text-xs font-semibold">数据表</div>
      {tableInfos.length > 0
        ? tableInfos.map((t) => (
            <button
              key={t.file}
              type="button"
              onClick={() => onOpen(t.file)}
              title={`打开 ${t.file}（可 SQL 查，表名 ${t.name}）`}
              className={
                "block w-full rounded px-1 py-1 text-left text-xs " + (t.file === selected ? "bg-secondary" : "hover:bg-secondary/60")
              }
            >
              <span className="flex items-center gap-2">
                <Database className="size-3 shrink-0 text-muted-foreground" />
                <span className="truncate">{t.name}</span>
                <span className="ml-auto shrink-0 text-muted-foreground">{t.rows} 行</span>
              </span>
              <span className="block truncate pl-5 text-[11px] text-muted-foreground">{(t.columns ?? []).join("、")}</span>
            </button>
          ))
        : tables.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => onOpen(t)}
              className="flex w-full items-center gap-2 rounded px-1 py-1 text-left text-xs hover:bg-secondary/60"
            >
              <Database className="size-3 shrink-0 text-muted-foreground" />
              <span className="truncate">{t}</span>
            </button>
          ))}
    </div>
  );
}

/** 数据表视图：按行列展示，并给出可直接跑的查询示例。 */
function TablePane({
  file,
  tableInfos,
  tableData,
}: {
  file: string;
  tableInfos: VaultTableInfo[];
  tableData: Record<string, { columns: string[]; rows: string[][] }>;
}) {
  const info = tableInfos.find((t) => t.file === file);
  const data = tableData[file];
  const name = info?.name ?? file;

  return (
    <div className="mx-auto max-w-[52rem] px-8 py-6">
      <header className="mb-3 border-b border-border pb-3">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="font-heading text-lg font-semibold">{name}</h1>
          <span className="rounded bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground">数据表</span>
          <span className="text-xs text-muted-foreground">{file}</span>
        </div>
        <div className="mt-2 text-xs text-muted-foreground">
          {info ? `${info.rows} 行 · 列：${(info.columns ?? []).join("、")}` : "（索引里没有这张表）"}
        </div>
      </header>

      {data && data.columns.length > 0 ? (
        <div className="overflow-auto rounded border border-border">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr>
                {data.columns.map((c) => (
                  <th key={c} className="border-b border-border bg-secondary px-2 py-1 text-left font-semibold">
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row, i) => (
                <tr key={i} className="even:bg-secondary/40">
                  {data.columns.map((_, j) => (
                    <td key={j} className="border-b border-border px-2 py-1">
                      {row[j] ?? ""}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">取不到这张表的数据。索引可能需要重建（工具条上的「刷新」会重建）。</p>
      )}

      <p className="mt-3 text-xs text-muted-foreground">
        查询示例：<code>ssot vault -root &lt;vault&gt; query 'SELECT * FROM "{name}"'</code>
        {' '}（也能和文档的 front matter 联查，见 <code>docs</code> 表）
      </p>
    </div>
  );
}

function DocPane({
  doc,
  items,
  tableData,
  showComments,
  backlinks,
  issues,
  busy,
  onFollowLink,
  onSearch,
  onPublish,
  onToggleTask,
  onToggleComments,
}: {
  doc: VaultDoc;
  items: VaultItem[];
  tableData: Record<string, { columns: string[]; rows: string[][] }>;
  showComments: boolean;
  backlinks: VaultBacklink[];
  issues: VaultIssue[];
  busy: boolean;
  onFollowLink: (raw: string) => Promise<void>;
  onSearch: (q: string) => void;
  onPublish: (status: string) => Promise<void>;
  onToggleTask: (bodyLine: number, checked: boolean) => Promise<void>;
  onToggleComments: () => void;
}) {
  const st = statusStyle(doc.status);

  const html = useMemo(
    () =>
      renderMarkdown(doc.body ?? "", {
        resolve: makeResolver(items),
        embedTable: (target) => tableData[target] ?? tableData[target.replace(/^\.\//, "")],
        embedDoc: (target) => {
          const hit = items.find((i) => i.path === target || i.path === `${target}.md`);
          return hit ? { title: hit.title, status: hit.status, excerpt: hit.path } : undefined;
        },
      }),
    [doc.body, items, tableData],
  );

  // 正文里的点击用事件委托：双链跳转、任务勾选。
  // 块锚点（`^id`）不做点击行为——它是**被引用的目标**，不是入口；悬停有说明。
  const onBodyClick = (e: MouseEvent<HTMLDivElement>) => {
    const el = e.target as HTMLElement;
    const link = el.closest("a.md-wikilink");
    if (link) {
      e.preventDefault();
      void onFollowLink(link.getAttribute("data-target") ?? "");
      return;
    }
    const task = el.closest("input.md-task") as HTMLInputElement | null;
    if (task) {
      e.preventDefault();
      // 以**源码**为准，不看 DOM 勾选框的运行时状态：浏览器的原生切换会先改它，
      // 依据它会得出「已经是这个状态了」而什么都不做（踩过）。
      const sourceChecked = task.dataset.checked === "true";
      void onToggleTask(Number(task.dataset.line ?? "0"), !sourceChecked);
    }
  };

  return (
    <div className="flex min-h-0">
      <article className="min-w-0 flex-1 px-8 py-6">
        {/* 阅读宽度限住：一行太长眼睛会跳行 */}
        <div className="mx-auto max-w-[46rem]">
          {/* 标记融在文档里：标题 + 状态 + 标签都在文档流里；**标签要比标题轻** */}
          <header className="mb-4 border-b border-border pb-3">
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <h1 className="font-heading text-lg font-semibold">{doc.title}</h1>
              {/* 路径与状态让到右边：标题是这一行要认的东西 */}
              <span className="ml-auto flex items-center gap-2 text-[11px] text-muted-foreground">
                <span>{doc.path}</span>
                <span className={`rounded px-1.5 py-0.5 leading-4 ${st.className}`}>{st.label}</span>
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
              {(doc.tags ?? []).length > 0 && (
                <span className="flex items-center gap-1.5">
                  {(doc.tags ?? []).map((tag) => (
                    <button
                      key={tag}
                      type="button"
                      onClick={() => onSearch(tag)}
                      className="text-[11px] text-muted-foreground hover:text-foreground hover:underline"
                      title={`按标签检索：${tag}`}
                    >
                      #{tag}
                    </button>
                  ))}
                </span>
              )}
              {doc.source && <span className="truncate">来源：{doc.source}</span>}
              <span className="ml-auto flex items-center gap-1">
                <button
                  type="button"
                  onClick={onToggleComments}
                  className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 hover:bg-secondary"
                  title="注释（%%…%%）默认隐藏——注释是给自己看的"
                >
                  {showComments ? <MessageSquare className="size-3" /> : <EyeOff className="size-3" />}
                  {showComments ? "隐藏注释" : "显示注释"}
                </button>
                {["draft", "published", "archived"].map((s) => (
                  <button
                    key={s}
                    type="button"
                    disabled={busy || s === doc.status}
                    onClick={() => void onPublish(s)}
                    className={
                      "rounded px-1.5 py-0.5 disabled:opacity-40 " +
                      (s === doc.status ? "bg-foreground text-background" : "hover:bg-secondary")
                    }
                  >
                    {statusStyle(s).label}
                  </button>
                ))}
              </span>
            </div>
          </header>

          {doc.status !== "published" && (
            <div className="mb-4 flex items-center gap-2 rounded border border-amber-200 bg-amber-50 px-3 py-1.5 text-xs text-amber-900">
              <AlertTriangle className="size-3 shrink-0" />
              这篇还没核验（status: {doc.status}）。未核验的内容可以用，但必须看得见。
            </div>
          )}

          {/* 正文：渲染后的 markdown（表、公式、双链、callout、任务列表都在里面） */}
          <div
            className={"md-body " + (showComments ? "show-comments" : "")}
            onClick={onBodyClick}
            dangerouslySetInnerHTML={{ __html: html }}
          />
        </div>
      </article>

      {/* 侧栏：反链与问题链接（窄窗口让位给文档） */}
      <aside className="hidden w-60 shrink-0 overflow-auto border-l border-border px-4 py-5 xl:block">
        <SidePanel icon={<Link2 className="size-3" />} title={`谁引了它（${backlinks.length}）`}>
          {backlinks.length === 0 && <Empty>没有文档链到它</Empty>}
          {backlinks.map((b, i) => (
            <button
              key={i}
              type="button"
              onClick={() => void onFollowLink(`[[${b.from}]]`)}
              className="block w-full truncate rounded px-1 py-0.5 text-left text-xs hover:bg-secondary"
              title={`${b.from} → ${b.raw}`}
            >
              <span className="text-muted-foreground">←</span> {b.from}
              {b.block && <span className="ml-2 text-muted-foreground">引的是块 ^{b.block}</span>}
            </button>
          ))}
        </SidePanel>

        {issues.length > 0 && (
          <SidePanel icon={<AlertTriangle className="size-3" />} title={`问题链接（${issues.length}）`}>
            {issues.map((is, i) => (
              <div key={i} className="px-1 py-1 text-xs text-rose-800">
                <span className="mr-1 rounded bg-rose-100 px-1">{is.kind === "ambiguous" ? "指不清" : "断链"}</span>
                {is.raw}
                <div className="text-muted-foreground">{is.reason}</div>
              </div>
            ))}
          </SidePanel>
        )}
      </aside>
    </div>
  );
}

function SidePanel({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return (
    <div className="mb-5">
      <div className="mb-1 flex items-center gap-1 text-xs font-semibold text-muted-foreground">
        {icon}
        {title}
      </div>
      <div className="rounded border border-border">{children}</div>
    </div>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return <div className="px-1 py-1 text-xs text-muted-foreground">{children}</div>;
}



