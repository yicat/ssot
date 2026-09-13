/**
 * 核验工作台。
 *
 * 它是本项目里**人唯一需要介入的地方**：数据由程序接入与准入，
 * 人在这里判断什么算数。
 *
 * 界面遵循四条规格要求：
 *  1. 一切默认未核验——队列里每一条都带着状态与分级，未核验不会被藏起来
 *  2. **排序理由必须可见**——排在前面的原因要写出来，否则人只能盲信排序
 *  3. 冲突成组呈现，且**系统不裁决**——只把同一件事的说法摆在一起
 *  4. 核验必须能回答「谁、何时、凭什么」——批准者与理由为必填
 */
import { useCallback, useEffect, useState } from "react";

import {
  Approve,
  BatchApprove,
  BatchPreview,
  BatchReject,
  Conflicts,
  DecisionStats,
  History,
  ProjectDir,
  Queue,
  Reject,
  Stats,
} from "../bindings/github.com/ngnl5/ssot/internal/api/reviewservice";
import type {
  BatchPreview as BatchPreviewData,
  ConflictGroup,
  DecisionStats as DecisionStatsData,
  FilterInput,
  HistoryItem,
  Item,
  QueueItem,
  Stats as StatsData,
} from "../bindings/github.com/ngnl5/ssot/internal/api/models";

import Decision from "./components/custom/Decision";
import { Badge } from "./components/ui/badge";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";

const METHODS = [
  { value: "editorial", label: "编审", hint: "人工阅读后判断——这是判断，不是验证" },
  { value: "cross-source", label: "多源", hint: "多个独立来源一致" },
  { value: "recompute", label: "重算", hint: "由其他已核验断言推导" },
  { value: "measurement", label: "实测", hint: "游戏内观测，必须记录版本/配置/样本数" },
];

const STATUS_LABEL: Record<string, string> = {
  pending: "待核验",
  verified: "已核验",
  rejected: "已驳回",
  disputed: "有争议",
  "auto-checked": "已自动预检",
};

function StatusBadge({ status }: { status: string }) {
  const cls: Record<string, string> = {
    pending: "border-amber-500/40 text-amber-400",
    verified: "border-emerald-500/40 text-emerald-400",
    rejected: "border-rose-500/40 text-rose-400",
    disputed: "border-orange-500/40 text-orange-400",
    "auto-checked": "border-sky-500/40 text-sky-400",
  };
  return (
    <Badge variant="outline" className={cls[status] ?? ""}>
      {STATUS_LABEL[status] ?? status}
    </Badge>
  );
}

function ConfBadge({ c }: { c: string }) {
  const hint: Record<string, string> = {
    L1: "直引：可与原文逐字比对",
    L2: "结构化：从文本解析得出",
    L3: "推导：由其他断言算出",
    L4: "推断：含补全的假设",
  };
  return (
    <Badge variant="outline" title={hint[c] ?? ""} className="border-slate-500/40 text-slate-300">
      {c}
    </Badge>
  );
}

function TierBadge({ q }: { q: QueueItem }) {
  const cls: Record<string, string> = {
    争议: "border-orange-500/40 text-orange-300",
    影响面: "border-sky-500/40 text-sky-300",
    分级: "border-violet-500/40 text-violet-300",
    抽检: "border-teal-500/40 text-teal-300",
    常规: "border-slate-500/30 text-slate-400",
  };
  return (
    <span className="inline-flex items-center gap-1">
      <Badge variant="outline" className={cls[q.tier] ?? ""}>
        {q.tier}
      </Badge>
      {q.sampled && q.tier !== "抽检" && (
        <Badge variant="outline" className="border-teal-500/40 text-teal-300">
          抽检
        </Badge>
      )}
    </span>
  );
}

