/**
 * 文件树：vault 的导航（构建与折叠逻辑见 useDocTree.ts）。
 *
 * 形状（见 docs/specs/document.spec.md 第一、二节）：
 *
 *   整理层                 ← 分组标题，样式与「数据表」那组一致
 *     式神                 ← 文件夹（可折叠）
 *       茨木童子   未核验    ← **状态放行尾**，标题在最前面
 *   原始层
 *     灰机wiki
 *       茨木童子   未核验
 *
 * 两条口径：
 *  - 两层用**中文**分组标题：`docs` / `raw` 是结构名，不是给人看的标签。
 *  - **状态标记放行尾**：标题是这一行要认的东西，标记只是附注，不该先看到。
 */
import { AlertTriangle, ChevronDown, ChevronRight, Folder, FolderOpen } from "lucide-react";

import type { VaultItem } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { statusStyle } from "../VaultBrowser/useVaultBrowser";
import { MAX_DEPTH, useDocTree, type TreeNode } from "./useDocTree";

type Props = {
  items: VaultItem[];
  selected: string | null;
  onSelect: (path: string) => void;
};

export function DocTree({ items, selected, onSelect }: Props) {
  const { tree, collapsed, toggle, deepCount } = useDocTree(items);

  return (
    <div className="text-sm">
      {deepCount > 0 && (
        <div className="mx-3 mb-2 flex items-start gap-1 rounded bg-amber-50 px-2 py-1 text-[11px] text-amber-900">
          <AlertTriangle className="mt-0.5 size-3 shrink-0" />
          有 {deepCount} 篇超过 {MAX_DEPTH} 层——层级该重划了
        </div>
      )}
      {tree.length === 0 && <div className="px-3 py-1 text-xs text-muted-foreground">（还没有文档）</div>}
      {tree.map((root) => (
        <div key={root.path} className="mb-3">
          {/* 分组标题：与「数据表」那一组同款，不参与缩进 */}
          <div className="mb-1 px-3 text-xs font-semibold text-muted-foreground">{root.name}</div>
          {root.children.map((c) => (
            <Node key={c.path} node={c} collapsed={collapsed} onToggle={toggle} selected={selected} onSelect={onSelect} />
          ))}
        </div>
      ))}
    </div>
  );
}

function Node({
  node,
  collapsed,
  onToggle,
  selected,
  onSelect,
}: {
  node: TreeNode;
  collapsed: Set<string>;
  onToggle: (path: string) => void;
  selected: string | null;
  onSelect: (path: string) => void;
}) {
  // 分组标题已经占了第 1 层，所以缩进从第 2 层起算。
  const pad = { paddingLeft: `${(node.depth - 2) * 12 + 12}px` };

  if (node.kind === "folder") {
    const open = !collapsed.has(node.path);
    return (
      <div>
        <button
          type="button"
          onClick={() => onToggle(node.path)}
          style={pad}
          className="flex w-full items-center gap-1 rounded py-1 pr-2 text-left hover:bg-secondary/60"
        >
          {open ? <ChevronDown className="size-3 shrink-0" /> : <ChevronRight className="size-3 shrink-0" />}
          {open ? (
            <FolderOpen className="size-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <Folder className="size-3.5 shrink-0 text-muted-foreground" />
          )}
          <span className="truncate">{node.name}</span>
          <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">{countDocs(node)}</span>
        </button>
        {open &&
          node.children.map((c) => (
            <Node key={c.path} node={c} collapsed={collapsed} onToggle={onToggle} selected={selected} onSelect={onSelect} />
          ))}
      </div>
    );
  }

  const st = statusStyle(node.status ?? "draft");
  const active = node.path === selected;
  return (
    <button
      type="button"
      onClick={() => onSelect(node.path)}
      style={pad}
      title={node.path + (node.tooDeep ? "（超过三层）" : "")}
      className={"flex w-full items-center gap-2 rounded py-1 pr-2 text-left " + (active ? "bg-secondary" : "hover:bg-secondary/60")}
    >
      <span className="truncate">{node.name}</span>
      {node.tooDeep && <AlertTriangle className="size-3 shrink-0 text-amber-600" />}
      {/* 状态在行尾：标题先被看到 */}
      <span className={`ml-auto shrink-0 rounded px-1 text-[10px] leading-4 ${st.className}`}>{st.label}</span>
    </button>
  );
}

function countDocs(node: TreeNode): number {
  let n = 0;
  const walk = (nodes: TreeNode[]) => {
    for (const c of nodes) {
      if (c.kind === "doc") n++;
      walk(c.children);
    }
  };
  walk(node.children);
  return n;
}
