/**
 * 文件树：vault 的导航（构建与折叠逻辑见 useDocTree.ts）。
 *
 * 只读 store/hook 给的东西，不含别的逻辑。层级、排序、超层提示的规则
 * 见 docs/specs/document.spec.md 第二节。
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
    <div className="px-2 py-1 text-sm">
      {deepCount > 0 && (
        <div className="mb-1 flex items-start gap-1 rounded bg-amber-50 px-2 py-1 text-[11px] text-amber-900">
          <AlertTriangle className="mt-0.5 size-3 shrink-0" />
          有 {deepCount} 篇超过 {MAX_DEPTH} 层——层级该重划了
        </div>
      )}
      {tree.length === 0 && <div className="px-2 py-1 text-xs text-muted-foreground">（还没有文档）</div>}
      {tree.map((node) => (
        <Node key={node.path} node={node} collapsed={collapsed} onToggle={toggle} selected={selected} onSelect={onSelect} />
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
  const pad = { paddingLeft: `${(node.depth - 1) * 12 + 8}px` };

  if (node.kind === "folder") {
    const open = !collapsed.has(node.path);
    return (
      <div>
        <button
          type="button"
          onClick={() => onToggle(node.path)}
          style={pad}
          className="flex w-full items-center gap-1 rounded px-1 py-1 text-left hover:bg-secondary/60"
        >
          {open ? <ChevronDown className="size-3 shrink-0" /> : <ChevronRight className="size-3 shrink-0" />}
          {open ? <FolderOpen className="size-3.5 shrink-0 text-muted-foreground" /> : <Folder className="size-3.5 shrink-0 text-muted-foreground" />}
          <span className="truncate font-medium">{node.name}</span>
          <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">{countDocs(node)}</span>
        </button>
        {open && node.children.map((c) => <Node key={c.path} node={c} collapsed={collapsed} onToggle={onToggle} selected={selected} onSelect={onSelect} />)}
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
      className={"flex w-full items-center gap-2 rounded px-1 py-1 text-left " + (active ? "bg-secondary" : "hover:bg-secondary/60")}
    >
      <span className={`shrink-0 rounded px-1 text-[10px] leading-4 ${st.className}`}>{st.label}</span>
      <span className="truncate">{node.name}</span>
      {node.tooDeep && <AlertTriangle className="size-3 shrink-0 text-amber-600" />}
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