export default function App() {
  const [view, setView] = useState<"queue" | "conflicts" | "decision">("queue");
  const [dir, setDir] = useState("");
  const [stats, setStats] = useState<StatsData | null>(null);
  const [dstats, setDstats] = useState<DecisionStatsData | null>(null);
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [conflicts, setConflicts] = useState<ConflictGroup[]>([]);
  const [sel, setSel] = useState<Item | null>(null);
  const [hist, setHist] = useState<HistoryItem[]>([]);

  const [entity, setEntity] = useState("");
  const [status, setStatus] = useState("pending");
  const [sampleRatio, setSampleRatio] = useState(0.05);

  const [by, setBy] = useState("");
  const [method, setMethod] = useState("editorial");
  const [reason, setReason] = useState("");
  const [evidence, setEvidence] = useState("");

  const [bPred, setBPred] = useState("");
  const [bConf, setBConf] = useState("");
  const [preview, setPreview] = useState<BatchPreviewData | null>(null);

  const [flash, setFlash] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const [s, q, c, d] = await Promise.all([
        Stats(),
        Queue(entity, status, 300, sampleRatio),
        Conflicts(),
        DecisionStats(),
      ]);
      setStats(s);
      // Go 的 nil 切片序列化成 null，绑定层因此把数组标成可空——
      // 这里补成空数组，免得后面到处判空。
      setQueue(q ?? []);
      setConflicts(c ?? []);
      setDstats(d);
      // 刻意不清 flash：刷新发生在操作之后，清掉会把刚产生的提示抹掉
    } catch (e) {
      setFlash({ ok: false, text: String(e) });
    } finally {
      setBusy(false);
    }
  }, [entity, status, sampleRatio]);

  useEffect(() => {
    ProjectDir().then(setDir).catch(() => {});
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const pick = async (it: Item) => {
    setSel(it);
    setFlash(null);
    try {
      setHist((await History(it.id)) ?? []);
    } catch {
      setHist([]);
    }
  };

  const act = async (kind: "approve" | "reject") => {
    if (!sel) return;
    setBusy(true);
    setFlash(null);
    try {
      if (kind === "approve") {
        await Approve(sel.id, by.trim(), method, reason.trim(), evidence.trim());
      } else {
        await Reject(sel.id, by.trim(), reason.trim());
      }
      setFlash({
        ok: true,
        text: `${kind === "approve" ? "已批准" : "已驳回"}　${sel.subject}.${sel.predicate}`,
      });
      setReason("");
      setEvidence("");
      setSel(null);
      setHist([]);
      await refresh();
    } catch (e) {
      setFlash({ ok: false, text: String(e) });
    } finally {
      setBusy(false);
    }
  };

  const buildFilter = (): FilterInput =>
    ({
      entity,
      status,
      predicate: bPred.trim(),
      confidence: bConf,
      artifact: "",
      revision: "",
      subject: "",
      limit: 0,
    }) as FilterInput;

  const doPreview = async () => {
    setFlash(null);
    try {
      setPreview(await BatchPreview(buildFilter()));
    } catch (e) {
      setFlash({ ok: false, text: String(e) });
    }
  };

  const doBatch = async (kind: "approve" | "reject") => {
    setBusy(true);
    setFlash(null);
    try {
      const f = buildFilter();
      const r =
        kind === "approve"
          ? await BatchApprove(f, by.trim(), method, reason.trim(), evidence.trim())
          : await BatchReject(f, by.trim(), reason.trim());
      setFlash({
        ok: true,
        text: `批量${kind === "approve" ? "批准" : "驳回"}：成功 ${r.applied}，失败 ${r.failed}`,
      });
      setPreview(null);
      await refresh();
    } catch (e) {
      setFlash({ ok: false, text: String(e) });
    } finally {
      setBusy(false);
    }
  };

  const byStatus = stats?.byStatus ?? {};

  return (
    <div className="dark min-h-screen bg-background text-foreground">
      <div className="mx-auto flex h-screen max-w-[1600px] flex-col gap-3 p-4">
        <header className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-border pb-3">
          <div>
            <h1 className="text-lg font-semibold">核验工作台</h1>
            <p className="text-xs text-muted-foreground">{dir || "（未连接）"}</p>
          </div>
          <div className="flex flex-wrap items-center gap-4 text-sm">
            <span>
              断言 <b className="tabular-nums">{stats?.total ?? 0}</b>
            </span>
            <span className="text-amber-400">
              待核验 <b className="tabular-nums">{byStatus["pending"] ?? 0}</b>
            </span>
            <span className="text-emerald-400">
              已核验 <b className="tabular-nums">{byStatus["verified"] ?? 0}</b>
            </span>
            <span className={conflicts.length ? "text-orange-400" : "text-muted-foreground"}>
              冲突 <b className="tabular-nums">{conflicts.length}</b>
            </span>
            <span className={dstats?.open ? "text-sky-400" : "text-muted-foreground"}>
              待判定 <b className="tabular-nums">{dstats?.open ?? 0}</b>
            </span>
            <span className="text-muted-foreground">
              核验记录 <b className="tabular-nums">{stats?.verifications ?? 0}</b>
            </span>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <div className="flex rounded-md border border-border text-xs">
              {(["queue", "conflicts", "decision"] as const).map((v) => (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  className={
                    "px-3 py-1.5 " +
                    (view === v ? "bg-accent text-accent-foreground" : "text-muted-foreground")
                  }
                >
                  {v === "queue" ? "队列" : v === "conflicts" ? "冲突" : "待判定"}
                  {v === "decision" && (dstats?.open ?? 0) > 0 && (
                    <span className="ml-1 tabular-nums text-sky-400">{dstats?.open}</span>
                  )}
                </button>
              ))}
            </div>
            <select
              aria-label="实体筛选"
              className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
              value={entity}
              onChange={(e) => setEntity(e.target.value)}
            >
              <option value="">全部实体</option>
              {(Object.keys(stats?.byEntity ?? {}) as string[]).map((k) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
            </select>
            <select
              aria-label="状态筛选"
              className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
              value={status}
              onChange={(e) => setStatus(e.target.value)}
            >
              <option value="pending">待核验</option>
              <option value="verified">已核验</option>
              <option value="rejected">已驳回</option>
              <option value="disputed">有争议</option>
            </select>
            <Button size="sm" variant="outline" onClick={() => void refresh()} disabled={busy}>
              刷新
            </Button>
          </div>
        </header>

        {flash && (
          <div
            className={
              "rounded-md border px-3 py-2 text-sm " +
              (flash.ok
                ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300"
                : "border-rose-500/40 bg-rose-500/10 text-rose-300")
            }
          >
            {flash.text}
          </div>
        )}

        {view === "decision" ? (
          <Decision by={by} onByChange={setBy} onFlash={setFlash} />
        ) : (
        <div className="grid min-h-0 flex-1 grid-cols-[1fr_26rem] gap-4">
          <section className="flex min-h-0 flex-col rounded-lg border border-border">
            {view === "queue" ? (
              <>
                <div className="flex items-center gap-3 border-b border-border px-3 py-2 text-xs text-muted-foreground">
                  <span>队列 {queue.length} 条</span>
                  <label className="ml-auto flex items-center gap-1">
                    强制抽检
                    <select
                      aria-label="抽检比例"
                      className="rounded border border-input bg-transparent px-1 py-0.5"
                      value={sampleRatio}
                      onChange={(e) => setSampleRatio(Number(e.target.value))}
                    >
                      <option value={0}>0%</option>
                      <option value={0.02}>2%</option>
                      <option value={0.05}>5%</option>
                      <option value={0.1}>10%</option>
                    </select>
                  </label>
                </div>
                <div className="min-h-0 flex-1 overflow-auto">
                  <table className="w-full text-sm">
                    <thead className="sticky top-0 bg-background text-xs text-muted-foreground">
                      <tr className="[&>th]:px-3 [&>th]:py-2 [&>th]:text-left [&>th]:font-medium">
                        <th>主体</th>
                        <th>谓词</th>
                        <th>取值</th>
                        <th>分级</th>
                        <th>类别</th>
                        <th>主导理由</th>
                      </tr>
                    </thead>
                    <tbody>
                      {queue.map((q) => (
                        <tr
                          key={q.item.id}
                          onClick={() => void pick(q.item)}
                          className={
                            "cursor-pointer border-t border-border/60 [&>td]:px-3 [&>td]:py-1.5 hover:bg-accent/40 " +
                            (sel?.id === q.item.id ? "bg-accent/60" : "")
                          }
                        >
                          <td className="whitespace-nowrap">
                            {q.item.entity}/{q.item.subject}
                          </td>
                          <td className="whitespace-nowrap">{q.item.predicate}</td>
                          <td className="max-w-[16rem] truncate">{q.item.value}</td>
                          <td>
                            <ConfBadge c={q.item.confidence} />
                          </td>
                          <td>
                            <TierBadge q={q} />
                          </td>
                          <td className="text-xs text-muted-foreground">{q.reason}</td>
                        </tr>
                      ))}
                      {queue.length === 0 && (
                        <tr>
                          <td colSpan={6} className="px-3 py-8 text-center text-muted-foreground">
                            该筛选下没有断言
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <>
                <div className="border-b border-border px-3 py-2 text-xs text-muted-foreground">
                  {conflicts.length} 组冲突 —— **系统不替你裁决**，只把同一件事的说法摆在一起
                </div>
                <div className="min-h-0 flex-1 overflow-auto p-3">
                  {conflicts.length === 0 && (
                    <p className="py-8 text-center text-sm text-muted-foreground">
                      未发现冲突。这只说明「没有两个来源给出不同值」，不说明数据已经正确。
                    </p>
                  )}
                  {conflicts.map((g, i) => (
                    <div key={i} className="mb-4 rounded-lg border border-orange-500/30 p-3">
                      <div className="mb-2 text-sm font-medium">
                        {g.subject}.{g.predicate}
                      </div>
                      {(g.claims ?? []).map((c) => (
                        <div
                          key={c.id}
                          className="mb-2 flex items-start gap-3 border-l-2 border-border pl-3"
                        >
                          <div className="flex-1">
                            <div className="text-sm">{c.value}</div>
                            <div className="text-[11px] text-muted-foreground">
                              {c.source} · {c.confidence} · {c.artifact} {c.anchor} @ {c.revision}
                            </div>
                          </div>
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={busy}
                            onClick={() => void pick(c)}
                          >
                            看这条
                          </Button>
                        </div>
                      ))}
                    </div>
                  ))}
                </div>
              </>
            )}
          </section>

          <aside className="flex min-h-0 flex-col gap-3 overflow-auto">
            {!sel ? (
              <div className="rounded-lg border border-border p-4 text-sm text-muted-foreground">
                从左侧选择一条断言开始核验。
                <p className="mt-3 text-xs leading-relaxed">
                  核验必须能回答「谁、何时、凭什么」。批准者与理由为必填——
                  <b className="text-foreground">无追责的核验等于没有核验</b>。
                  <br />
                  agent 可以提出与取证，但不能自己决定什么算数。
                </p>
              </div>
            ) : (
              <>
                <div className="rounded-lg border border-border p-4">
                  <div className="mb-2 flex items-center gap-2">
                    <StatusBadge status={sel.status} />
                    <ConfBadge c={sel.confidence} />
                  </div>
                  <div className="text-base font-medium">
                    {sel.subject}.{sel.predicate}
                  </div>
                  <div className="mt-1 text-lg tabular-nums">{sel.value}</div>
                  <dl className="mt-3 space-y-1 border-t border-border pt-3 text-xs">
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">来源</dt>
                      <dd>{sel.source}</dd>
                    </div>
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">原件</dt>
                      <dd className="break-all">{sel.artifact}</dd>
                    </div>
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">锚点</dt>
                      <dd className="break-all">{sel.anchor}</dd>
                    </div>
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">修订</dt>
                      <dd>{sel.revision}</dd>
                    </div>
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">断言 ID</dt>
                      <dd className="break-all font-mono text-[11px]">{sel.id}</dd>
                    </div>
                  </dl>
                </div>

                <div className="rounded-lg border border-border p-4">
                  <div className="mb-3 text-sm font-medium">核验</div>
                  <label className="mb-1 block text-xs text-muted-foreground">
                    批准者（必须是人）
                  </label>
                  <Input
                    value={by}
                    onChange={(e) => setBy(e.target.value)}
                    placeholder="你的名字"
                    className="mb-3 h-8"
                  />
                  <label className="mb-1 block text-xs text-muted-foreground">方法</label>
                  <select
                    aria-label="核验方法"
                    className="mb-1 h-8 w-full rounded-md border border-input bg-transparent px-2 text-sm"
                    value={method}
                    onChange={(e) => setMethod(e.target.value)}
                  >
                    {METHODS.map((m) => (
                      <option key={m.value} value={m.value}>
                        {m.label}
                      </option>
                    ))}
                  </select>
                  <p className="mb-3 text-[11px] text-muted-foreground">
                    {METHODS.find((m) => m.value === method)?.hint}
                  </p>
                  {method === "measurement" && (
                    <>
                      <label className="mb-1 block text-xs text-muted-foreground">
                        依据（实测必填：版本、配置、样本数）
                      </label>
                      <Input
                        value={evidence}
                        onChange={(e) => setEvidence(e.target.value)}
                        placeholder="例：版本 2026-09；无增益；样本 3 次"
                        className="mb-3 h-8"
                      />
                    </>
                  )}
                  <label className="mb-1 block text-xs text-muted-foreground">理由（必填）</label>
                  <Input
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="例：与原文逐字比对一致"
                    className="mb-3 h-8"
                  />
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      className="flex-1"
                      disabled={busy}
                      onClick={() => void act("approve")}
                    >
                      批准
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="flex-1 border-rose-500/40 text-rose-300"
                      disabled={busy}
                      onClick={() => void act("reject")}
                    >
                      驳回
                    </Button>
                  </div>
                </div>

                <div className="rounded-lg border border-border p-4">
                  <div className="mb-2 text-sm font-medium">核验历史</div>
                  {hist.length === 0 ? (
                    <p className="text-xs text-muted-foreground">尚无核验记录。</p>
                  ) : (
                    <ul className="space-y-2">
                      {hist.map((h, i) => (
                        <li key={i} className="border-l-2 border-border pl-3 text-xs">
                          <div>
                            <b>{h.method}</b> · {h.approvedBy} 批准
                          </div>
                          <div className="text-muted-foreground">{h.reason}</div>
                          {h.evidence && (
                            <div className="text-muted-foreground">依据：{h.evidence}</div>
                          )}
                          <div className="text-[11px] text-muted-foreground/70">
                            {h.at} · 提出者 {h.proposedBy}
                          </div>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </>
            )}

            {/* 批量：执行前必须先看见影响面 */}
            <div className="rounded-lg border border-border p-4">
              <div className="mb-2 text-sm font-medium">批量核验</div>
              <p className="mb-3 text-[11px] text-muted-foreground">
                批量最容易造成大面积错误，因此**先预览**。每条断言会各自留下核验记录——
                审计轨迹不合并，批量不等于免责。
              </p>
              <div className="mb-2 flex gap-2">
                <Input
                  value={bPred}
                  onChange={(e) => setBPred(e.target.value)}
                  placeholder="谓词（可空）"
                  className="h-8"
                />
                <select
                  aria-label="批量分级"
                  className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
                  value={bConf}
                  onChange={(e) => setBConf(e.target.value)}
                >
                  <option value="">全部分级</option>
                  <option value="L1">L1</option>
                  <option value="L2">L2</option>
                  <option value="L3">L3</option>
                  <option value="L4">L4</option>
                </select>
              </div>
              <Button size="sm" variant="outline" className="w-full" onClick={() => void doPreview()}>
                预览影响面
              </Button>
              {preview && (
                <div className="mt-3 rounded border border-border p-2 text-xs">
                  <div className="mb-1">
                    命中 <b className="tabular-nums">{preview.matched}</b> 条
                  </div>
                  <div className="mb-2 text-muted-foreground">{preview.where}</div>
                  {preview.sample?.map((s) => (
                    <div key={s.id} className="truncate text-muted-foreground">
                      {s.subject}.{s.predicate} = {s.value}
                    </div>
                  ))}
                  <div className="mt-2 flex gap-2">
                    <Button
                      size="sm"
                      className="flex-1"
                      disabled={busy || preview.matched === 0}
                      onClick={() => void doBatch("approve")}
                    >
                      批准全部
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="flex-1 border-rose-500/40 text-rose-300"
                      disabled={busy || preview.matched === 0}
                      onClick={() => void doBatch("reject")}
                    >
                      驳回全部
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </aside>
        </div>
        )}
      </div>
    </div>
  );
}
