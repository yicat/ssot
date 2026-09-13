/**
 * 备选方案页的状态。
 *
 * 刻意**没有**「默认选中项」：系统不替使用者做取舍，
 * 一个预选的方案会让人以为那就是推荐。
 */
import { create } from "zustand";

import type {
  EvaluationView,
  PlanView,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type AlternativesStore = {
  plans: PlanView[];
  evaluation: EvaluationView | null;
  /** 使用者声明的偏好；空串表示未声明。 */
  preference: string;
  /** 必须满足的约束，逗号分隔。 */
  constraints: string;
  /** 合并阈值：差异低于它的方案会被合并，避免伪多样性。 */
  threshold: number;

  by: string;
  reason: string;

  loading: boolean;
  error: string | null;

  setPlans: (v: PlanView[]) => void;
  setEvaluation: (v: EvaluationView | null) => void;
  setPreference: (v: string) => void;
  setConstraints: (v: string) => void;
  setThreshold: (v: number) => void;
  setBy: (v: string) => void;
  setReason: (v: string) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

const initial = {
  plans: [],
  evaluation: null,
  preference: "",
  constraints: "",
  threshold: 0,
  by: "",
  reason: "",
  loading: false,
  error: null,
};

export const useAlternativesStore = create<AlternativesStore>((set) => ({
  ...initial,
  setPlans: (plans) => set({ plans }),
  setEvaluation: (evaluation) => set({ evaluation }),
  setPreference: (preference) => set({ preference }),
  setConstraints: (constraints) => set({ constraints }),
  setThreshold: (threshold) => set({ threshold }),
  setBy: (by) => set({ by }),
  setReason: (reason) => set({ reason }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

export const STATUS_CLASS: Record<string, string> = {
  candidate: "border-amber-300 text-amber-700",
  chosen: "border-emerald-300 text-emerald-700",
  stale: "border-orange-300 text-orange-700",
  invalid: "border-rose-300 text-rose-700",
  superseded: "border-slate-300 text-slate-500",
};

/** 把逗号分隔的约束串拆成列表。 */
export function parseConstraints(s: string): string[] {
  return s
    .split(/[,，\n]/)
    .map((x) => x.trim())
    .filter(Boolean);
}
