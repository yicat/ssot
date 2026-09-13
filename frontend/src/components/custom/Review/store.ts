/**
 * 核验（队列 / 冲突）的状态。
 *
 * 表单字段放这里而不是组件里：批准者、方法、理由在「核验」整块里是共享的，
 * 选一条断言前后不该被清空——那正是人刚要开始填的时候。
 */
import { create } from "zustand";

import type {
  BatchPreview,
  ConflictGroup,
  HistoryItem,
  Item,
  QueueItem,
  Stats,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

export type ReviewState = {
  stats: Stats | null;
  queue: QueueItem[];
  conflicts: ConflictGroup[];
  selected: Item | null;
  history: HistoryItem[];

  entity: string;
  status: string;
  sampleRatio: number;

  method: string;
  reason: string;
  evidence: string;

  batchPredicate: string;
  batchConfidence: string;
  preview: BatchPreview | null;

  loading: boolean;
  error: string | null;

  setStats: (v: Stats | null) => void;
  setQueue: (v: QueueItem[]) => void;
  setConflicts: (v: ConflictGroup[]) => void;
  setSelected: (v: Item | null) => void;
  setHistory: (v: HistoryItem[]) => void;
  setEntity: (v: string) => void;
  setStatus: (v: string) => void;
  setSampleRatio: (v: number) => void;
  setMethod: (v: string) => void;
  setReason: (v: string) => void;
  setEvidence: (v: string) => void;
  setBatchPredicate: (v: string) => void;
  setBatchConfidence: (v: string) => void;
  setPreview: (v: BatchPreview | null) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

const initial = {
  stats: null,
  queue: [],
  conflicts: [],
  selected: null,
  history: [],
  entity: "",
  status: "pending",
  sampleRatio: 0.05,
  method: "editorial",
  reason: "",
  evidence: "",
  batchPredicate: "",
  batchConfidence: "",
  preview: null,
  loading: false,
  error: null,
};

export const useReviewStore = create<ReviewState>((set) => ({
  ...initial,
  setStats: (stats) => set({ stats }),
  setQueue: (queue) => set({ queue }),
  setConflicts: (conflicts) => set({ conflicts }),
  setSelected: (selected) => set({ selected }),
  setHistory: (history) => set({ history }),
  setEntity: (entity) => set({ entity }),
  setStatus: (status) => set({ status }),
  setSampleRatio: (sampleRatio) => set({ sampleRatio }),
  setMethod: (method) => set({ method }),
  setReason: (reason) => set({ reason }),
  setEvidence: (evidence) => set({ evidence }),
  setBatchPredicate: (batchPredicate) => set({ batchPredicate }),
  setBatchConfidence: (batchConfidence) => set({ batchConfidence }),
  setPreview: (preview) => set({ preview }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

/** 界面上只允许这四种方法；方法不同，可信度不同。 */
export const METHODS = [
  { value: "editorial", label: "编审", hint: "人工阅读后判断——这是判断，不是验证" },
  { value: "cross-source", label: "多源", hint: "多个独立来源一致" },
  { value: "recompute", label: "重算", hint: "由其他已核验断言推导" },
  { value: "measurement", label: "实测", hint: "游戏内观测，必须记录版本/配置/样本数" },
];
