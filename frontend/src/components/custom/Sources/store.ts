/**
 * 来源页的状态。
 *
 * 只放状态与纯 setter——取数在 useSources.ts。
 */
import { create } from "zustand";

import type {
  DocView,
  Item,
  UnregisteredView,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type SourcesStore = {
  docs: DocView[];
  unregistered: UnregisteredView[];
  selectedRevId: string;
  history: DocView[];
  usage: Item[];

  /**
   * 从断言跳过来、但没有对应文档的原件标题。
   *
   * **必须显示出来**：那个原件是被点开的依据，它不在列表里这件事
   * 只能说明「依据未登记」——不能让页面看起来像加载失败或点错了。
   */
  notFound: string | null;

  by: string;
  reason: string;

  loading: boolean;
  error: string | null;

  setDocs: (v: DocView[]) => void;
  setUnregistered: (v: UnregisteredView[]) => void;
  select: (revId: string) => void;
  setHistory: (v: DocView[]) => void;
  setUsage: (v: Item[]) => void;
  setNotFound: (v: string | null) => void;
  setBy: (v: string) => void;
  setReason: (v: string) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

const initial = {
  docs: [],
  unregistered: [],
  selectedRevId: "",
  history: [],
  usage: [],
  notFound: null,
  by: "",
  reason: "",
  loading: false,
  error: null,
};

export const useSourcesStore = create<SourcesStore>((set) => ({
  ...initial,
  setDocs: (docs) => set({ docs }),
  setUnregistered: (unregistered) => set({ unregistered }),
  select: (selectedRevId) => set({ selectedRevId }),
  setHistory: (history) => set({ history }),
  setUsage: (usage) => set({ usage }),
  setNotFound: (notFound) => set({ notFound }),
  setBy: (by) => set({ by }),
  setReason: (reason) => set({ reason }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

export const DOC_STATUS_CLASS: Record<string, string> = {
  unverified: "border-amber-300 text-amber-700",
  verified: "border-emerald-300 text-emerald-700",
  rejected: "border-slate-300 text-slate-500",
  superseded: "border-slate-300 text-slate-400",
};
