/**
 * 「待判定」视图的状态。
 *
 * 只放状态与纯 setter：IO 在 useDecision.ts，渲染在 index.tsx。
 * 这不是形式主义——把三者混在一起时，最容易出现的问题是
 * 「刷新把用户刚做的选择抹掉」，而那正是人正在判断时最不能接受的。
 */
import { create } from "zustand";

import type { DecisionItem, DecisionStats } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

type DecisionState = {
  items: DecisionItem[];
  stats: DecisionStats | null;
  /** 当前展开的事项 ID；空串表示未选中 */
  selectedId: string;
  /** 当前勾选的候选序号；decision.None(-1) 表示「都不对」 */
  choice: number;
  loading: boolean;
  error: string | null;

  setItems: (items: DecisionItem[]) => void;
  setStats: (stats: DecisionStats | null) => void;
  select: (id: string) => void;
  setChoice: (choice: number) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
};

export const useDecisionStore = create<DecisionState>((set) => ({
  items: [],
  stats: null,
  selectedId: "",
  choice: 0,
  loading: false,
  error: null,

  setItems: (items) => set({ items }),
  setStats: (stats) => set({ stats }),
  select: (id) => set({ selectedId: id, choice: 0 }),
  setChoice: (choice) => set({ choice }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ items: [], stats: null, selectedId: "", choice: 0, error: null }),
}));

/** 当前选中的事项。 */
export function selectedItem(state: DecisionState): DecisionItem | null {
  return state.items.find((it) => it.id === state.selectedId) ?? null;
}
