/**
 * 「待判定」视图：原文里有多个说得通的取值，必须由人选。
 *
 * 界面的全部意义在于**让人能做出判断**，因此：
 *  - 每个候选都带原文片段，不必回去翻原件
 *  - 歧义所在的原文整段可见，判断依据摆在眼前
 *  - 「都不对」与「暂缓」是两个不同的按钮：前者是结论，后者是「先放着」
 *  - 已裁决的条目仍能看到当时没选的那些候选——「当时还有哪些说法」
 *    是可追溯性的一部分
 */
import { useState } from "react";

import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { DECISION_METHODS, type DecisionForm, useDecision } from "./useDecision";

const STATUS_CLASS: Record<string, string> = {
  open: "border-amber-500/40 text-amber-400",
  deferred: "border-slate-500/40 text-slate-400",
  decided: "border-emerald-500/40 text-emerald-400",
  stale: "border-rose-500/40 text-rose-400",
};

export function DecisionStatusBadge({ status, text }: { status: string; text: string }) {
  return (
    <Badge variant="outline" className={STATUS_CLASS[status] ?? ""}>
      {text}
    </Badge>
  );
}

export default function Decision({
  by,
  onByChange,
  onFlash,
}: {
  by: string;
  onByChange: (v: string) => void;
  onFlash: (f: { ok: boolean; text: string } | null) => void;
}) {
  const d = useDecision("", 500);
  const [method, setMethod] = useState("editorial");
  const [reason, setReason] = useState("");
  const [evidence, setEvidence] = useState("");

  const form: DecisionForm = { by, method, reason, evidence };
  const s = d.stats;

  const run = async (fn: () => Promise<string>) => {
    onFlash(null);
    try {
      onFlash({ ok: true, text: await fn() });
      setReason("");
      setEvidence("");
    } catch (e) {
      onFlash({ ok: false, text: String(e) });
    }
  };

  const it = d.selected;

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_30rem] gap-4">
      <section className="flex min-h-0 flex-col rounded-lg border border-border">
        <div className="flex items-center gap-3 border-b border-border px-3 py-2 text-xs text-muted-foreground">
          <span>待判定 {s?.open ?? 0} 条</span>
          <span>已暂缓 {s?.deferred ?? 0}</span>
          <span className={s?.stale ? "text-rose-400" : ""}>需复核 {s?.stale ?? 0}</span>
          <span>已裁决 {s?.decided ?? 0}</span>
          <span className={s?.missing ? "text-amber-400" : ""}>
            判为缺失 {s?.missing ?? 0}
          </span>
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            disabled={d.loading}
            onClick={() => void d.reload()}
          >
            刷新
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-background text-xs text-muted-foreground">
              <tr className="[&>th]:px-3 [&>th]:py-2 [&>th]:text-left [&>th]:font-medium">
                <th>主体</th>
                <th>谓词</th>
                <th>候选</th>
                <th>状态</th>
                <th>为什么排在前面</th>
              </tr>
            </thead>
            <tbody>
              {d.items.map((x) => (
                <tr
                  key={x.id}
                  onClick={() => d.select(x.id)}
                  className={
                    "cursor-pointer border-t border-border/60 [&>td]:px-3 [&>td]:py-1.5 hover:bg-accent/40 " +
                    (d.selectedId === x.id ? "bg-accent/60" : "")
                  }
                >
                  <td className="whitespace-nowrap">
                    {x.entity}/{x.subject}
                  </td>
                  <td className="whitespace-nowrap">{x.predicate}</td>
                  <td className="tabular-nums">{x.candidates?.length ?? 0}</td>
                  <td>
                    <DecisionStatusBadge status={x.status} text={x.statusText} />
                  </td>
                  <td className="text-xs text-muted-foreground">{x.impactReason}</td>
                </tr>
              ))}
              {d.items.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                    没有需要人看的待判定事项。
                    <div className="mt-2 text-xs">
                      这只说明「原文里没有多值歧义」，不说明数据已经完整。
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      <aside className="flex min-h-0 flex-col gap-3 overflow-auto">
        {!it ? (
          <div className="rounded-lg border border-border p-4 text-sm text-muted-foreground">
            从左侧选择一条待判定事项。
            <p className="mt-3 text-xs leading-relaxed">
              这类条目是**文本里有多个候选值、无法确定取哪一个**的项。工具在这里刻意不猜：
              猜错的值会带着「已抽取」的样子进入事实源，比缺失更危险。
            </p>
          </div>
        ) : (
          <>
            <div className="rounded-lg border border-border p-4">
              <div className="mb-2 flex items-center gap-2">
                <DecisionStatusBadge status={it.status} text={it.statusText} />
                <span className="text-xs text-muted-foreground">被引用 {it.impact} 次</span>
              </div>
              <div className="text-base font-medium">
                {it.subject}.{it.predicate}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">{it.reason}</div>
              <dl className="mt-3 space-y-1 border-t border-border pt-3 text-xs">
                <div className="flex gap-2">
                  <dt className="w-16 shrink-0 text-muted-foreground">原件</dt>
                  <dd className="break-all">{it.artifact}</dd>
                </div>
                <div className="flex gap-2">
                  <dt className="w-16 shrink-0 text-muted-foreground">修订</dt>
                  <dd>{it.revision}</dd>
                </div>
                <div className="flex gap-2">
                  <dt className="w-16 shrink-0 text-muted-foreground">来源</dt>
                  <dd>{it.source}</dd>
                </div>
              </dl>
            </div>

            <div className="rounded-lg border border-border p-4">
              <div className="mb-2 text-sm font-medium">歧义所在的原文</div>
              <p className="rounded bg-muted/40 p-2 text-xs leading-relaxed">{it.context}</p>
            </div>

            <div className="rounded-lg border border-border p-4">
              <div className="mb-2 text-sm font-medium">
                候选（{it.candidates?.length ?? 0} 个）
                <span className="ml-2 text-[11px] font-normal text-muted-foreground">
                  逐句核对后选一个——工具不替你选
                </span>
              </div>
              {it.candidates?.map((c) => {
                const active = d.choice === c.index && it.editable;
                return (
                  <button
                    key={c.index}
                    type="button"
                    disabled={!it.editable}
                    onClick={() => d.setChoice(c.index)}
                    className={
                      "mb-2 block w-full rounded border p-2 text-left " +
                      (active ? "border-emerald-500/60 bg-emerald-500/10" : "border-border") +
                      (it.editable ? " hover:bg-accent/40" : " opacity-70")
                    }
                  >
                    <div className="flex items-baseline gap-2">
                      <span className="text-xs text-muted-foreground">[{c.index}]</span>
                      <span className="text-base font-medium tabular-nums">
                        {c.value} {c.unit}
                      </span>
                      <span className="ml-auto text-[11px] text-muted-foreground">{c.anchor}</span>
                    </div>
                    <div className="mt-1 text-xs text-muted-foreground">{c.context}</div>
                    {c.note && <div className="mt-1 text-[11px] text-sky-300">{c.note}</div>}
                  </button>
                );
              })}
              {it.resolution && (
                <div className="mt-3 border-t border-border pt-3 text-xs">
                  <div>
                    裁决：
                    <b>
                      {it.resolution.choice === -1
                        ? "都不对（该谓词记为缺失）"
                        : it.resolution.chosenValue}
                    </b>
                  </div>
                  <div className="text-muted-foreground">
                    {it.resolution.by} · {it.resolution.method} · {it.resolution.at}
                  </div>
                  <div className="text-muted-foreground">理由：{it.resolution.reason}</div>
                  {it.resolution.evidence && (
                    <div className="text-muted-foreground">依据：{it.resolution.evidence}</div>
                  )}
                  {it.resolution.assertionId && (
                    <div className="mt-1 break-all font-mono text-[11px] text-muted-foreground">
                      断言 {it.resolution.assertionId}
                    </div>
                  )}
                </div>
              )}
            </div>

            {it.editable ? (
              <div className="rounded-lg border border-border p-4">
                <div className="mb-3 text-sm font-medium">裁决</div>
                <label className="mb-1 block text-xs text-muted-foreground">
                  裁决人（必须是人）
                </label>
                <Input
                  value={by}
                  onChange={(e) => onByChange(e.target.value)}
                  placeholder="你的名字"
                  className="mb-3 h-8"
                />
                <label className="mb-1 block text-xs text-muted-foreground">方法</label>
                <select
                  aria-label="裁决方法"
                  className="mb-1 h-8 w-full rounded-md border border-input bg-transparent px-2 text-sm"
                  value={method}
                  onChange={(e) => setMethod(e.target.value)}
                >
                  {DECISION_METHODS.map((m) => (
                    <option key={m.value} value={m.value}>
                      {m.label}
                    </option>
                  ))}
                </select>
                <p className="mb-3 text-[11px] text-muted-foreground">
                  {DECISION_METHODS.find((m) => m.value === method)?.hint}
                </p>
                {method === "measurement" && (
                  <>
                    <label className="mb-1 block text-xs text-muted-foreground">
                      依据（实测必填：版本、配置、样本数）
                    </label>
                    <Input
                      value={evidence}
                      onChange={(e) => setEvidence(e.target.value)}
                      placeholder="例：版本 2026-09；本体无增益；样本 3 次"
                      className="mb-3 h-8"
                    />
                  </>
                )}
                <label className="mb-1 block text-xs text-muted-foreground">理由（必填）</label>
                <Input
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="例：主伤害那一句是 88%，另一处是条件分支"
                  className="mb-3 h-8"
                />
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    className="flex-1"
                    disabled={d.loading || d.choice < 0}
                    onClick={() => void run(() => d.resolve(d.choice, form))}
                  >
                    裁决选中候选
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="flex-1 border-amber-500/40 text-amber-300"
                    disabled={d.loading}
                    onClick={() => void run(() => d.resolve(-1, form))}
                  >
                    都不对
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={d.loading}
                    onClick={() => void run(() => d.defer(form))}
                  >
                    暂缓
                  </Button>
                </div>
                <p className="mt-2 text-[11px] leading-relaxed text-muted-foreground">
                  「都不对」是**结论**：该谓词记为缺失，谁都别猜。「暂缓」是**未处理**：
                  条目仍留在队列里。两者不可混为一谈。
                </p>
              </div>
            ) : (
              <div className="rounded-lg border border-border p-4 text-xs text-muted-foreground">
                {it.status === "decided"
                  ? "已裁决。改判不是覆盖——需要以新的核验记录表达，当前版本不支持。"
                  : "该状态暂不可裁决。"}
              </div>
            )}
          </>
        )}
      </aside>
    </div>
  );
}
