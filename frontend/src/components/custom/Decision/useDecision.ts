/**
 * 「待判定」的交互逻辑。
 *
 * 三条规格要求落在这里（docs/specs/decision.spec.md）：
 *  1. 暂缓不是结论——暂缓后事项**仍在队列里**（后端保证，界面照实呈现）
 *  2. 裁决必须能回答「谁、凭什么」——裁决人与理由为必填，界面先挡一道
 *  3. 「都不对」是一个**合法结论**，不是逃避；它必须和选一个候选一样好点
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import {
  DecisionStats,
  Decisions,
  DeferDecision,
  ResolveDecision,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/reviewservice";
import type { DecisionItem } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useSessionStore } from "../Session/store";
import { selectedItem, useDecisionStore } from "./store";

/** 界面上只允许这两种方法：在原文候选间取舍不是重算，也不是多源。 */
export const DECISION_METHODS = [
  { value: "editorial", label: "编审", hint: "读原文后判断——这是判断，不是验证" },
  { value: "measurement", label: "实测", hint: "游戏内量过，必须记录版本/配置/样本数" },
];

export function useDecision(status = "", limit = 500) {
  const s = useDecisionStore();

  const reload = useCallback(async () => {
    s.setLoading(true);
    s.setError(null);
    try {
      const [items, stats] = await Promise.all([Decisions(status, limit), DecisionStats()]);
      // Go 的 nil 切片序列化成 null，绑定层因此把数组标成可空。
      const list = items ?? [];
      s.setItems(list);
      s.setStats(stats);
      // 选中的那条可能已经不在此次筛选里了——此时清掉选择，
      // 免得右侧显示一条不属于当前队列的详情。
      const still = list.some((it: DecisionItem) => it.id === s.selectedId);
      if (!still) s.select(list.length > 0 ? list[0].id : "");
    } catch (e) {
      s.setError(String(e));
    } finally {
      s.setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status, limit]);

  useEffect(() => {
    void reload();
  }, [reload]);

  /** 裁决：choice >= 0 选中候选，-1 表示「都不对」。 */
  const resolve = useCallback(
    async (choice: number) => {
      const st = useDecisionStore.getState();
      const it = selectedItem(st);
      if (!it) return;
      const by = useSessionStore.getState().by.trim();
      if (!by) {
        toast.error("必须填写裁决人——无追责的裁决等于没有裁决");
        return;
      }
      if (!st.reason.trim()) {
        toast.error("必须说明理由——只有结论没有理由的不是裁决");
        return;
      }
      if (st.method === "measurement" && !st.evidence.trim()) {
        toast.error("以「实测」裁决时必须记录依据（版本、配置、样本数）");
        return;
      }
      st.setLoading(true);
      st.setError(null);
      try {
        const r = await ResolveDecision(
          it.id,
          choice,
          by,
          st.method,
          st.reason.trim(),
          st.evidence.trim(),
        );
        await reload();
        toast.success(r.message || "已裁决");
        st.setReason("");
        st.setEvidence("");
      } catch (e) {
        st.setError(String(e));
        toast.error(String(e));
      } finally {
        st.setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [reload],
  );

  const defer = useCallback(async () => {
    const st = useDecisionStore.getState();
    const it = selectedItem(st);
    if (!it) return;
    const by = useSessionStore.getState().by.trim();
      if (!by) {
      toast.error("必须填写裁决人");
      return;
    }
    if (!st.reason.trim()) {
      toast.error("必须说明理由——只说「先放着」不构成记录");
      return;
    }
    st.setLoading(true);
    st.setError(null);
    try {
      await DeferDecision(it.id, by, st.reason.trim());
      await reload();
      toast.success("已暂缓——它仍在待判定队列里，只是标为「人已看过、先放着」");
      st.setReason("");
    } catch (e) {
      st.setError(String(e));
      toast.error(String(e));
    } finally {
      st.setLoading(false);
    }
  }, [reload]);

  return { ...s, selected: selectedItem(s), reload, resolve, defer };
}
