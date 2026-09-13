/**
 * 经验页的状态。
 *
 * 会话记录的「当前段落草稿」也放这里：它是人正在写的一句话，
 * 切换条目不该把它抹掉——那正是人刚想到要记下来的东西。
 */
import { create } from "zustand";

import type {
  ConflictView,
  EntryView,
  SessionView,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type ExperienceStore = {
  entries: EntryView[];
  conflicts: ConflictView[];
  sessions: SessionView[];

  selectedId: string;
  sessionId: string;
  /** 追加发言的草稿 */
  draft: string;
  /** 批准/驳回理由。使用者身份是会话级的，不在这里。 */
  reason: string;

  loading: boolean;
  error: string | null;

  setEntries: (v: EntryView[]) => void;
  setConflicts: (v: ConflictView[]) => void;
  setSessions: (v: SessionView[]) => void;
  select: (id: string) => void;
  selectSession: (id: string) => void;
  setDraft: (v: string) => void;
  setReason: (v: string) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

const initial = {
  entries: [],
  conflicts: [],
  sessions: [],
  selectedId: "",
  sessionId: "",
  draft: "",
  reason: "",
  loading: false,
  error: null,
};

export const useExperienceStore = create<ExperienceStore>((set) => ({
  ...initial,
  setEntries: (entries) => set({ entries }),
  setConflicts: (conflicts) => set({ conflicts }),
  setSessions: (sessions) => set({ sessions }),
  select: (selectedId) => set({ selectedId }),
  selectSession: (sessionId) => set({ sessionId }),
  setDraft: (draft) => set({ draft }),
  setReason: (reason) => set({ reason }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

/** 责任级别的展示名。级别是算出来的，界面只显示，不提供修改入口。 */
export const LEVEL_CLASS: Record<number, string> = {
  1: "border-emerald-300 text-emerald-700",
  2: "border-sky-300 text-sky-700",
  3: "border-amber-300 text-amber-700",
  4: "border-rose-300 text-rose-700",
};

export const STATUS_CLASS: Record<string, string> = {
  candidate: "border-amber-300 text-amber-700",
  effective: "border-emerald-300 text-emerald-700",
  rejected: "border-slate-300 text-slate-500",
  stale: "border-rose-300 text-rose-700",
  recompute: "border-orange-300 text-orange-700",
  superseded: "border-slate-300 text-slate-500",
};

export const KINDS = [
  { value: "derived", label: "推导型", hint: "按公式从事实算出，算例可复现" },
  { value: "judgment", label: "判定型", hint: "人工裁定，依据可查、可推翻" },
  { value: "summary", label: "总结型", hint: "从多次实践归纳——必须带样本量" },
];
