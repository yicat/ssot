/**
 * 核验：队列与冲突。
 *
 * 它是本项目里**人唯一需要介入的地方**：数据由程序接入与准入，
 * 人在这里判断什么算数。
 *
 * 界面遵循四条规格要求：
 *  1. 一切默认未核验——每条都带着状态与分级，未核验不会被藏起来
 *  2. 排序理由必须可见——否则人只能盲信排序
 *  3. 冲突成组呈现，且**系统不裁决**——只把同一件事的说法摆在一起
 *  4. 批量最容易造成大面积错误，因此**先预览影响面**
 */
import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../../ui/card";
import { Input } from "../../ui/input";
import { Label } from "../../ui/label";
import { ScrollArea } from "../../ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/select";
import { Separator } from "../../ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../ui/table";
import type { Item, QueueItem } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useSessionStore } from "../Session/store";
import { useGlossary } from "../Session/glossary";
import {
  Actor,
  Confidence,
  Field,
  Quantity,
  Subject,
} from "../Term";
import { METHODS } from "./store";
import { STATUS_CLASS, STATUS_LABEL, useReview } from "./useReview";

export function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant="outline" className={STATUS_CLASS[status] ?? ""}>
      {STATUS_LABEL[status] ?? status}
    </Badge>
  );
}

export function ConfidenceBadge({ c }: { c: string }) {
  return (
    <Badge variant="outline" className="border-slate-300 text-slate-700">
      <Confidence value={c} />
    </Badge>
  );
}

function TierBadge({ q }: { q: QueueItem }) {
  const cls: Record<string, string> = {
    争议: "border-orange-300 text-orange-700",
    影响面: "border-sky-300 text-sky-700",
    分级: "border-violet-300 text-violet-700",
    抽检: "border-teal-300 text-teal-700",
    常规: "border-slate-300 text-slate-600",
  };
  return (
    <span className="inline-flex items-center gap-1">
      <Badge variant="outline" className={cls[q.tier] ?? ""}>
        {q.tier}
      </Badge>
      {q.sampled && q.tier !== "抽检" && (
        <Badge variant="outline" className="border-teal-300 text-teal-700">
          抽检
        </Badge>
      )}
    </span>
  );
}

