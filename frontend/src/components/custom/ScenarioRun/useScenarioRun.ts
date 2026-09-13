/**
 * 场景运行的交互逻辑。
 *
 * 三条规格要求落在这里（docs/specs/workspace.spec.md）：
 *  1. requires 未满足时**挡住运行**，并说明缺什么
 *  2. 外部输入的量纲由声明决定，单位不让手写——手写单位就是歧义制造机
 *  3. 运行失败时**保留已填输入**：那正是人最需要保留刚才填了什么的时候
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import { Run, RunSetup } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/scenarioservice";
import type { RefInput, RunInput } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useRunStore } from "./store";

export function useScenarioRun(scenario: string | null, subject: string) {
  const s = useRunStore();

  const load = useCallback(async () => {
    if (!scenario) {
      s.setSetup(null);
      return;
    }
    s.setLoading(true);
    s.setError(null);
    try {
      const next = await RunSetup(scenario, subject);
      s.setSetup(next);
      // 换了主体之后，引用候选变了：把已经不在候选里的旧选择清掉，
      // 否则会拿着 A 式神的技能去算 B 式神——那是一个算得出结果、
      // 但结果毫无意义的错误。
      const st = useRunStore.getState();
      const keep: Record<string, string> = {};
      for (const r of next.refs ?? []) {
        const chosen = st.refs[r.name];
        if (chosen && (r.candidates ?? []).includes(chosen)) {
          keep[r.name] = chosen;
        }
      }
      useRunStore.setState({ refs: keep, result: null });
    } catch (e) {
      s.setError(String(e));
    } finally {
      s.setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scenario, subject]);

  useEffect(() => {
    void load();
  }, [load]);

  const run = useCallback(async () => {
    const st = useRunStore.getState();
    const setup = st.setup;
    if (!setup || !scenario) return;
    if (!subject) {
      toast.error("先选一个主体");
      return;
    }
    if (!setup.runnable) {
      toast.error("requires 未满足，拒绝运行——不得用不完整数据产出方案");
      return;
    }
    for (const inp of setup.inputs ?? []) {
      if (!(st.values[inp.name] ?? "").trim()) {
        toast.error(`外部输入 ${inp.name} 还没填`);
        return;
      }
    }
    for (const ref of setup.refs ?? []) {
      if (!st.refs[ref.name]) {
        toast.error(`引用 ${ref.name} 还没选来源`);
        return;
      }
    }

    st.setLoading(true);
    st.setError(null);
    try {
      const inputs: RunInput[] = (setup.inputs ?? []).map((inp) => ({
        name: inp.name,
        value: (st.values[inp.name] ?? "").trim(),
        unit: inp.unit,
      }));
      const refs: RefInput[] = (setup.refs ?? []).map((r) => ({
        name: r.name,
        subject: st.refs[r.name] ?? "",
      }));
      const res = await Run(setup.scenario, subject, inputs, refs);
      st.setResult(res);
      if (res.ok) {
        toast.success(res.message);
      } else {
        // 失败也不清表单：人正要据原因改一个字段，不是重填一遍。
        toast.error(res.message);
      }
    } catch (e) {
      st.setError(String(e));
      toast.error(String(e));
    } finally {
      st.setLoading(false);
    }
  }, [scenario, subject]);

  return { ...s, load, run };
}
