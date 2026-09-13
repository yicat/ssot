/**
 * 经验页的交互逻辑。
 *
 * 三条规格要求落在这里（docs/specs/experience.spec.md）：
 *  1. 批准者必须是人，且理由必填——无追责的确认等于没有确认
 *  2. 会话记录**只能追加**：没有修改与删除的入口，因为它们不存在
 *  3. 责任级别与状态是**算出来的**，界面不提供修改入口
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import {
  AppendTurn,
  Approve,
  Conflicts,
  List,
  OpenSession,
  Propose,
  Refresh,
  Reject,
  Sessions,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/experienceservice";
import type { EntryView, ProposalInput } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useSessionStore } from "../Session/store";
import { useExperienceStore } from "./store";

export function useExperience(scenario: string | null) {
  const s = useExperienceStore();

  const reload = useCallback(async () => {
    if (!scenario) {
      s.setEntries([]);
      s.setConflicts([]);
      s.setSessions([]);
      return;
    }
    s.setLoading(true);
    s.setError(null);
    try {
      const [entries, conflicts, sessions] = await Promise.all([
        List(scenario),
        Conflicts(scenario),
        Sessions(scenario),
      ]);
      const list = entries ?? [];
      s.setEntries(list);
      s.setConflicts(conflicts ?? []);
      s.setSessions(sessions ?? []);
      const still = list.some((e: EntryView) => e.id === s.selectedId);
      if (!still) s.select(list.length > 0 ? list[0].id : "");
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
      const st = useExperienceStore.getState();
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

  const approve = useCallback(
    (id: string) =>
      act(async () => {
        const st = useExperienceStore.getState();
        if (!useSessionStore.getState().by.trim()) throw new Error("必须填写批准者——无追责的确认等于没有确认");
        if (!st.reason.trim()) throw new Error("必须说明理由——只有状态没有理由的不是确认");
        await Approve(id, useSessionStore.getState().by.trim(), st.reason.trim());
        return "已批准";
      }),
    [act],
  );

  const reject = useCallback(
    (id: string) =>
      act(async () => {
        const st = useExperienceStore.getState();
        if (!useSessionStore.getState().by.trim()) throw new Error("必须填写驳回人");
        if (!st.reason.trim()) throw new Error("必须说明理由");
        await Reject(id, useSessionStore.getState().by.trim(), st.reason.trim());
        return "已驳回——条目保留，驳回是审计轨迹";
      }),
    [act],
  );

  /** 追加一段发言。会话记录没有「修改」，只有「继续往下说」。 */
  const appendTurn = useCallback(
    (sessionID: string, roleKind: string) =>
      act(async () => {
        const st = useExperienceStore.getState();
        const text = st.draft.trim();
        if (!text) throw new Error("发言不能为空");
        const who = roleKind === "agent" ? "dsh" : useSessionStore.getState().by.trim();
        if (!who) throw new Error("必须指明身份——无追责的记录不是依据");
        await AppendTurn(sessionID, roleKind, who, text);
        st.setDraft("");
        return "已追加";
      }),
    [act],
  );

  const openSession = useCallback(
    (title: string) =>
      act(async () => {
        if (!scenario) throw new Error("先选一个场景");
        if (!title.trim()) throw new Error("会话要有标题");
        const v = await OpenSession(scenario, title.trim());
        useExperienceStore.getState().selectSession(v.id);
        return "已开一段会话记录";
      }),
    [act, scenario],
  );

  /** 提出候选经验。agent 与人都可以提，但都只能提到候选。 */
  const propose = useCallback(
    (proposal: ProposalInput) =>
      act(async () => {
        const e = await Propose(proposal);
        useExperienceStore.getState().select(e.id);
        return `已提出候选经验（责任级别 ${e.level}：${e.levelText}）`;
      }),
    [act],
  );

  /** 重算依赖：依赖被驳回则失效、依赖公式变更则待重算。 */
  const refresh = useCallback(
    () =>
      act(async () => {
        if (!scenario) throw new Error("先选一个场景");
        const r = await Refresh(scenario);
        const parts = [`检查 ${r.checked} 条`];
        if ((r.staled ?? []).length) parts.push(`失效 ${r.staled?.length ?? 0} 条`);
        if ((r.recompute ?? []).length) parts.push(`待重算 ${r.recompute?.length ?? 0} 条`);
        return parts.join("，");
      }),
    [act, scenario],
  );

  return { ...s, reload, approve, reject, appendTurn, openSession, propose, refresh };
}