export default function Review({ mode }: { mode: "queue" | "conflicts" }) {
  const r = useReview();
  // 使用者身份是**会话级**的：在核验里填一次，经验与待判定里不该再填一次。
  const sessionBy = useSessionStore((s) => s.by);
  const sessionSetBy = useSessionStore((s) => s.setBy);
  const g = useGlossary();
  const byStatus = r.stats?.byStatus ?? {};

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_26rem] gap-4 p-4">
      <Card className="flex min-h-0 flex-col gap-0 py-0">
        <CardHeader className="flex-row items-center gap-3 border-b py-2">
          <CardTitle className="text-sm font-medium">
            {mode === "queue" ? `队列 ${r.queue.length} 条` : `${r.conflicts.length} 组冲突`}
          </CardTitle>
          {mode === "queue" ? (
            <div className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
              <Label htmlFor="entity-filter" className="text-xs">
                实体
              </Label>
              <Select value={r.entity || "__all"} onValueChange={(v) => r.setEntity(v === "__all" ? "" : v)}>
                <SelectTrigger id="entity-filter" size="sm" className="w-32" aria-label="实体筛选">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all">全部实体</SelectItem>
                  {Object.keys(r.stats?.byEntity ?? {}).map((k) => (
                    <SelectItem key={k} value={k}>
                      {k}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Label htmlFor="status-filter" className="text-xs">
                状态
              </Label>
              <Select value={r.status} onValueChange={r.setStatus}>
                <SelectTrigger id="status-filter" size="sm" className="w-28" aria-label="状态筛选">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.keys(STATUS_LABEL).map((k) => (
                    <SelectItem key={k} value={k}>
                      {STATUS_LABEL[k]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Label htmlFor="sample-ratio" className="text-xs">
                强制抽检
              </Label>
              <Select
                value={String(r.sampleRatio)}
                onValueChange={(v) => r.setSampleRatio(Number(v))}
              >
                <SelectTrigger id="sample-ratio" size="sm" className="w-20" aria-label="抽检比例">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[0, 0.02, 0.05, 0.1].map((v) => (
                    <SelectItem key={v} value={String(v)}>
                      {v * 100}%
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : (
            <span className="ml-auto text-xs text-muted-foreground">
              **系统不替你裁决**，只把同一件事的说法摆在一起
            </span>
          )}
        </CardHeader>
        <CardContent className="min-h-0 flex-1 overflow-hidden px-0">
          <ScrollArea className="h-full">
            {mode === "queue" ? (
              <Table>
                <TableHeader className="sticky top-0 bg-card">
                  <TableRow>
                    <TableHead>主体</TableHead>
                    <TableHead>谓词</TableHead>
                    <TableHead>取值</TableHead>
                    <TableHead>分级</TableHead>
                    <TableHead>类别</TableHead>
                    <TableHead>主导理由</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {r.queue.map((q) => (
                    <TableRow
                      key={q.item.id}
                      onClick={() => void r.pick(q.item)}
                      className={
                        "cursor-pointer " + (r.selected?.id === q.item.id ? "bg-secondary" : "")
                      }
                    >
                      <TableCell className="whitespace-nowrap">
                        <Subject entity={q.item.entity} subject={q.item.subject} />
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        <Field entity={q.item.entity} predicate={q.item.predicate} />
                      </TableCell>
                      <TableCell className="max-w-[16rem] truncate">
                        <Quantity value={q.item.value} unit={q.item.unit} />
                      </TableCell>
                      <TableCell>
                        <ConfidenceBadge c={q.item.confidence} />
                      </TableCell>
                      <TableCell>
                        <TierBadge q={q} />
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{q.reason}</TableCell>
                    </TableRow>
                  ))}
                  {r.queue.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                        该筛选下没有断言。
                        <div className="mt-1 text-xs">
                          这只说明「这个筛选下没有」，不说明数据已经核验完了。
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            ) : (
              <div className="p-3">
                {r.conflicts.length === 0 && (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    未发现冲突。这只说明「没有两个来源给出不同值」，不说明数据已经正确。
                  </p>
                )}
                {r.conflicts.map((g, i) => (
                  <Card key={i} className="mb-3 border-orange-300">
                    <CardHeader className="py-2">
                      <CardTitle className="text-sm font-medium">
                        {/* 冲突的 identity 是「主体 + 谓词」，两者都要有中文 */}
                        {g.claims?.[0] ? (
                          <>
                            <Subject entity={g.claims[0].entity} subject={g.subject} />
                            <span className="ml-2">
                              <Field entity={g.claims[0].entity} predicate={g.predicate} />
                            </span>
                          </>
                        ) : (
                          `${g.subject}.${g.predicate}`
                        )}
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      {(g.claims ?? []).map((c: Item) => (
                        <div key={c.id} className="flex items-start gap-3 border-l-2 pl-3">
                          <div className="flex-1">
                            <div className="text-sm">{c.value}</div>
                            <div className="text-[11px] text-muted-foreground">
                              {c.source} · {c.confidence} · {c.artifact} {c.anchor} @ {c.revision}
                            </div>
                          </div>
                          <Button size="sm" variant="outline" disabled={r.loading} onClick={() => void r.pick(c)}>
                            看这条
                          </Button>
                        </div>
                      ))}
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </ScrollArea>
        </CardContent>
      </Card>

      <ScrollArea className="min-h-0">
        <div className="grid gap-3 pr-1">
          {!r.selected ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">从左侧选择一条断言开始核验</CardTitle>
              </CardHeader>
              <CardContent className="text-xs leading-relaxed text-muted-foreground">
                核验必须能回答「谁、何时、凭什么」。批准者与理由为必填——
                <b className="text-foreground">无追责的核验等于没有核验</b>。
                <br />
                agent 可以提出与取证，但不能自己决定什么算数。
                <Separator className="my-3" />
                当前状态：待核验{" "}
                <b className="tabular-nums text-foreground">{byStatus["pending"] ?? 0}</b>，已核验{" "}
                <b className="tabular-nums text-foreground">{byStatus["verified"] ?? 0}</b>，
                核验记录 <b className="tabular-nums text-foreground">{r.stats?.verifications ?? 0}</b>
              </CardContent>
            </Card>
          ) : (
            <>
              <Card>
                <CardHeader className="py-3">
                  <div className="flex items-center gap-2">
                    <StatusBadge status={r.selected.status} />
                    <ConfidenceBadge c={r.selected.confidence} />
                  </div>
                  <CardTitle className="text-base">
                    <Subject entity={r.selected.entity} subject={r.selected.subject} />
                    <span className="ml-2 text-sm font-normal">
                      <Field entity={r.selected.entity} predicate={r.selected.predicate} />
                    </span>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-lg tabular-nums">
                    <Quantity value={r.selected.value} unit={r.selected.unit} />
                  </div>
                  <dl className="mt-3 grid gap-1 border-t pt-3 text-xs">
                    <Row k="来源" v={r.selected.source} />
                    <Row k="原件" v={r.selected.artifact} />
                    <Row k="锚点" v={r.selected.anchor} />
                    <Row k="修订" v={g.revision(r.selected.revision)} />
                    <Row k="断言 ID" v={r.selected.id} mono />
                  </dl>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">核验</CardTitle>
                </CardHeader>
                <CardContent className="grid gap-2">
                  <div className="grid gap-1">
                    <Label htmlFor="by" className="text-xs text-muted-foreground">
                      批准者（必须是人）
                    </Label>
                    <Input
                      id="by"
                      value={sessionBy}
                      onChange={(e) => sessionSetBy(e.target.value)}
                      placeholder="你的名字"
                      className="h-8"
                    />
                  </div>
                  <div className="grid gap-1">
                    <Label htmlFor="method" className="text-xs text-muted-foreground">
                      方法
                    </Label>
                    <Select value={r.method} onValueChange={r.setMethod}>
                      <SelectTrigger id="method" className="h-8 w-full" aria-label="核验方法">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {METHODS.map((m) => (
                          <SelectItem key={m.value} value={m.value}>
                            {m.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <p className="text-[11px] text-muted-foreground">
                      {METHODS.find((m) => m.value === r.method)?.hint}
                    </p>
                  </div>
                  {r.method === "measurement" && (
                    <div className="grid gap-1">
                      <Label htmlFor="evidence" className="text-xs text-muted-foreground">
                        依据（实测必填：版本、配置、样本数）
                      </Label>
                      <Input
                        id="evidence"
                        value={r.evidence}
                        onChange={(e) => r.setEvidence(e.target.value)}
                        placeholder="例：版本 2026-09；无增益；样本 3 次"
                        className="h-8"
                      />
                    </div>
                  )}
                  <div className="grid gap-1">
                    <Label htmlFor="reason" className="text-xs text-muted-foreground">
                      理由（必填）
                    </Label>
                    <Input
                      id="reason"
                      value={r.reason}
                      onChange={(e) => r.setReason(e.target.value)}
                      placeholder="例：与原文逐字比对一致"
                      className="h-8"
                    />
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      className="flex-1"
                      disabled={r.loading}
                      onClick={() => void r.decide("approve")}
                    >
                      批准
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="flex-1 border-rose-300 text-rose-700"
                      disabled={r.loading}
                      onClick={() => void r.decide("reject")}
                    >
                      驳回
                    </Button>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">核验历史</CardTitle>
                </CardHeader>
                <CardContent>
                  {r.history.length === 0 ? (
                    <p className="text-xs text-muted-foreground">
                      尚无核验记录——该断言尚未被任何人核验。
                    </p>
                  ) : (
                    <ul className="grid gap-2">
                      {r.history.map((h, i) => (
                        <li key={i} className="border-l-2 pl-3 text-xs">
                          <div>
                            <b>{h.method}</b> · <Actor id={h.approvedBy} /> 批准
                          </div>
                          <div className="text-muted-foreground">{h.reason}</div>
                          {h.evidence && (
                            <div className="text-muted-foreground">依据：{h.evidence}</div>
                          )}
                          <div className="text-[11px] text-muted-foreground/70">
                            {h.at} · 提出者 <Actor id={h.proposedBy} />
                          </div>
                        </li>
                      ))}
                    </ul>
                  )}
                </CardContent>
              </Card>
            </>
          )}

          {/* 批量：执行前必须先看见影响面 */}
          <Card>
            <CardHeader className="py-3">
              <CardTitle className="text-sm">批量核验</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2">
              <p className="text-[11px] leading-relaxed text-muted-foreground">
                批量最容易造成大面积错误，因此**先预览**。每条断言会各自留下核验记录——
                审计轨迹不合并，批量不等于免责。
              </p>
              <div className="flex gap-2">
                <Input
                  value={r.batchPredicate}
                  onChange={(e) => r.setBatchPredicate(e.target.value)}
                  placeholder="谓词（可空）"
                  className="h-8"
                  aria-label="批量谓词"
                />
                <Select
                  value={r.batchConfidence || "__all"}
                  onValueChange={(v) => r.setBatchConfidence(v === "__all" ? "" : v)}
                >
                  <SelectTrigger className="h-8 w-28" aria-label="批量分级">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all">全部分级</SelectItem>
                    {["L1", "L2", "L3", "L4"].map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <Button size="sm" variant="outline" className="w-full" onClick={() => void r.previewBatch()}>
                预览影响面
              </Button>
              {r.preview && (
                <div className="grid gap-2 rounded-md border p-2 text-xs">
                  <div>
                    命中 <b className="tabular-nums">{r.preview.matched}</b> 条
                  </div>
                  <div className="text-muted-foreground">{r.preview.where}</div>
                  {r.preview.sample?.map((x) => (
                    <div key={x.id} className="truncate text-muted-foreground">
                      {x.subject}.{x.predicate} = {x.value}
                    </div>
                  ))}
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      className="flex-1"
                      disabled={r.loading || r.preview.matched === 0}
                      onClick={() => void r.batch("approve")}
                    >
                      批准全部
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="flex-1 border-rose-300 text-rose-700"
                      disabled={r.loading || r.preview.matched === 0}
                      onClick={() => void r.batch("reject")}
                    >
                      驳回全部
                    </Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </ScrollArea>
    </div>
  );
}

function Row({ k, v, mono = false }: { k: string; v: string; mono?: boolean }) {
  return (
    <div className="flex gap-2">
      <dt className="w-16 shrink-0 text-muted-foreground">{k}</dt>
      <dd className={"break-all " + (mono ? "font-mono text-[11px]" : "")}>{v}</dd>
    </div>
  );
}
