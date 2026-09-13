/**
 * 数据页的状态。
 *
 * 三个视图共用一份筛选条件：断言库、质量、schema。
 * 分开存会导致「在质量页看到 skill 的缺口，切到断言库却看到全部」——
 * 而人切换视图的目的恰恰是接着上一眼继续看。
 */
import { create } from "zustand";

import type {
  AssertionPage,
  EntityQuality,
  EntitySchema,
  Item,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type DataStore = {
  page: AssertionPage | null;
  quality: EntityQuality[];
  schemas: EntitySchema[];
  detail: Item[];

  entity: string;
  status: string;
  predicate: string;
  confidence: string;
  selected: { entity: string; subject: string } | null;

  loading: boolean;
  error: string | null;

  setPage: (v: AssertionPage | null) => void;
  setQuality: (v: EntityQuality[]) => void;
  setSchemas: (v: EntitySchema[]) => void;
  setDetail: (v: Item[]) => void;
  setEntity: (v: string) => void;
  setStatus: (v: string) => void;
  setPredicate: (v: string) => void;
  setConfidence: (v: string) => void;
  setSelected: (v: { entity: string; subject: string } | null) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

const initial = {
  page: null,
  quality: [],
  schemas: [],
  detail: [],
  entity: "",
  status: "",
  predicate: "",
  confidence: "",
  selected: null,
  loading: false,
  error: null,
};

export const useDataStore = create<DataStore>((set) => ({
  ...initial,
  setPage: (page) => set({ page }),
  setQuality: (quality) => set({ quality }),
  setSchemas: (schemas) => set({ schemas }),
  setDetail: (detail) => set({ detail }),
  setEntity: (entity) => set({ entity }),
  setStatus: (status) => set({ status }),
  setPredicate: (predicate) => set({ predicate }),
  setConfidence: (confidence) => set({ confidence }),
  setSelected: (selected) => set({ selected }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

/** 一页取多少条。分页是必须的：7792 条一次性渲染会让界面卡住。 */
export const PAGE_SIZE = 100;
