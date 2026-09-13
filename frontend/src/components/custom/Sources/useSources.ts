/**
 * 来源页的交互逻辑。
 *
 * 三条规格要求落在这里（docs/specs/document.spec.md）：
 *  1. 文档核验人必须是人，理由必填，方法只能是编审
 *  2. **变更传导必须说出来**——一次同步让多少条断言回到待核验，
 *     不告知人的批量回退就是最坏的那种静默
 *  3. 依据未登记的数量必须可见——「溯源的终点是一个字符串」正是要消灭的状态
 */
import { useCallback, useEffect, useRef } from "react";
import { toast } from "sonner";

import {
  History,
  List,
  Register,
  Reject,
  Unregistered,
  Usage,
  Verify,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/documentservice";
import type { DocInput, DocView } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useSourcesStore } from "./store";

export function useSources(scenario: string | null) {
  const s = useSourcesStore();

  const openRef = useRef<(revId: string) => Promise<void>>(async () => {});

  const reload = useCallback(async () => {
    s.setLoading(true);
    s.setError(null);
    try {
      // 场景名传空：来源是项目级的，换场景不该让文档列表变样
      const [docs, unregistered] = await Promise.all([List(""), Unregistered()]);
      const list = docs ?? [];
      s.setDocs(list);
      s.setUnregistered(unregistered ?? []);
      const still = list.some((d: DocView) => d.revId === s.selectedRevId);
      if (list.length === 0) {
        s.select("");
      } else if (!still) {
        // 选中第一份之后**必须把它的详情取回来**（引用与修订历史），
        // 否则详情区一直是空的——看起来像「这份文档没有断言引用」，
        // 而事实是「压根没去查」。
        await openRef.current(list[0].revId);
      }
    } catch (e) {
      s.setError(String(e));
    } finally {
      s.setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void reload();
  }, [reload, scenario]);

  /** 选中一份文档：同时取它的修订历史与引用它的断言。 */
  const open = useCallback(
    async (revId: string) => {
      s.select(revId);
      try {
        const [history, usage] = await Promise.all([History(revId), Usage(revId)]);
        s.setHistory(history ?? []);
        s.setUsage(usage ?? []);
      } catch (e) {
        s.setHistory([]);
        s.setUsage([]);
        toast.error(String(e));
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );
  openRef.current = open;

  const act = useCallback(
    async (fn: () => Promise<string>) => {
      const st = useSourcesStore.getState();
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

  const verify = useCallback(
    (revId: string) =>
      act(async () => {
        const st = useSourcesStore.getState();
        if (!st.by.trim()) throw new Error("必须填写核验人（必须是人）——无追责的核验等于没有核验");
        if (!st.reason.trim()) throw new Error("必须说明理由——只有状态没有理由的不是核验");
        // 方法固定为编审：一个人读过并背书，不构成多源
        await Verify(revId, st.by.trim(), "editorial", st.reason.trim());
        return "已核验这份原文";
      }),
    [act],
  );

  const reject = useCallback(
    (revId: string) =>
      act(async () => {
        const st = useSourcesStore.getState();
        if (!st.by.trim()) throw new Error("必须填写驳回人");
        if (!st.reason.trim()) throw new Error("必须说明理由");
        await Reject(revId, st.by.trim(), st.reason.trim());
        return "已驳回——条目保留，驳回是审计轨迹";
      }),
    [act],
  );

  /** 登记一份自撰文档（机制说明、依据声明）。 */
  const register = useCallback(
    (input: DocInput) =>
      act(async () => {
        const r = await Register(input);
        // 变更传导必须说出来
        if (r.changed && r.requeued > 0) {
          return `已登记新修订：${r.requeued} 条断言回到待核验`;
        }
        if (r.reverted) {
          return "已登记：修订标识回退了——这通常意味着来源出过问题，请核对";
        }
        return r.changed ? "已登记新修订" : "修订与内容都没变，未产生变更";
      }),
    [act],
  );

  return { ...s, reload, open, verify, reject, register };
}
