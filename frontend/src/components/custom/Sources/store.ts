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

  by: string;
  reason: string;

  loading: boolean;
  error: string | null;

  setDocs: (v: DocView[]) => void;
  setUnregistered: (v: UnregisteredView[]) => void;
  select: (revId: string) => void;
  setHistory: (v: DocView[]) => void;
  setUsage: (v: Item[]) => void;
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
