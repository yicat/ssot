/**
 * 场景运行页的状态。
 *
 * 表单值单独存：运行失败时**不得清空已填的输入**——那正是人最需要
 * 保留刚才填了什么的时候。
 */
import { create } from "zustand";

import type { RunResult, RunSetup } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type RunStore = {
  setup: RunSetup | null;
  result: RunResult | null;
  /** 外部输入：名 → 值（单位按声明补全，不让人手写） */
  values: Record<string, string>;
  /** 引用：绑定名 → 来源主体 */
  refs: Record<string, string>;
  loading: boolean;
  error: string | null;

  setSetup: (v: RunSetup | null) => void;
  setResult: (v: RunResult | null) => void;
  setValue: (name: string, v: string) => void;
  setRef: (name: string, v: string) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  /** 只清结果与错误，保留表单 */
  clearResult: () => void;
  reset: () => void;
};

export const useRunStore = create<RunStore>((set) => ({
  setup: null,
  result: null,
  values: {},
  refs: {},
  loading: false,
  error: null,

  setSetup: (setup) => set({ setup }),
  setResult: (result) => set({ result }),
  setValue: (name, v) => set((s) => ({ values: { ...s.values, [name]: v } })),
  setRef: (name, v) => set((s) => ({ refs: { ...s.refs, [name]: v } })),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  clearResult: () => set({ result: null, error: null }),
  reset: () => set({ setup: null, result: null, values: {}, refs: {}, error: null }),
}));
