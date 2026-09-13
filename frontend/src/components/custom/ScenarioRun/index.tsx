/**
 * 场景运行页：选主体 → 填外部输入 → 选引用来源 → 出方案。
 *
 * 三件事必须显式：
 *  1. requires 不满足就**挡住按钮**，并逐项说明缺什么
 *  2. 外部输入的单位来自声明，不让手写——手写单位就是歧义制造机
 *  3. 产出必须带上**未核验比例**：只给结果不给可信度，
 *     正是「看起来已经核验过」的来源
 */
import { useState } from "react";

import { Button } from "../../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../ui/card";
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
import { Em } from "../Prose";
import { useScenarioRun } from "./useScenarioRun";
import {
  AssertionStatus,
  ClaimRef,
  Confidence,
  Field,
  Quantity,
  Requirement,
  Unit,
} from "../Term";

export default function ScenarioRun({ scenario }: { scenario: string | null }) {
  const [subject, setSubject] = useState("");
  const r = useScenarioRun(scenario, subject);
  const setup = r.setup;

  if (!scenario) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        当前项目没有场景，或尚未选择场景。
      </div>
    );
  }

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[26rem_1fr] gap-4 p-4">
      <ScrollArea className="min-h-0">
        <div className="grid gap-3 pr-1">
          <Card>
            <CardHeader className="py-3">
              <CardTitle className="text-sm">运行 {scenario}</CardTitle>
              <CardDescription>
                {setup?.runnable ? "requires 已满足，可以运行" : "requires 未满足，拒绝运行"}
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              <div className="grid gap-1">
                <Label htmlFor="run-subject" className="text-xs text-muted-foreground">
                  主体（{setup?.entity ?? "—"}）
                </Label>
                <Select value={subject} onValueChange={setSubject}>
                  <SelectTrigger id="run-subject" className="h-8 w-full" aria-label="主体">
                    <SelectValue placeholder="选择一个主体" />
                  </SelectTrigger>
                  <SelectContent>
                    {(setup?.subjects ?? []).map((x) => (
                      <SelectItem key={x} value={x}>
                        {x}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {(setup?.inputs ?? []).map((inp) => (
                <div key={inp.name} className="grid gap-1">
                  <Label htmlFor={`run-${inp.name}`} className="text-xs text-muted-foreground">
                    {inp.name}
                    <span className="ml-1 rounded bg-muted px-1 text-[10px]"><Unit name={inp.unit} /></span>
                    {inp.min !== null && inp.min !== undefined && (
                      <span className="ml-1 text-[10px]">
                        {inp.min} ~ {inp.max ?? "+∞"}
                      </span>
                    )}
                  </Label>
                  <Input
                    id={`run-${inp.name}`}
                    value={r.values[inp.name] ?? ""}
                    onChange={(e) => r.setValue(inp.name, e.target.value)}
                    placeholder={`数值（单位 ${inp.unit}）`}
                    className="h-8"
                  />
                  <p className="text-[11px] leading-snug text-muted-foreground">
                    <Em>{inp.description}</Em>
                  </p>
                </div>
              ))}

              {(setup?.refs ?? []).map((ref) => (
                <div key={ref.name} className="grid gap-1">
                  <Label htmlFor={`ref-${ref.name}`} className="text-xs text-muted-foreground">
                    {ref.name}
                    <span className="ml-1 text-[10px]">
                      来自 <Field entity={ref.entity} predicate={ref.via} />
                    </span>
                  </Label>
                  <Select
                    value={r.refs[ref.name] ?? ""}
                    onValueChange={(v) => r.setRef(ref.name, v)}
                    disabled={(ref.candidates ?? []).length === 0}
                  >
                    <SelectTrigger id={`ref-${ref.name}`} className="h-8 w-full" aria-label={ref.name}>
                      <SelectValue
                        placeholder={
                          subject === ""
                            ? "先选主体"
                            : (ref.candidates ?? []).length === 0
                              ? "没有候选"
                              : "选择来源"
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {(ref.candidates ?? []).map((c) => (
                        <SelectItem key={c} value={c}>
                          {c}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-[11px] leading-snug text-muted-foreground">
                    <Em>{ref.description}</Em>
                  </p>
                </div>
              ))}

              <Separator />

              <Button
                size="sm"
                className="w-full"
                aria-label="运行场景"
                disabled={r.loading || !setup?.runnable || !subject}
                onClick={() => void r.run()}
              >
                运行
              </Button>
              {!setup?.runnable && (
                <ul className="list-disc pl-4 text-[11px] text-rose-700">
                  {(setup?.missing ?? []).map((m) => (
                    <li key={m}>{m}</li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="py-3">
              <CardTitle className="text-sm">requires</CardTitle>
              <CardDescription>库里应当有的数据；缺失即不可运行</CardDescription>
            </CardHeader>
            <CardContent className="px-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>需求</TableHead>
                    <TableHead className="text-right">覆盖</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(setup?.requires ?? []).map((q) => (
                    <TableRow key={q.want}>
                      <TableCell className="text-xs">
                        {q.status === "satisfied" ? (
                          <span className="text-emerald-700">✓</span>
                        ) : (
                          <span className="text-rose-700">✗</span>
                        )}{" "}
                        <Requirement want={q.want} />
                      </TableCell>
                      <TableCell className="text-right text-xs tabular-nums">
                        {q.have}/{q.total}（{(q.coverage * 100).toFixed(0)}%）
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </ScrollArea>

      <ScrollArea className="min-h-0">
        <div className="grid gap-3 pr-1">
          {!r.result ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">还没有产出</CardTitle>
              </CardHeader>
              <CardContent className="text-xs leading-relaxed text-muted-foreground">
                选主体、填外部输入、选引用来源，然后运行。
                <br />
                产出会带上它依赖的<strong>未核验比例</strong>——一个没有可信度标注的数字，
                比没有数字更危险。
              </CardContent>
            </Card>
          ) : (
            <>
              <Card className={r.result.ok ? "" : "border-rose-200 bg-rose-50"}>
                <CardHeader className="py-3">
                  <CardTitle className="text-base">
                    {r.result.ok ? r.result.result : "拒绝运行"}
                  </CardTitle>
                  <CardDescription>
                    {r.result.scenario} · 主体 {r.result.subject} · {r.result.at}
                  </CardDescription>
                </CardHeader>
                <CardContent className="grid gap-2 text-sm">
                  <p className={r.result.ok ? "" : "text-rose-800"}>{r.result.message}</p>
                  {!r.result.ok && (r.result.missing ?? []).length > 0 && (
                    <ul className="list-disc pl-4 text-xs text-rose-800">
                      {(r.result.missing ?? []).map((m) => (
                        <li key={m}>{m}</li>
                      ))}
                    </ul>
                  )}
                </CardContent>
              </Card>

              {r.result.ok && (
                <>
                  <Card>
                    <CardHeader className="py-3">
                      <CardTitle className="text-sm">可信度</CardTitle>
                      <CardDescription>
                        未核验 {(r.result.unverifiedRatio * 100).toFixed(0)}%
                        （{(r.result.unverified ?? []).length} 项依赖未经核验）
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      {(r.result.external ?? []).length > 0 && (
                        <div>
                          <div className="mb-1 text-xs font-medium text-amber-700">
                            外部输入——<strong>始终未核验</strong>
                          </div>
                          {(r.result.external ?? []).map((e) => (
                            <div key={e.name} className="text-xs text-muted-foreground">
                              {e.name} = {e.value}
                            </div>
                          ))}
                        </div>
                      )}
                      <div>
                        <div className="mb-1 text-xs font-medium">参与计算的取值</div>
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>绑定</TableHead>
                              <TableHead>取值</TableHead>
                              <TableHead>单位</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {(r.result.bindings ?? []).map((b) => (
                              <TableRow key={b.name}>
                                <TableCell className="font-mono text-xs">{b.name}</TableCell>
                                <TableCell className="tabular-nums">
                                  <Quantity value={b.value} unit={b.unit} />
                                </TableCell>
                                <TableCell className="text-xs">{b.unit ? <Unit name={b.unit} /> : "—"}</TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                      {(r.result.notes ?? []).length > 0 && (
                        <ul className="list-disc pl-4 text-[11px] text-muted-foreground">
                          {(r.result.notes ?? []).map((n, i) => (
                            <li key={i}>{n}</li>
                          ))}
                        </ul>
                      )}
                    </CardContent>
                  </Card>

                  {(r.result.unverified ?? []).length > 0 && (
                    <Card>
                      <CardHeader className="py-3">
                        <CardTitle className="text-sm">
                          未核验的依赖（{(r.result.unverified ?? []).length} 项）
                        </CardTitle>
                        <CardDescription>
                          这些断言还没有人核验过——结论据此产出，请据此判断它的分量
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="px-0">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>断言</TableHead>
                              <TableHead>取值</TableHead>
                              <TableHead>分级</TableHead>
                              <TableHead>状态</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {(r.result.unverified ?? []).map((c) => (
                              <TableRow key={c.id}>
                                <TableCell className="text-xs">
                                  <ClaimRef claim={c} />
                                </TableCell>
                                <TableCell className="tabular-nums">{c.value}</TableCell>
                                <TableCell className="text-xs"><Confidence value={c.confidence} /></TableCell>
                                <TableCell className="text-xs"><AssertionStatus value={c.status} /></TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </CardContent>
                    </Card>
                  )}
                </>
              )}
            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}