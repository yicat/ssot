/**
 * 词表：把原始标识符翻成看得懂的词。
 *
 * 界面此前到处是 `shikigami.atk`、`L2`、`percent`、`human:ngnl5` 这类标识符——
 * 准确但不可读。而定义里早就有中文（schema 的 description、单位名、枚举名），
 * 词表只是把它取出来。
 *
 * 两条原则：
 *   不编词 —— 定义里没有中文的，界面退回显示原始标识符
 *   双语并出 —— 中文名给人看，标识符给对账用
 */
import { create } from "zustand";

import type { Glossary } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

type GlossaryStore = {
  g: Glossary | null;
  loading: boolean;
  error: string | null;
  setG: (g: Glossary | null) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

export const useGlossaryStore = create<GlossaryStore>((set) => ({
  g: null,
  loading: false,
  error: null,
  setG: (g) => set({ g }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ g: null, error: null }),
}));

/** 一个词条的显示形式。 */
export type Named = {
  /** 中文名；定义里没有时为空串 */
  label: string;
  /** 原始标识符 */
  key: string;
  /** 完整说明 */
  note: string;
  /** 附加信息（单位、类型、目标实体） */
  extra: string;
  /** 中文名 + 标识符；没有中文名时只有标识符 */
  text: string;
};

const EMPTY: Named = { label: "", key: "", note: "", extra: "", text: "" };

function make(key: string, term?: { label?: string; note?: string; extra?: string } | null): Named {
  if (!key) return EMPTY;
  const label = term?.label ?? "";
  return {
    label,
    key,
    note: term?.note ?? "",
    extra: term?.extra ?? "",
    // 中文名在前、标识符在后：先看得懂，再对得上。
    text: label ? `${label}（${key}）` : key,
  };
}

/** 词表的取值方法。传 null 时全部退回原始标识符。 */
export function glossaryLookup(g: Glossary | null) {
  type TermMap = Record<string, { label?: string; note?: string; extra?: string } | undefined> | null | undefined;
  const term = (m: TermMap, k?: string) => (m && k ? m[k] : undefined);

  return {
    /** 字段：`shikigami.atk` → 攻击（shikigami.atk） */
    field: (entity: string, predicate: string): Named =>
      make(`${entity}.${predicate}`, term(g?.fields, `${entity}.${predicate}`)),
    /** 实体：`shikigami` → 式神（shikigami） */
    entity: (entity: string): Named => make(entity, term(g?.entities, entity)),
    /** 单位：`percent` → 百分比（percent） */
    unit: (unit: string): Named => make(unit, term(g?.units, unit)),
    /** 字段类型：`number` → 数值（number） */
    type: (t: string): Named => make(t, term(g?.types, t)),
    /** 分级：`L2` → 结构化（L2） */
    confidence: (c: string): Named => make(c, term(g?.confidences, c)),
    /** 断言状态 */
    status: (s: string): Named => make(s, term(g?.statuses, s)),
    /** 经验类型 */
    expKind: (k: string): Named => make(k, term(g?.expKinds, k)),
    /** 经验状态 */
    expStatus: (s: string): Named => make(s, term(g?.expStatuses, s)),
    /** 待判定状态 */
    decStatus: (s: string): Named => make(s, term(g?.decStatuses, s)),
    /** 方案状态 */
    planStatus: (s: string): Named => make(s, term(g?.planStatuses, s)),
    /** 参与者：`human:ngnl5` → 人：ngnl5 */
    actor: (kindID: string): string => {
      const [kind, ...rest] = (kindID ?? "").split(":");
      const id = rest.join(":");
      const k = term(g?.actorKinds, kind)?.label;
      if (!id) return kindID;
      return k ? `${k}：${id}` : kindID;
    },
    /** 主体：`shikigami/262` → 姑获鸟（262） */
    subject: (entity: string, subject: string): string => {
      const name = g?.subjects?.[`${entity}/${subject}`];
      const e = term(g?.entities, entity)?.label ?? entity;
      return name ? `${name}（${subject}）` : `${e} ${subject}`;
    },
    /** 依赖项：`assert:a1` → 断言 a1；`formula:scale@1` → 公式 scale@1 */
    dep: (d: string): string => {
      if (d.startsWith("assert:")) return `断言 ${d.slice("assert:".length)}`;
      if (d.startsWith("formula:")) return `公式 ${d.slice("formula:".length)}`;
      return d;
    },
    /** 修订标识：`revid:8053` → 修订 8053 */
    revision: (r: string): string =>
      r.startsWith("revid:") ? `修订 ${r.slice("revid:".length)}` : r,
    /** 参与计算的取值：`3082 point` → `3082 点数（point）` */
    quantity: (value: string, unit: string): string => {
      const u = make(unit, term(g?.units, unit));
      if (!unit) return value;
      return u.label ? `${value} ${u.text}` : `${value} ${unit}`;
    },
  };
}

/** 组件里取词表。 */
export function useGlossary() {
  const g = useGlossaryStore((s) => s.g);
  return glossaryLookup(g);
}
