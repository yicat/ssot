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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../ui/table";
import { useSessionStore } from "../Session/store";
import { useGlossary } from "../Session/glossary";
import { Field, Subject, Unit } from "../Term";
import { DECISION_METHODS, useDecision } from "./useDecision";

const STATUS_CLASS: Record<string, string> = {
  open: "border-amber-300 text-amber-700",
  deferred: "border-slate-300 text-slate-600",
  decided: "border-emerald-300 text-emerald-700",
  stale: "border-rose-300 text-rose-700",
};

export function DecisionStatusBadge({ status, text }: { status: string; text: string }) {
  return (
    <Badge variant="outline" className={STATUS_CLASS[status] ?? ""}>
      {text}
    </Badge>
  );
}

export default function Decision() {
  const d = useDecision();
  // 使用者身份是**会话级**的（同核验页）。
  const sessionBy = useSessionStore((s) => s.by);
  const sessionSetBy = useSessionStore((s) => s.setBy);
  const g = useGlossary();
  const s = d.stats;
  const it = d.selected;

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_30rem] gap-4 p-4">
      <Card className="flex min-h-0 flex-col gap-0 py-0">
        <CardHeader className="flex-row items-center gap-3 border-b py-2">
          <CardTitle className="text-sm font-medium">待判定 {s?.open ?? 0} 条</CardTitle>
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span>已暂缓 {s?.deferred ?? 0}</span>
            <span className={s?.stale ? "text-rose-700" : ""}>需复核 {s?.stale ?? 0}</span>
            <span>已裁决 {s?.decided ?? 0}</span>
            <span className={s?.missing ? "text-amber-700" : ""}>判为缺失 {s?.missing ?? 0}</span>
          </div>
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            disabled={d.loading}
            onClick={() => void d.reload()}
          >
            刷新
          </Button>
        </CardHeader>
        <CardContent className="min-h-0 flex-1 overflow-hidden px-0">
          <ScrollArea className="h-full">
            <Table>
              <TableHeader className="sticky top-0 bg-card">
                <TableRow>
                  <TableHead>主体</TableHead>
                  <TableHead>谓词</TableHead>
                  <TableHead className="text-right">候选</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>为什么排在前面</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {d.items.map((x) => (
                  <TableRow
                    key={x.id}
                    onClick={() => d.select(x.id)}
                    className={"cursor-pointer " + (d.selectedId === x.id ? "bg-secondary" : "")}
                  >
                    <TableCell className="whitespace-nowrap">
                      <Subject entity={x.entity} subject={x.subject} />`r`n                    </TableCell>
                    <TableCell className="whitespace-nowrap">`r`n                      <Field entity={x.entity} predicate={x.predicate} />`r`n                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {x.candidates?.length ?? 0}
                    </TableCell>
                    <TableCell>
                      <DecisionStatusBadge status={x.status} text={x.statusText} />
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{x.impactReason}</TableCell>
                  </TableRow>
                ))}
                {d.items.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                      没有需要人看的待判定事项。
                      <div className="mt-1 text-xs">
                        这只说明「原文里没有多值歧义」，不说明数据已经完整。
                      </div>
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </ScrollArea>
        </CardContent>
      </Card>

      <ScrollArea className="min-h-0">
        <div className="grid gap-3 pr-1">
          {!it ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">从左侧选择一条待判定事项</CardTitle>
              </CardHeader>
              <CardContent className="text-xs leading-relaxed text-muted-foreground">
                这类条目是**文本里有多个候选值、无法确定取哪一个**的项。工具在这里刻意不猜：
                猜错的值会带着「已抽取」的样子进入事实源，比缺失更危险。
              </CardContent>
            </Card>
          ) : (
            <>
              <Card>
                <CardHeader className="py-3">
                  <div className="flex items-center gap-2">
                    <DecisionStatusBadge status={it.status} text={it.statusText} />
                    <span className="text-xs text-muted-foreground">被引用 {it.impact} 次</span>
                  </div>
                  <CardTitle className="text-base">
                    <Subject entity={it.entity} subject={it.subject} />
                    <span className="ml-2 text-sm font-normal">
                      <Field entity={it.entity} predicate={it.predicate} />
                    </span>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-xs text-muted-foreground">{it.reason}</div>
                  <dl className="mt-3 grid gap-1 border-t pt-3 text-xs">
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">原件</dt>
                      <dd className="break-all">{it.artifact}</dd>
                    </div>
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">修订</dt>
                      <dd>{g.revision(it.revision)}</dd>
                    </div>
                    <div className="flex gap-2">
                      <dt className="w-16 shrink-0 text-muted-foreground">来源</dt>
                      <dd>{it.source}</dd>
                    </div>
                  </dl>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">歧义所在的原文</CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="rounded bg-muted/60 p-2 text-xs leading-relaxed">{it.context}</p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">
                    候选（{it.candidates?.length ?? 0} 个）
                    <span className="ml-2 text-[11px] font-normal text-muted-foreground">
                      逐句核对后选一个——工具不替你选
                    </span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid gap-2">
                  {it.candidates?.map((c) => {
                    const active = d.choice === c.index && it.editable;
                    return (
                      <button
                        key={c.index}
                        type="button"
                        disabled={!it.editable}
                        onClick={() => d.setChoice(c.index)}
                        className={
                          "block w-full rounded-md border p-2 text-left transition-colors " +
                          (active ? "border-emerald-400 bg-emerald-50" : "border-border") +
                          (it.editable ? " hover:bg-secondary/60" : " opacity-70")
                        }
                      >
                        <div className="flex items-baseline gap-2">
                          <span className="text-xs text-muted-foreground">[{c.index}]</span>
                          <span className="text-base font-medium tabular-nums">
                            {c.value} <Unit name={c.unit} />
                          </span>
                          <span className="ml-auto text-[11px] text-muted-foreground">
                            {c.anchor}
                          </span>
                        </div>
                        <div className="mt-1 text-xs text-muted-foreground">{c.context}</div>
                        {c.note && <div className="mt-1 text-[11px] text-sky-700">{c.note}</div>}
                      </button>
                    );
                  })}
                  {it.resolution && (
                    <div className="mt-2 grid gap-1 border-t pt-3 text-xs">
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
                        <div className="break-all font-mono text-[11px] text-muted-foreground">
                          断言 {it.resolution.assertionId}
                        </div>
                      )}
                    </div>
                  )}
                </CardContent>
              </Card>

              {it.editable ? (
                <Card>
                  <CardHeader className="py-3">
                    <CardTitle className="text-sm">裁决</CardTitle>
                  </CardHeader>
                  <CardContent className="grid gap-2">
                    <div className="grid gap-1">
                      <Label htmlFor="decision-by" className="text-xs text-muted-foreground">
                        裁决人（必须是人）
                      </Label>
                      <Input
                        id="decision-by"
                        value={sessionBy}
                        onChange={(e) => sessionSetBy(e.target.value)}
                        placeholder="你的名字"
                        className="h-8"
                      />
                    </div>
                    <div className="grid gap-1">
                      <Label htmlFor="decision-method" className="text-xs text-muted-foreground">
                        方法
                      </Label>
                      <Select value={d.method} onValueChange={d.setMethod}>
                        <SelectTrigger id="decision-method" className="h-8 w-full" aria-label="裁决方法">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {DECISION_METHODS.map((m) => (
                            <SelectItem key={m.value} value={m.value}>
                              {m.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <p className="text-[11px] text-muted-foreground">
                        {DECISION_METHODS.find((m) => m.value === d.method)?.hint}
                      </p>
                    </div>
                    {d.method === "measurement" && (
                      <div className="grid gap-1">
                        <Label htmlFor="decision-evidence" className="text-xs text-muted-foreground">
                          依据（实测必填：版本、配置、样本数）
                        </Label>
                        <Input
                          id="decision-evidence"
                          value={d.evidence}
                          onChange={(e) => d.setEvidence(e.target.value)}
                          placeholder="例：版本 2026-09；本体无增益；样本 3 次"
                          className="h-8"
                        />
                      </div>
                    )}
                    <div className="grid gap-1">
                      <Label htmlFor="decision-reason" className="text-xs text-muted-foreground">
                        理由（必填）
                      </Label>
                      <Input
                        id="decision-reason"
                        value={d.reason}
                        onChange={(e) => d.setReason(e.target.value)}
                        placeholder="例：主伤害那一句是 88%，另一处是条件分支"
                        className="h-8"
                      />
                    </div>
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        className="flex-1"
                        disabled={d.loading || d.choice < 0}
                        onClick={() => void d.resolve(d.choice)}
                      >
                        裁决选中候选
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1 border-amber-300 text-amber-700"
                        disabled={d.loading}
                        onClick={() => void d.resolve(-1)}
                      >
                        都不对
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={d.loading}
                        onClick={() => void d.defer()}
                      >
                        暂缓
                      </Button>
                    </div>
                    <p className="text-[11px] leading-relaxed text-muted-foreground">
                      「都不对」是**结论**：该谓词记为缺失，谁都别猜。「暂缓」是**未处理**：
                      条目仍留在队列里。两者不可混为一谈。
                    </p>
                  </CardContent>
                </Card>
              ) : (
                <Card>
                  <CardContent className="pt-4 text-xs text-muted-foreground">
                    {it.status === "decided"
                      ? "已裁决。改判不是覆盖——需要以新的核验记录表达，当前版本不支持。"
                      : "该状态暂不可裁决。"}
                  </CardContent>
                </Card>
              )}
            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
