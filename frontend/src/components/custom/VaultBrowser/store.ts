/**
 * VaultBrowser 的状态（交互逻辑在 useVaultBrowser.ts，渲染在 index.tsx）。
 *
 * 状态与交互分开的理由：渲染只读 store，异步与副作用都在 hook 里，
 * 这样出问题时能一眼看出是「状态不对」还是「逻辑没跑」。
 */
import { create } from "zustand";

import type {
  VaultBacklinkResult,
  VaultDoc,
  VaultHit,
  VaultItem,
  VaultTableInfo,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type VaultState = {
  /** vault 根目录（由后端返回，用来显示「现在在看哪个项目」）。 */
  root: string;
  /** 左栏：全部文档。 */
  items: VaultItem[];
  /** 左栏：数据表（文件清单）。 */
  tables: string[];
  /** 数据表（含索引里推断出来的列与行数）。 */
  tableInfos: VaultTableInfo[];
  /**
   * 数据表的行数据，按来源文件（`tables/xxx.csv`）索引。
   *
   * 预取而不是渲染时现取：文档里的 `![[表.csv]]` 是**同步渲染**的，
   * 渲染函数不该等 IO——不然整篇文档会跳一下才出来。
   */
  tableData: Record<string, { columns: string[]; rows: string[][] }>;
  /** 是否显示 `%%注释%%`（默认不显示：注释是给自己看的）。 */
  showComments: boolean;
  /** 检索词（弹窗里当前输入的）。 */
  query: string;
  /** 检索弹窗是否打开。 */
  searchOpen: boolean;
  /** 弹窗里选中的第几条（键盘上下键用）。 */
  searchCursor: number;
  /** 检索结果；`null` 表示当前没在检索（左栏显示全部文档）。 */
  hits: VaultHit[] | null;
  /** 当前选中的文档路径。 */
  selected: string | null;
  /** 当前在看的**数据表**（相对路径）；非空时主体区显示表而不是文档。 */
  selectedTable: string | null;
  /**
   * 主体区显示哪个模式：`doc` / `table` / `agent`。
   *
   * 显式存一个 mode（而不是靠 selectedTable 推）：切到 Agent 之后再切回来，
   * 要回到原来那篇文档——靠推导就会丢掉「刚才在看什么」。
   */
  mode: "doc" | "table" | "agent";
  /** 配置弹窗是否打开。 */
  settingsOpen: boolean;
  /** 右栏：当前文档。 */
  doc: VaultDoc | null;
  /** 右栏：当前文档的反链与问题链接。 */
  links: VaultBacklinkResult | null;
  /** 一句话提示：错误、或「块锚点跳到第几行」这类结果。 */
  notice: string | null;
  /** 提示是错误还是普通信息（决定用不用红色）。 */
  noticeIsError: boolean;
  busy: boolean;
  set: (patch: Partial<VaultState>) => void;
};

export const useVaultStore = create<VaultState>((set) => ({
  root: "",
  items: [],
  tables: [],
  tableInfos: [],
  tableData: {},
  showComments: false,
  query: "",
  searchOpen: false,
  searchCursor: 0,
  hits: null,
  selected: null,
  selectedTable: null,
  mode: "doc",
  settingsOpen: false,
  doc: null,
  links: null,
  notice: null,
  noticeIsError: false,
  busy: false,
  set: (patch) => set(patch),
}));

