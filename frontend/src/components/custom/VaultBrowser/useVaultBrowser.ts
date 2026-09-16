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
  Read,
  Resolve,
  Search,
  SetStatus,
  TableInfos,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/vaultservice";
import type { VaultDoc, VaultLink } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
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
      set({ hits: hits ?? [], notice: null, noticeIsError: false });
    } catch (e) {
      set({ notice: String(e), noticeIsError: true });
    } finally {
      set({ busy: false });
    }
  }, []);

  const clearSearch = useCallback(() => {
    useVaultStore.getState().set({ query: "", hits: null });
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
  const follow = useCallback(
    async (link: VaultLink) => {
      const { set } = useVaultStore.getState();
      try {
        set({ busy: true });
        const res = await Resolve(link.raw);
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

  return { ...store, reload: load, select, follow, publish, search, clearSearch };
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
