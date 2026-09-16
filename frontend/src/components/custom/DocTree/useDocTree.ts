/**
 * 文件树的构建与折叠状态（渲染见 index.tsx）。
 *
 * 规则见 docs/specs/document.spec.md 第二节：
 *  - 层级最多三层：`docs/` = 1、`docs/式神/` = 2、`docs/式神/茨木童子.md` = 3
 *  - 同层**文件夹在前、文档在后**，各自按名称排序（文档按标题排，文件名只用来寻址）
 *  - 排序必须确定：同一份 vault，树的样子永远一样
 *  - 超过三层的文档仍然显示，但带提示——层级乱长是要被看见的
 */
import { useCallback, useMemo, useState } from "react";

import type { VaultItem } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type TreeNode = {
  kind: "folder" | "doc";
  /** 显示名：文件夹用目录名，文档用标题。 */
  name: string;
  /** 完整路径（文件夹如 docs/式神，文档如 docs/式神/茨木童子.md）。 */
  path: string;
  depth: number;
  /** 文档才有。 */
  layer?: string;
  status?: string;
  /** 超过三层（layer + 2 层文件夹）的文档。 */
  tooDeep?: boolean;
  /** **落在顶层**的文档（两层根下直接摊文件，不合规，见 document.spec.md 第一节）。 */
  topLevel?: boolean;
  children: TreeNode[];
};

/** 两层的分组标题：目录名是结构，标签给人看。 */
export const LAYER_LABEL: Record<string, string> = { docs: "整理层", raw: "原始层" };

/** 允许的最大层数：根 + 一层文件夹 + 文档。 */
export const MAX_DEPTH = 3;

export function buildTree(items: VaultItem[]): TreeNode[] {
  const roots = new Map<string, TreeNode>();

  for (const it of items) {
    const segs = it.path.split("/").filter(Boolean);
    if (segs.length === 0) continue;
    const layer = segs[0];
    let node: TreeNode | undefined = roots.get(layer);
    if (node === undefined) {
      const created: TreeNode = {
        kind: "folder",
        name: LAYER_LABEL[layer] ?? layer, // 中文分组标题（见 document.spec.md）
        path: layer,
        depth: 1,
        children: [],
      };
      roots.set(layer, created);
      node = created;
    }
    for (let i = 1; i < segs.length; i++) {
      const isFile = i === segs.length - 1;
      const path = segs.slice(0, i + 1).join("/");
      if (isFile) {
        node.children.push({
          kind: "doc",
          name: it.title || segs[i].replace(/\.md$/, ""),
          path,
          depth: i + 1,
          layer: it.layer,
          status: it.status,
          tooDeep: i + 1 > MAX_DEPTH,
          topLevel: i + 1 === 2, // 只在两层根下 = 没进分类文件夹
          children: [],
        });
      } else {
        let child: TreeNode | undefined = node.children.find((c) => c.kind === "folder" && c.path === path);
        if (child === undefined) {
          const created: TreeNode = { kind: "folder", name: segs[i], path, depth: i + 1, children: [] };
          node.children.push(created);
          child = created;
        }
        node = child;
      }
    }
  }

  const sort = (nodes: TreeNode[]): TreeNode[] => {
    nodes.sort((a, b) => {
      if (a.kind !== b.kind) return a.kind === "folder" ? -1 : 1;
      // 文档按标题排：人记得的是标题，不是文件名。
      return a.name.localeCompare(b.name, "zh-Hans-CN", { numeric: true });
    });
    for (const n of nodes) sort(n.children);
    return nodes;
  };

  return sort([...roots.values()]);
}

export function useDocTree(items: VaultItem[]) {
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const tree = useMemo(() => buildTree(items), [items]);

  const toggle = useCallback((path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  // 两类不合规分开数：越层是一回事，摊在顶层是另一回事。
  const counts = useMemo(() => {
    let deep = 0;
    let top = 0;
    const walk = (nodes: TreeNode[]) => {
      for (const node of nodes) {
        if (node.tooDeep) deep++;
        if (node.topLevel) top++;
        walk(node.children);
      }
    };
    walk(tree);
    return { deep, top };
  }, [tree]);

  return { tree, collapsed, toggle, deepCount: counts.deep, topLevelCount: counts.top };
}




