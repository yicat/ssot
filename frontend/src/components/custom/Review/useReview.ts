/**
 * 核验的交互逻辑。
 *
 * 三条规格要求落在这里（docs/specs/verification.spec.md）：
 *  1. 默认一切未核验——队列里每一条都带着状态与分级
 *  2. 排序理由必须可见——排在前面的原因要写出来，否则人只能盲信排序
 *  3. 核验必须能回答「谁、何时、凭什么」——批准者与理由为必填
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import {
  Approve,
  BatchApprove,
  BatchPreview,
  BatchReject,
  Conflicts,
  History,
  Queue,
  Reject,
  Stats,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/reviewservice";
import type { FilterInput, Item } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useReviewStore } from "./store";

export function useReview() {
  const s = useReviewStore();

  const refresh = useCallback(async () => {
    s.setLoading(true);
    try {
      const [stats, queue, conflicts] = await Promise.all([
        Stats(),
        Queue(s.entity, s.status, 300, s.sampleRatio),
        Conflicts(),
      ]);
      s.setStats(stats);
      s.setQueue(queue ?? []);
      s.setConflicts(conflicts ?? []);
      // 刻意不清错误：刷新发生在操作之后，清掉会把刚产生的提示抹掉
    } catch (e) {
      s.setError(String(e));
    } finally {
      s.setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [s.entity, s.status, s.sampleRatio]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const pick = useCallback(
    async (it: Item) => {
      s.setSelected(it);
      s.setError(null);
      try {
        s.setHistory((await History(it.id)) ?? []);
      } catch {
        s.setHistory([]);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const decide = useCallback(
    async (kind: "approve" | "reject") => {
      const sel = useReviewStore.getState().selected;
      if (!sel) return;
      const st = useReviewStore.getState();
      if (!st.by.trim()) {
        toast.error("必须填写批准者——无追责的核验等于没有核验");
        return;
      }
      if (!st.reason.trim()) {
        toast.error("必须说明理由——只有状态没有理由的不是核验");
        return;
      }
      st.setLoading(true);
      st.setError(null);
      try {
        if (kind === "approve") {
          await Approve(sel.id, st.by.trim(), st.method, st.reason.trim(), st.evidence.trim());
        } else {
          await Reject(sel.id, st.by.trim(), st.reason.trim());
        }
        toast.success(`${kind === "approve" ? "已批准" : "已驳回"}　${sel.subject}.${sel.predicate}`);
        st.setReason("");
        st.setEvidence("");
        st.setSelected(null);
        st.setHistory([]);
        await refresh();
      } catch (e) {
        st.setError(String(e));
        toast.error(String(e));
      } finally {
        st.setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh],
  );

  const buildFilter = useCallback((): FilterInput => {
    const st = useReviewStore.getState();
    return {
      entity: st.entity,
      status: st.status,
      predicate: st.batchPredicate.trim(),
      confidence: st.batchConfidence,
      artifact: "",
      revision: "",
      subject: "",
      limit: 0,
    } as FilterInput;
  }, []);

  const previewBatch = useCallback(async () => {
    const st = useReviewStore.getState();
    st.setError(null);
    try {
      st.setPreview(await BatchPreview(buildFilter()));
    } catch (e) {
      st.setError(String(e));
      toast.error(String(e));
    }
  }, [buildFilter]);

  const batch = useCallback(
    async (kind: "approve" | "reject") => {
      const st = useReviewStore.getState();
      if (!st.by.trim()) {
        toast.error("必须填写批准者");
        return;
      }
      if (!st.reason.trim()) {
        toast.error("批量核验同样需要理由");
        return;
      }
      st.setLoading(true);
      st.setError(null);
      try {
        const f = buildFilter();
        const r =
          kind === "approve"
            ? await BatchApprove(f, st.by.trim(), st.method, st.reason.trim(), st.evidence.trim())
            : await BatchReject(f, st.by.trim(), st.reason.trim());
        toast.success(`批量${kind === "approve" ? "批准" : "驳回"}：成功 ${r.applied}，失败 ${r.failed}`);
        st.setPreview(null);
        await refresh();
      } catch (e) {
        st.setError(String(e));
        toast.error(String(e));
      } finally {
        st.setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [refresh, buildFilter],
  );

  return { ...s, refresh, pick, decide, previewBatch, batch };
}

/** 状态的中文名与配色。 */
export const STATUS_LABEL: Record<string, string> = {
  pending: "待核验",
  verified: "已核验",
  rejected: "已驳回",
  disputed: "有争议",
  "auto-checked": "已自动预检",
};

export const STATUS_CLASS: Record<string, string> = {
  pending: "border-amber-300 text-amber-700",
  verified: "border-emerald-300 text-emerald-700",
  rejected: "border-rose-300 text-rose-700",
  disputed: "border-orange-300 text-orange-700",
  "auto-checked": "border-sky-300 text-sky-700",
};

/** 分级的中文说明。分级的意义是让隐式信息与直引事实可分辨。 */
export const CONFIDENCE_HINT: Record<string, string> = {
  L1: "直引：可与原文逐字比对",
  L2: "结构化：从文本解析得出",
  L3: "推导：由其他断言算出",
  L4: "推断：含补全的假设",
};
