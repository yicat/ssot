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
  /** 当前勾选的候选序号；-1 表示「都不对」 */
  choice: number;

  by: string;
  method: string;
  reason: string;
  evidence: string;

  loading: boolean;
  error: string | null;

  setItems: (items: DecisionItem[]) => void;
  setStats: (stats: DecisionStats | null) => void;
  select: (id: string) => void;
  setChoice: (choice: number) => void;
  setBy: (v: string) => void;
  setMethod: (v: string) => void;
  setReason: (v: string) => void;
  setEvidence: (v: string) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
};

const initial = {
  items: [],
  stats: null,
  selectedId: "",
  choice: 0,
  by: "",
  method: "editorial",
  reason: "",
  evidence: "",
  loading: false,
  error: null,
};

export const useDecisionStore = create<DecisionState>((set) => ({
  ...initial,
  setItems: (items) => set({ items }),
  setStats: (stats) => set({ stats }),
  select: (id) => set({ selectedId: id, choice: 0 }),
  setChoice: (choice) => set({ choice }),
  setBy: (by) => set({ by }),
  setMethod: (method) => set({ method }),
  setReason: (reason) => set({ reason }),
  setEvidence: (evidence) => set({ evidence }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

/** 当前选中的事项。 */
export function selectedItem(state: DecisionState): DecisionItem | null {
  return state.items.find((it) => it.id === state.selectedId) ?? null;
}
