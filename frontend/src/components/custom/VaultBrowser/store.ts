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
  /** 左栏：数据表（含索引里推断出来的列与行数）。 */
  tableInfos: VaultTableInfo[];
  /** 检索框里的词。 */
  query: string;
  /** 检索结果；`null` 表示当前没在检索（左栏显示全部文档）。 */
  hits: VaultHit[] | null;
  /** 当前选中的文档路径。 */
  selected: string | null;
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
  query: "",
  hits: null,
  selected: null,
  doc: null,
  links: null,
  notice: null,
  noticeIsError: false,
  busy: false,
  set: (patch) => set(patch),
}));
