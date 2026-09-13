/**
 * 备选方案页：多解并排，系统不裁决。
 *
 * 界面上刻意**没有**的东西：没有推荐标记、没有默认选中项、没有「一键选最优」。
 * 一个预选中的方案会让人以为那就是推荐，而这一层要守的边界恰恰是
 * 「系统不替使用者做取舍」。
 */
import { useState } from "react";

import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../ui/card";
import { Input } from "../../ui/input";
import { Label } from "../../ui/label";
import { ScrollArea } from "../../ui/scroll-area";
import { Separator } from "../../ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../ui/table";
import { STATUS_CLASS, useAlternativesStore } from "./store";
import { Actor, Confidence, Dep, Quantity } from "../Term";
import { useAlternatives } from "./useAlternatives";

export default function Alternatives({ scenario }: { scenario: string | null }) {
  const a = useAlternatives(scenario);
  const sessionBy = useAlternativesStore((s) => s.by);
  const setSessionBy = useAlternativesStore((s) => s.setBy);
  const [showForm, setShowForm] = useState(false);

  if (!scenario) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        当前项目没有场景，或尚未选择场景。备选方案是某个用途下的取舍，
        因此它挂在场景下。
      </div>
    );
  }

  const ev = a.evaluation;
  const shown = ev?.plans ?? a.plans;

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_26rem] gap-4 p-4">
      <Card className="flex min-h-0 flex-col gap-0 py-0">
        <CardHeader className="flex-row items-center gap-3 border-b py-2">
          <CardTitle className="text-sm font-medium">
            备选 {shown.length} 个
            {ev && (ev.pruned ?? []).length > 0 && (
              <span className="ml-2 font-normal text-muted-foreground">
                剪掉 {(ev.pruned ?? []).length} 个被支配的
              </span>
            )}
          </CardTitle>
          <div className="ml-auto flex items-center gap-2">
            <Button size="sm" variant="outline" disabled={a.loading} onClick={() => setShowForm((v) => !v)}>
              {showForm ? "收起" : "提出方案"}
            </Button>
            <Button size="sm" variant="outline" disabled={a.loading} onClick={() => void a.refresh()}>
              检查依据
            </Button>
            <Button size="sm" variant="outline" disabled={a.loading} onClick={() => void a.reload()}>
              刷新
            </Button>
          </div>
        </CardHeader>

        <CardContent className="min-h-0 flex-1 overflow-hidden px-0">
          <ScrollArea className="h-full">
            {showForm && <ProposeForm scenario={scenario} disabled={a.loading} onSubmit={a.propose} />}

            {ev && (ev.constraints ?? []).length > 0 && (
              <div className="m-3 rounded-md border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
                <div className="font-medium">{ev.note}</div>
                <ul className="mt-2 list-disc pl-5 text-xs">
                  {(ev.constraints ?? []).map((c) => (
                    <li key={c}>{c}</li>
                  ))}
                </ul>
              </div>
            )}

            {ev?.note && (ev.constraints ?? []).length === 0 && (
              <div className="m-3 rounded-md border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800">
                {ev.note}
              </div>
            )}

            {ev && (ev.preferenceInferred ?? "") !== "" && (
              <div className="mx-3 mt-3 rounded-md border border-sky-200 bg-sky-50 p-3 text-xs text-sky-800">
                从历史选择推测你偏好「{ev.preferenceInferred}」——这只是**建议**，
                必须你自己确认才生效。其他备选没有被删掉。
              </div>
            )}

            {shown.length === 0 && !ev && (
              <div className="p-8 text-center text-sm text-muted-foreground">
                这个场景还没有方案。
                <div className="mt-1 text-xs">
                  方案不是「最优解」——它是一组取舍不同的备选，各自声明目标、代价与假设的偏好。
                </div>
              </div>
            )}

            <div className="grid gap-3 p-3">
              {shown.map((p) => (
                <Card key={p.id} className={p.chosen ? "border-emerald-300" : ""}>
                  <CardHeader className="py-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <CardTitle className="text-base">{p.title}</CardTitle>
                      <Badge variant="outline" className={STATUS_CLASS[p.status] ?? ""}>
                        {p.statusText}
                      </Badge>
                      {p.chosen && (
                        <Badge variant="outline" className="border-emerald-300 text-emerald-700">
                          你选的
                        </Badge>
                      )}
                      {p.fromConflict && (
                        <Badge variant="outline" className="border-orange-300 text-orange-700">
                          来源冲突·各自成案
                        </Badge>
                      )}
                      <Badge variant="outline" className="ml-auto border-slate-300 text-slate-700">
                        可信度上限 <Confidence value={p.maxConfidence} />
                      </Badge>
                    </div>
                    <CardDescription>
                      目标：{p.objective}
                      {p.preference ? ` · 假设偏好：${p.preference}` : " · **未标注假设的偏好**"}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-3">
                    <div className="grid gap-3 lg:grid-cols-2">
                      <div>
                        <div className="mb-1 text-xs text-muted-foreground">代价（可比维度）</div>
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>维度</TableHead>
                              <TableHead className="text-right">值</TableHead>
                              <TableHead>方向</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {(p.metrics ?? []).map((m) => (
                              <TableRow key={m.name}>
                                <TableCell className="text-xs">{m.name}</TableCell>
                                <TableCell className="text-right tabular-nums">
                                  <Quantity value={String(m.value)} unit={m.unit} />
                                </TableCell>
                                <TableCell className="text-xs text-muted-foreground">
                                  {m.lowerIsBetter ? "越小越好" : "越大越好"}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                        {/* 机会成本必须显式给出：它是取舍里最容易被忽略的一项 */}
                        <p className="mt-2 text-xs">
                          <span className="text-muted-foreground">机会成本：</span>
                          {p.opportunity}
                        </p>
                      </div>
                      <div className="grid gap-2 text-xs">
                        {(p.actions ?? []).length > 0 && (
                          <div>
                            <div className="mb-1 text-muted-foreground">行动</div>
                            <ul className="list-disc pl-4">
                              {(p.actions ?? []).map((x) => (
                                <li key={x}>{x}</li>
                              ))}
                            </ul>
                          </div>
                        )}
                        {(p.constraints ?? []).length > 0 && (
                          <div>
                            <div className="mb-1 text-muted-foreground">遵守的约束</div>
                            <div>{(p.constraints ?? []).join("、")}</div>
                          </div>
                        )}
                        {(p.assumptions ?? []).length > 0 && (
                          <div>
                            <div className="mb-1 text-muted-foreground">假设（变化时方案可能失效）</div>
                            <div>{(p.assumptions ?? []).join("、")}</div>
                          </div>
                        )}
                        <Separator />
                        <div className="text-muted-foreground">
                          未核验 {(p.unverifiedRatio * 100).toFixed(0)}%
                        </div>
                        {(p.depends ?? []).length > 0 && (
                          <div className="text-muted-foreground">
                            依据：
                            <ul className="ml-4 list-disc">
                              {(p.depends ?? []).map((x) => (
                                <li key={x}>
                                  <Dep value={x} />
                                </li>
                              ))}
                            </ul>
                          </div>
                        )}
                        <div className="text-muted-foreground">
                          来源 {(p.sources ?? []).join("、")} · 生成于 {p.at}
                        </div>
                        {p.chosen && (
                          <div className="text-emerald-700">
                            由 <Actor id={p.chosenBy} /> 选定 · {p.chooseReason}
                          </div>
                        )}
                      </div>
                    </div>

                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={a.loading || !p.executable || p.chosen}
                        aria-label={`选定 ${p.title}`}
                        onClick={() => void a.choose(p.id)}
                      >
                        {p.chosen ? "已选定" : "选定这个"}
                      </Button>
                      {!p.executable && (
                        <span className="self-center text-xs text-rose-700">
                          依据已失效，不可执行
                        </span>
                      )}
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>

            {ev && (ev.pruned ?? []).length > 0 && (
              <div className="p-3">
                <div className="mb-2 text-xs text-muted-foreground">
                  被剪枝的备选——剪掉必须说得出是被谁支配的
                </div>
                {(ev.pruned ?? []).map((x) => (
                  <div key={x.plan.id} className="border-l-2 border-slate-300 pl-3 text-xs text-muted-foreground">
                    <b>{x.plan.title}</b>：{x.dominance}（{x.by}）
                  </div>
                ))}
              </div>
            )}
          </ScrollArea>
        </CardContent>
      </Card>

      <ScrollArea className="min-h-0">
        <div className="grid gap-3 pr-1">
          <Card>
            <CardHeader className="py-3">
              <CardTitle className="text-sm">对比</CardTitle>
              <CardDescription>
                **系统不替你做取舍**：它把取舍、代价、依据摊开，让选择可见。
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-2">
              <div className="grid gap-1">
                <Label htmlFor="alt-pref" className="text-xs text-muted-foreground">
                  你的偏好（留空表示未声明）
                </Label>
                <Input
                  id="alt-pref"
                  value={a.preference}
                  onChange={(e) => a.setPreference(e.target.value)}
                  placeholder="例：省时间 / 省资源"
                  className="h-8"
                />
                <p className="text-[11px] text-muted-foreground">
                  留空时，备选必须覆盖**至少两种偏好**——否则后端会拒绝，
                  因为它会变成「针对某个人的单一方案」，而你没说自己是谁。
                </p>
              </div>
              <div className="grid gap-1">
                <Label htmlFor="alt-constraints" className="text-xs text-muted-foreground">
                  必须满足的约束（逗号分隔）
                </Label>
                <Input
                  id="alt-constraints"
                  value={a.constraints}
                  onChange={(e) => a.setConstraints(e.target.value)}
                  placeholder="例：不花勾玉"
                  className="h-8"
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="alt-threshold" className="text-xs text-muted-foreground">
                  合并阈值（差异低于它就算同一个方案）
                </Label>
                <Input
                  id="alt-threshold"
                  type="number"
                  value={String(a.threshold)}
                  onChange={(e) => a.setThreshold(Number(e.target.value) || 0)}
                  className="h-8"
                />
                <p className="text-[11px] text-muted-foreground">
                  不合并会产生**伪多样性**：一排看着不同的方案，选哪个都一样。
                </p>
              </div>
              <Button size="sm" disabled={a.loading} onClick={() => void a.compare()}>
                对比并剪枝
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="py-3">
              <CardTitle className="text-sm">选定</CardTitle>
              <CardDescription>选定者必须是人，且要留下理由</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-2">
              <div className="grid gap-1">
                <Label htmlFor="alt-by" className="text-xs text-muted-foreground">
                  选定人（必须是人）
                </Label>
                <Input
                  id="alt-by"
                  value={sessionBy}
                  onChange={(e) => setSessionBy(e.target.value)}
                  placeholder="你的名字"
                  className="h-8"
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="alt-reason" className="text-xs text-muted-foreground">
                  理由（必填）
                </Label>
                <Input
                  id="alt-reason"
                  value={a.reason}
                  onChange={(e) => a.setReason(e.target.value)}
                  placeholder="例：时间更短，资源还能接受"
                  className="h-8"
                />
              </div>
              <p className="text-[11px] leading-relaxed text-muted-foreground">
                选定之后**其他备选仍在列表里**：偏好只影响顺序，不删除任何东西。
                理由会被保留——它是推断偏好的原料。
              </p>
            </CardContent>
          </Card>

          {ev && (ev.merged ?? []).length > 0 && (
            <Card>
              <CardHeader className="py-3">
                <CardTitle className="text-sm">被合并的组</CardTitle>
                <CardDescription>差异低于阈值——留着只会造成伪多样性</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-1 text-xs text-muted-foreground">
                {(ev.merged ?? []).map((g) => (
                  <div key={(g ?? []).join(",")}>{(g ?? []).join(" + ")}</div>
                ))}
              </CardContent>
            </Card>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

/** 提出方案。界面不提供「可信度」输入——它由依赖推导。 */
function ProposeForm({
  scenario,
  disabled,
  onSubmit,
}: {
  scenario: string;
  disabled: boolean;
  onSubmit: (input: import("../../../../bindings/github.com/ngnl5/ssot/internal/api/models").PlanProposalInput) => void;
}) {
  const [title, setTitle] = useState("");
  const [objective, setObjective] = useState("");
  const [preference, setPreference] = useState("");
  const [opportunity, setOpportunity] = useState("");
  const [constraints, setConstraints] = useState("");
  const [actions, setActions] = useState("");
  const [depends, setDepends] = useState("");
  const [sources, setSources] = useState("huijiwiki");

  const canSubmit =
    title.trim() && objective.trim() && opportunity.trim() && sources.trim();

  return (
    <div className="m-3 rounded-md border p-3">
      <div className="mb-2 text-sm font-medium">提出一个备选</div>
      <div className="grid gap-2">
        <Input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="标题（例：先刷御魂那条线）"
          className="h-8"
          aria-label="标题"
        />
        <Input
          value={objective}
          onChange={(e) => setObjective(e.target.value)}
          placeholder="它优化的目标（必填——不声明目标就无法被比较）"
          className="h-8"
          aria-label="目标"
        />
        <Input
          value={preference}
          onChange={(e) => setPreference(e.target.value)}
          placeholder="它假设的偏好（例：省时间）"
          className="h-8"
          aria-label="假设的偏好"
        />
        <Input
          value={opportunity}
          onChange={(e) => setOpportunity(e.target.value)}
          placeholder="机会成本（必填——放弃什么）"
          className="h-8"
          aria-label="机会成本"
        />
        <Input
          value={constraints}
          onChange={(e) => setConstraints(e.target.value)}
          placeholder="遵守的约束（逗号分隔）"
          className="h-8"
          aria-label="约束"
        />
        <Input
          value={actions}
          onChange={(e) => setActions(e.target.value)}
          placeholder="行动（逗号分隔）"
          className="h-8"
          aria-label="行动"
        />
        <Input
          value={depends}
          onChange={(e) => setDepends(e.target.value)}
          placeholder="依赖（assert:<ID> / formula:<名>@<版本>，逗号分隔）"
          className="h-8"
          aria-label="依赖"
        />
        <Input
          value={sources}
          onChange={(e) => setSources(e.target.value)}
          placeholder="来源（逗号分隔）"
          className="h-8"
          aria-label="来源"
        />
        <Button
          size="sm"
          disabled={disabled || !canSubmit}
          onClick={() =>
            onSubmit({
              scenario,
              title: title.trim(),
              objective: objective.trim(),
              preference: preference.trim(),
              opportunity: opportunity.trim(),
              constraints: splitList(constraints),
              actions: splitList(actions),
              assumptions: [],
              depends: splitList(depends),
              sources: splitList(sources),
              // 维度留空：界面不替人编维度，先提出再补
              metrics: [],
              fromConflict: false,
            } as unknown as import("../../../../bindings/github.com/ngnl5/ssot/internal/api/models").PlanProposalInput)
          }
        >
          提出
        </Button>
        <p className="text-[11px] leading-relaxed text-muted-foreground">
          方案的可信度不在这里填——它由依赖断言中的最低分级决定，
          允许人工上调就等于把核验分级绕过去了。
        </p>
      </div>
    </div>
  );
}

function splitList(s: string): string[] {
  return s
    .split(/[,，\n]/)
    .map((x) => x.trim())
    .filter(Boolean);
}
