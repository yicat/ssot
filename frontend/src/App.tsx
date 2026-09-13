/**
 * 核验工作台。
 *
 * 它是本项目里**人唯一需要介入的地方**：数据由程序接入与准入，
 * 人在这里判断什么算数。
 *
 * 界面遵循三条规格要求：
 *  1. 一切默认未核验——队列里每一条都带着状态与分级，未核验不会被藏起来
 *  2. 核验必须能回答「谁、何时、凭什么」——批准者与理由为必填
 *  3. 溯源可见——每条断言都能看到它来自哪个原件、哪个位置、哪个修订
 */
import { useCallback, useEffect, useState } from "react";

import {
  Approve,
  History,
  Pending,
  ProjectDir,
  Reject,
  Stats,
} from "../bindings/github.com/ngnl5/ssot/internal/api/reviewservice";
import type {
  HistoryItem,
  Item,
  Stats as StatsData,
} from "../bindings/github.com/ngnl5/ssot/internal/api/models";

import { Badge } from "./components/ui/badge";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";

const METHODS = [
  { value: "editorial", label: "编审", hint: "人工阅读后判断——这是判断，不是验证" },
  { value: "cross-source", label: "多源", hint: "多个**独立**来源一致" },
  { value: "recompute", label: "重算", hint: "由其他已核验断言推导" },
  { value: "measurement", label: "实测", hint: "游戏内观测，必须记录版本/配置/样本数" },
];

function StatusBadge({ status }: { status: string }) {
  const cls: Record<string, string> = {
    pending: "border-amber-500/40 text-amber-400",
    verified: "border-emerald-500/40 text-emerald-400",
    rejected: "border-rose-500/40 text-rose-400",
    disputed: "border-orange-500/40 text-orange-400",
    "auto-checked": "border-sky-500/40 text-sky-400",
  };
  const label: Record<string, string> = {
    pending: "待核验",
    verified: "已核验",
    rejected: "已驳回",
    disputed: "有争议",
    "auto-checked": "已自动预检",
  };
  return (
    <Badge variant="outline" className={cls[status] ?? ""}>
      {label[status] ?? status}
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

export default function App() {
  const [dir, setDir] = useState("");
  const [stats, setStats] = useState<StatsData | null>(null);
  const [items, setItems] = useState<Item[]>([]);
  const [sel, setSel] = useState<Item | null>(null);
  const [hist, setHist] = useState<HistoryItem[]>([]);

  const [entity, setEntity] = useState("");
  const [status, setStatus] = useState("pending");

  const [by, setBy] = useState("");
  const [method, setMethod] = useState("editorial");
  const [reason, setReason] = useState("");
  const [evidence, setEvidence] = useState("");

  const [flash, setFlash] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const [s, list] = await Promise.all([Stats(), Pending(entity, status, 300)]);
      setStats(s);
      setItems(list);
      // 刻意不清 flash：刷新发生在操作之后，清掉会把刚产生的成功/失败提示抹掉。
      // 提示由操作本身负责清除（见 pick 与 act）。
    } catch (e) {
      setFlash({ ok: false, text: String(e) });
    } finally {
      setBusy(false);
    }
  }, [entity, status]);

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
      setHist(await History(it.id));
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

  const byStatus = stats?.byStatus ?? {};
  const pendingCount = byStatus["pending"] ?? 0;
  const verifiedCount = byStatus["verified"] ?? 0;
  const rejectedCount = byStatus["rejected"] ?? 0;

  return (
    <div className="dark min-h-screen bg-background text-foreground">
      <div className="mx-auto flex h-screen max-w-[1500px] flex-col gap-3 p-4">
        {/* 顶部：库的整体状态。未核验的比例必须一眼可见 */}
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
              待核验 <b className="tabular-nums">{pendingCount}</b>
            </span>
            <span className="text-emerald-400">
              已核验 <b className="tabular-nums">{verifiedCount}</b>
            </span>
            <span className="text-rose-400">
              已驳回 <b className="tabular-nums">{rejectedCount}</b>
            </span>
            <span className="text-muted-foreground">
              核验记录 <b className="tabular-nums">{stats?.verifications ?? 0}</b>
            </span>
          </div>
          <div className="ml-auto flex items-center gap-2">
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

        <div className="grid min-h-0 flex-1 grid-cols-[1fr_26rem] gap-4">
          {/* 左：队列 */}
          <section className="flex min-h-0 flex-col rounded-lg border border-border">
            <div className="border-b border-border px-3 py-2 text-xs text-muted-foreground">
              队列 {items.length} 条{items.length >= 300 ? "（已截断至 300）" : ""}
            </div>
            <div className="min-h-0 flex-1 overflow-auto">
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-background text-xs text-muted-foreground">
                  <tr className="[&>th]:px-3 [&>th]:py-2 [&>th]:text-left [&>th]:font-medium">
                    <th>主体</th>
                    <th>谓词</th>
                    <th>取值</th>
                    <th>分级</th>
                    <th>状态</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((it) => (
                    <tr
                      key={it.id}
                      onClick={() => void pick(it)}
                      className={
                        "cursor-pointer border-t border-border/60 [&>td]:px-3 [&>td]:py-1.5 hover:bg-accent/40 " +
                        (sel?.id === it.id ? "bg-accent/60" : "")
                      }
                    >
                      <td className="whitespace-nowrap">
                        {it.entity}/{it.subject}
                      </td>
                      <td className="whitespace-nowrap">{it.predicate}</td>
                      <td className="max-w-[22rem] truncate">{it.value}</td>
                      <td>
                        <ConfBadge c={it.confidence} />
                      </td>
                      <td>
                        <StatusBadge status={it.status} />
                      </td>
                    </tr>
                  ))}
                  {items.length === 0 && (
                    <tr>
                      <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                        该筛选下没有断言
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>

          {/* 右：详情与核验 */}
          <aside className="flex min-h-0 flex-col gap-3 overflow-auto">
            {!sel ? (
              <div className="rounded-lg border border-border p-4 text-sm text-muted-foreground">
                从左侧选择一条断言开始核验。
                <p className="mt-3 text-xs leading-relaxed">
                  核验必须能回答「谁、何时、凭什么」。因此批准者与理由为必填——
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
          </aside>
        </div>
      </div>
    </div>
  );
}
