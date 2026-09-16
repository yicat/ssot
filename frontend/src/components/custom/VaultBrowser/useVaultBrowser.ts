/**
 * VaultBrowser 的交互逻辑：拉列表、开文档、点双链、改发布态。
 *
 * 所有读写都走 bindings —— 也就是走 Go 侧的用例层，
 * 所以「谁能发布」那条门在界面上绕不过去（点按钮和敲命令是同一份实现）。
 */
import { useCallback, useEffect } from "react";

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

  const openDoc = useCallback(
    async (path: string, notice?: string) => {
      const [doc, links] = await Promise.all([Read(path), Backlinks(path)]);
      useVaultStore.getState().set({ selected: path, doc, links, notice: notice ?? null, noticeIsError: false });
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
        await openDoc(first);
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
    useVaultStore.getState().set({ selectedTable: file, searchOpen: false, notice: null, noticeIsError: false });
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
        await openDoc(res.path, parts.join("｜"));
      } catch (e) {
        set({ notice: String(e), noticeIsError: true });
      } finally {
        set({ busy: false });
      }
    },
    [openDoc],
  );

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


