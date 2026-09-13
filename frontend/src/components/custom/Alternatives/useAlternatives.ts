/**
 * 备选方案页的交互逻辑。
 *
 * 三条规格要求落在这里（docs/specs/alternatives.spec.md）：
 *  1. 未声明偏好时**不得**给出单一方案——后端会拒绝，界面照实呈现原因
 *  2. 选定必须由人做，且要留下理由
 *  3. 选定之后其他备选**仍在列表里**：偏好只影响顺序，不删除任何东西
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import {
  Choose,
  Evaluate,
  List,
  Propose,
  Refresh,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/alternativesservice";
import type { PlanProposalInput } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { parseConstraints, useAlternativesStore } from "./store";

export function useAlternatives(scenario: string | null) {
  const s = useAlternativesStore();

  const reload = useCallback(async () => {
    if (!scenario) {
      s.setPlans([]);
      s.setEvaluation(null);
      return;
    }
    s.setLoading(true);
    s.setError(null);
    try {
      s.setPlans((await List(scenario)) ?? []);
    } catch (e) {
      s.setError(String(e));
    } finally {
      s.setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scenario]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const act = useCallback(
    async (fn: () => Promise<string>) => {
      const st = useAlternativesStore.getState();
      st.setLoading(true);
      st.setError(null);
      try {
        const msg = await fn();
        await reload();
        toast.success(msg);
        st.setReason("");
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

  /** 对比：按偏好与约束整理出一组可并排看的方案。 */
  const compare = useCallback(
    () =>
      act(async () => {
        const st = useAlternativesStore.getState();
        if (!scenario) throw new Error("先选一个场景");
        const ev = await Evaluate(
          scenario,
          st.preference.trim(),
          parseConstraints(st.constraints),
          st.threshold,
        );
        st.setEvaluation(ev);
        if ((ev.constraints ?? []).length > 0) {
          // 约束打架是**信息**，不是错误：用成功提示说清它。
          return "没有方案满足全部约束——约束互相打架，清单已列出";
        }
        const kept = (ev.plans ?? []).length;
        const pruned = (ev.pruned ?? []).length;
        return `呈现 ${kept} 个备选${pruned > 0 ? `，剪掉 ${pruned} 个被支配的` : ""}`;
      }),
    [act, scenario],
  );

  const choose = useCallback(
    (id: string) =>
      act(async () => {
        const st = useAlternativesStore.getState();
        const who = st.by.trim();
        if (!who) throw new Error("必须填写选定人（必须是人）——系统不替使用者做取舍");
        if (!st.reason.trim()) throw new Error("必须说明理由——保留理由才能从历史里看出偏好");
        await Choose(id, who, st.reason.trim());
        return "已选定。其他备选仍然在列表里。";
      }),
    [act],
  );

  const refresh = useCallback(
    () =>
      act(async () => {
        if (!scenario) throw new Error("先选一个场景");
        const r = await Refresh(scenario);
        const parts = [`检查 ${r.checked} 个`];
        if ((r.invalid ?? []).length) parts.push(`不可执行 ${r.invalid?.length ?? 0} 个`);
        if ((r.stale ?? []).length) parts.push(`依据已变 ${r.stale?.length ?? 0} 个`);
        return parts.join("，");
      }),
    [act, scenario],
  );

  const propose = useCallback(
    (input: PlanProposalInput) =>
      act(async () => {
        const p = await Propose(input);
        return `已提出「${p.title}」（可信度上限 ${p.maxConfidence}）`;
      }),
    [act],
  );

  return { ...s, reload, compare, choose, refresh, propose };
}
