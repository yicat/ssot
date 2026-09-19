/**
 * VaultBrowser 的交互逻辑：拉列表、开文档、点双链、改发布态。
 *
 * 所有读写都走 bindings —— 也就是走 Go 侧的用例层，
 * 所以「谁能发布」那条门在界面上绕不过去（点按钮和敲命令是同一份实现）。
 */
import { useCallback, useEffect, useState } from "react";
import { resumeLastSession } from "../AgentPane/useAgent";
import { useAgentStore } from "../AgentPane/store";

import {
  Backlinks,
  Overview,
  Query,
  Read,
  Resolve,
  Search,
  SetStatus,
  TableInfos,
  Write,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/vaultservice";
import type { VaultDoc, VaultItem, VaultLink } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import type { ResolvedLink } from "../../../lib/markdown";
import { useVaultStore } from "./store";

/** 界面上点按钮的人。核验身份的分级（多个使用者）还没定，先用这一档。 */
const UI_ACTOR = "human:界面";

export function useVaultBrowser() {
  const store = useVaultStore();

  const [locate, setLocate] = useState<{ path: string; kind: "heading" | "block"; value: string } | null>(null);

  const openDoc = useCallback(
    async (path: string, notice?: string, switchMode = true) => {
      const [doc, links] = await Promise.all([Read(path), Backlinks(path)]);
      // switchMode=false 只给「开机自动打开第一篇」用：那时用户可能已经切到 Agent 了，
      // 再把 mode 掰回 doc 会把他的切换冲掉（踩过：点 Agent 没反应就是这个竞态）。
      useVaultStore
        .getState()
        .set({
          selected: path, doc, links,
          ...(switchMode ? { mode: "doc" as const } : {}),
          notice: notice ?? null, noticeIsError: false,
        });
    },
    [],
  );

  const load = useCallback(async () => {
    const { set } = useVaultStore.getState();
    set({ busy: true });
    try {
      const [overview, tableInfos] = await Promise.all([Overview(), TableInfos()]);
      set({
        root: overview.root,
        items: overview.items ?? [],
        tables: overview.tables ?? [],
        tableInfos: tableInfos ?? [],
      });
      // 预取表数据：文档里的 `![[表.csv]]` 是同步渲染的，渲染函数不该等 IO。
      const tableData: Record<string, { columns: string[]; rows: string[][] }> = {};
      for (const t of tableInfos ?? []) {
        try {
          const rs = await Query(`SELECT * FROM "${t.name}" LIMIT 200`, 200);
          tableData[t.file] = {
            columns: rs.columns ?? [],
            rows: (rs.rows ?? []).map((r) => r ?? []),
          };
        } catch {
          // 单张表取不到不影响别的：它照样在左栏列出来，只是嵌入时提示取不到。
        }
      }
      set({ tableData });
      const first = overview.items?.[0]?.path;
      if (first) {
        // 不切模式：用户可能已经切到别的模式了
        await openDoc(first, undefined, false);
      } else {
        set({ selected: null, doc: null, links: null });
      }
    } catch (e) {
      set({ notice: String(e), noticeIsError: true });
    } finally {
      set({ busy: false });
    }
  }, [openDoc]);

  useEffect(() => {
    void load();
  }, [load]);

  /**
   * **打开应用就把 Agent 后端备好，并落回上次那个会话**（用户要的行为）。
   *
   * 靠 `Resume`：它在后端没起时会顺手拉起来（`agentapp.ensureBackend`），**不建新会话**；
   * 没有「上次的会话」时什么也不做——等用户真要说第一句话时再懒启动。
   * 所以「进来一次就多一个空会话」这件事不会再发生。
   */
  useEffect(() => {
    const root = useVaultStore.getState().root;
    if (root) void resumeLastSession(root);
  }, []);

  /**
   * 文件变了要重载列表——**这一步以前是缺的**：agent 在能力层删/建了文件之后，
   * 左边那棵树还拿着旧列表（实测：删了 121 篇剧情，左栏照样列着它们）。
   *
   * 两个触发点，覆盖两种改动来源：
   *  1. **agent 一轮结束**（busy: true → false）：本应用里的 agent 改的（走 MCP 能力层）；
   *  2. **窗口重新聚焦**：从外部改的（另一个 session 的 CLI、git checkout、编辑器）——
   *     外部改时应用收不到通知，切回来对一次是最省事的办法。
   *
   * ⚠️ 不做目录监听（fsnotify）：那要给 Go 侧加 watcher 并往界面推事件；这两个点已覆盖
   * 实际会遇到的情形，等真需要「改一下树立刻动」再上监听。
   */
  useEffect(() => {
    let wasBusy = useAgentStore.getState().busy;
    const unsub = useAgentStore.subscribe((st) => {
      if (wasBusy && !st.busy) void load();
      wasBusy = st.busy;
    });
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => {
      unsub();
      window.removeEventListener("focus", onFocus);
    };
  }, [load]);

  /**
   * 检索：走派生索引（后端在索引缺失时会先建）。
   *
   * 结果留在左栏（而不是弹层）：检索是「换一种方式看同一批文档」，
   * 点结果就开那一篇，语义比弹层顺。
   */
  const search = useCallback(async (q: string) => {
    const { set } = useVaultStore.getState();
    const query = q.trim();
    set({ query: q });
    if (!query) {
      set({ hits: null });
      return;
    }
    try {
      set({ busy: true });
      const hits = await Search(query, 50);
      set({ hits: hits ?? [], searchCursor: 0, notice: null, noticeIsError: false });
    } catch (e) {
      set({ notice: String(e), noticeIsError: true });
    } finally {
      set({ busy: false });
    }
  }, []);

  const clearSearch = useCallback(() => {
    useVaultStore.getState().set({ query: "", hits: null, searchCursor: 0 });
  }, []);

  /** 打开检索弹窗（`/` 或 Ctrl+K）。 */
  const openSearch = useCallback(() => {
    useVaultStore.getState().set({ searchOpen: true });
  }, []);

  /** 关掉弹窗：**保留**结果，方便再按一次 / 接着看。 */
  const closeSearch = useCallback(() => {
    useVaultStore.getState().set({ searchOpen: false });
  }, []);

  /** 弹窗里用上下键选结果（在结果条目上循环）。 */
  const moveCursor = useCallback((delta: number) => {
    const { hits, searchCursor, set } = useVaultStore.getState();
    const n = hits?.length ?? 0;
    if (n === 0) return;
    set({ searchCursor: (((searchCursor + delta) % n) + n) % n });
  }, []);

  /** 在主体区打开一张数据表（左栏点表就走这里）。 */
  const openTable = useCallback((file: string) => {
    useVaultStore.getState().set({ selectedTable: file, mode: "table", searchOpen: false, notice: null, noticeIsError: false });
  }, []);

  /** 点左栏的文档。 */
  const select = useCallback(
    async (path: string) => {
      const { set } = useVaultStore.getState();
      try {
        set({ busy: true });
        await openDoc(path);
      } catch (e) {
        set({ notice: String(e), noticeIsError: true });
      } finally {
        set({ busy: false });
      }
    },
    [openDoc],
  );

  /**
   * 点正文里的双链：解析后跳过去。
   *
   * 解析失败**不吞**：断链与「指不清」（同名多篇）都要说出来——
   * 这正是这套东西该有的样子，报错信息里带着候选。
   */
  const followLink = useCallback(
    async (raw: string) => {
      const { set } = useVaultStore.getState();
      try {
        set({ busy: true });
        const res = await Resolve(raw);
        const parts = [`已跳到 ${res.path}`];
        if (res.block) {
          parts.push(
            res.blockLine > 0
              ? `块锚点 ^${res.block} 在第 ${res.blockLine} 行：${res.blockText}`
              : `块锚点 ^${res.block} **没找到**——溯源断了，别当成已经引到`,
          );
        }
        if (res.heading) {
          parts.push(res.headingLine > 0 ? `标题锚点在第 ${res.headingLine} 行` : `标题锚点「${res.heading}」没找到`);
        }
        // 带锚点时**真的跳过去**（滚动 + 闪一下），不是只弹一句「在第几行」——
        // spec 写的是「点开跳到该标题/该块」（见 document.spec.md 第三节）。
        if (res.block) setLocate({ path: res.path, kind: "block", value: res.block });
        else if (res.heading) setLocate({ path: res.path, kind: "heading", value: res.heading });
        await openDoc(res.path, parts.join("｜"));
      } catch (e) {
        set({ notice: String(e), noticeIsError: true });
      } finally {
        set({ busy: false });
      }
    },
    [openDoc],
  );

  /**
   * 锚点跳转：等**目标文档渲染出来**再滚过去。
   *
   * 为什么用 effect 而不是 openDoc 里立刻滚：文档是先 set 状态、React 下次渲染才进 DOM 的，
   * 立刻查 DOM 会查不到。这里等 doc.path 与目标一致（说明已经渲染）再找元素。
   */
  useEffect(() => {
    if (!locate || !store.doc || store.doc.path !== locate.path) return;
    const sel = locate.kind === "block" ? `[data-block="${locate.value}"]` : `[data-heading="${locate.value}"]`;
    const el = document.querySelector<HTMLElement>(`.md-body ${sel}`);
    if (el) {
      // 三条一起做：
      //  1) scrollIntoView —— 实测这条最可靠（只设 hash 时在 WebView2 里出现过不滚动）
      //  2) 设 hash —— 让 CSS 的 `:target` 命中，链接也可复制
      //  3) 渲染完成后再补一次闪烁类（命令式改 DOM 要等 React 这轮更新落定）
      el.scrollIntoView({ block: "center", behavior: "smooth" });
      window.location.hash = locate.kind === "heading" ? `h-${locate.value}` : `^${locate.value}`;
      // ⚠️ 不再自己加 class 做「闪一下」：正文是 dangerouslySetInnerHTML 渲染的，
      // React 每轮更新会重设 innerHTML，命令式加的 class 会被冲掉或落在已被替换的节点上
      // （实测：MutationObserver 只看到 childList 变动，没有任何 class 变动）。
      // 高亮交给 CSS 的 `:target`（浏览器认 hash），跳转本身靠 scrollIntoView。
    }
    // 找不到也清掉：锚点坏了要让上面那条提示承担说明，不是无限重试。
    setLocate(null);
  }, [locate, store.doc]);

  /** 点正文里的双链：解析后跳过去（供列表里的链接用）。 */
  const follow = useCallback((link: VaultLink) => followLink(link.raw), [followLink]);

  /**
   * 勾选任务列表：把这一行的 `[ ]`/`[x]` 换掉并写回文件。
   *
   * 行号是**正文内行号**（渲染时 markdown-it 给的），写回时直接改正文那一行——
   * 人写的 front matter 与其它行由后端原样保留。
   * 界面上点的是人，所以按 human 记账（只有人能发布，agent 改动才回落 draft）。
   */
  const toggleTask = useCallback(
    async (bodyLine: number, checked: boolean) => {
      const { doc, set } = useVaultStore.getState();
      if (!doc) return;
      const lines = (doc.body ?? "").split("\n");
      const i = bodyLine - 1;
      if (i < 0 || i >= lines.length) {
        set({ notice: `勾选失败：渲染给的是正文第 ${bodyLine} 行，但正文只有 ${lines.length} 行`, noticeIsError: true });
        return;
      }
      const after = lines[i].replace(/\[([ xX])\]/, checked ? "[x]" : "[ ]");
      if (after === lines[i]) {
        // 静默返回会让人以为「点了没反应」；这里说清是哪一行对不上。
        set({
          notice: `勾选失败：正文第 ${bodyLine} 行（文件第 ${(doc.bodyOffset ?? 1) + i} 行）里没有任务标记，内容是「${lines[i].slice(0, 40)}」`,
          noticeIsError: true,
        });
        return;
      }
      lines[i] = after;
      try {
        set({ busy: true });
        const change = await Write(doc.path, lines.join("\n"), UI_ACTOR);
        const fileLine = (doc.bodyOffset ?? 1) + i;
        await openDoc(doc.path, `第 ${fileLine} 行已${checked ? "勾选" : "取消勾选"}（${change.from} → ${change.to}）`);
      } catch (e) {
        set({ notice: String(e), noticeIsError: true });
      } finally {
        set({ busy: false });
      }
    },
    [openDoc],
  );

  const toggleComments = useCallback(() => {
    const { showComments, set } = useVaultStore.getState();
    set({ showComments: !showComments });
  }, []);

  /**
   * 人改发布态。agent 走不了这条：后端会拒（只有人能发布）。
   * 界面这里固定用 human 身份，所以它演示的是「人能发布」；
   * 「agent 不能发布」由 CLI 与测试守着。
   */
  const publish = useCallback(
    async (status: string) => {
      const { doc, set } = useVaultStore.getState();
      if (!doc) return;
      try {
        set({ busy: true });
        const change = await SetStatus(doc.path, status, UI_ACTOR);
        await openDoc(doc.path, `${change.path}：${change.from} → ${change.to}`);
        // 状态变了，左栏的标记也要跟着变。
        const overview = await Overview();
        set({ items: overview.items ?? [] });
      } catch (e) {
        set({ notice: String(e), noticeIsError: true });
      } finally {
        set({ busy: false });
      }
    },
    [openDoc],
  );

  return {
    ...store,
    reload: load,
    select,
    follow,
    followLink,
    publish,
    search,
    clearSearch,
    openSearch,
    closeSearch,
    moveCursor,
    openTable,
    toggleTask,
    toggleComments,
  };
}

/**
 * 渲染时的链接解析：只用已经拿到的文档列表，**同步**回答。
 *
 * 这样渲染是纯函数（同源码→同 HTML），点击时的准确性另由后端 `Resolve` 保证
 * （歧义与锚点命中都在那时才查）。规则与 Go 侧一致：先精确路径，再补 `.md`，最后按文件名。
 */
export function makeResolver(items: VaultItem[]): (target: string) => ResolvedLink | undefined {
  const norm = (t: string) => t.trim().replace(/\\/g, "/").replace(/^\.\//, "").replace(/^\//, "");
  const byPath = new Map(items.map((i) => [i.path, i]));
  const byBase = new Map<string, VaultItem[]>();
  for (const i of items) {
    const base = i.path.split("/").pop()!.replace(/\.md$/, "");
    byBase.set(base, [...(byBase.get(base) ?? []), i]);
  }
  return (target: string) => {
    const t = norm(target);
    if (!t) return undefined;
    const hit = byPath.get(t) ?? byPath.get(`${t}.md`);
    if (hit) return { kind: "doc", path: hit.path, title: hit.title, status: hit.status };
    // ⚠️ 退一步按**文件名**匹配时，要用目标串的 basename（而不是整个 `docs/语法示例`）——
    // 后端 domain/vault.Resolve 就是这么做的。少这一步会在文档挪过位置之后
    // 把**还能解析**的链接误判成断链：渲染成红字、点它却不报错，两边规则不一致（踩过）。
    const base = t.split("/").pop()!.replace(/\.md$/, "");
    const same = byBase.get(base);
    if (!same || same.length === 0) return { kind: "broken" };
    if (same.length > 1) return { kind: "ambiguous", candidates: same.map((i) => i.path) };
    return { kind: "doc", path: same[0].path, title: same[0].title, status: same[0].status };
  };
}

/** 状态徽标的样式与中文标签。draft（未核验）要最显眼。 */
export function statusStyle(status: string): { className: string; label: string } {
  switch (status) {
    case "published":
      return { className: "bg-emerald-100 text-emerald-900", label: "已发布" };
    case "archived":
      return { className: "bg-secondary text-muted-foreground", label: "已归档" };
    default:
      return { className: "bg-amber-100 text-amber-900", label: "未核验" };
  }
}

/** 文档是不是「未核验」——列表与正文都要靠它标出重点。 */
export function isDraft(doc: { status: string } | null | undefined): boolean {
  return !doc || doc.status !== "published";
}

export type { VaultDoc };









