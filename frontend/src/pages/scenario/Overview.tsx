/**
 * 场景概览：需要什么、有什么、缺什么、能不能跑。
 *
 * 这一页回答的是场景存在的理由——把数据缺口从「事后发现」变成「事前可见」。
 * 因此 requires 必须**逐项**列出，而不是给一个总数。
 */
import { useEffect, useState } from "react";

import { Overview } from "../../../bindings/github.com/ngnl5/ssot/internal/api/scenarioservice";
import type { ScenarioOverview as ScenarioOverviewData } from "../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { Badge } from "../../components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../components/ui/card";
import { ScrollArea } from "../../components/ui/scroll-area";
import { Separator } from "../../components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../components/ui/table";

export default function ScenarioOverview({ scenario }: { scenario: string | null }) {
  const [data, setData] = useState<ScenarioOverviewData | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!scenario) {
      setData(null);
      setError(null);
      return;
    }
    let alive = true;
    Overview(scenario)
      .then((d) => {
        if (alive) {
          setData(d);
          setError(null);
        }
      })
      .catch((e) => {
        if (alive) {
          setData(null);
          setError(String(e));
        }
      });
    return () => {
      alive = false;
    };
  }, [scenario]);

  if (!scenario) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        当前项目没有场景，或尚未选择场景。场景是「缺什么」的载体——
        没有场景就没有数据缺口的事前可见性。
      </div>
    );
  }
  if (error) {
    return <div className="p-6 text-sm text-rose-700">加载场景失败：{error}</div>;
  }
  if (!data) {
    return <div className="p-6 text-sm text-muted-foreground">加载中…</div>;
  }

  return (
    <ScrollArea className="min-h-0 flex-1">
      <div className="grid gap-4 p-4">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-lg font-semibold">{data.name}</h1>
          <Badge
            variant="outline"
            className={
              data.runnable ? "border-emerald-300 text-emerald-700" : "border-rose-300 text-rose-700"
            }
          >
            {data.runnable ? "可以运行" : "不可运行"}
          </Badge>
          <p className="text-xs text-muted-foreground">{data.description}</p>
        </div>

        {!data.runnable && (
          <Card className="border-rose-200 bg-rose-50">
            <CardContent className="pt-4 text-sm text-rose-800">
              <div className="font-medium">requires 未满足，**拒绝运行**</div>
              <ul className="mt-2 list-disc pl-5 text-xs">
                {(data.missing ?? []).map((m) => (
                  <li key={m}>{m}</li>
                ))}
              </ul>
              <p className="mt-2 text-xs">
                不得静默降级，也不得用不完整数据产出方案——缺的部分必须显式标注。
              </p>
            </CardContent>
          </Card>
        )}

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">需要什么（requires）</CardTitle>
            <CardDescription>库里**应当有**的数据；缺失即不可运行</CardDescription>
          </CardHeader>
          <CardContent className="px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>需求</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead className="text-right">现有 / 总数</TableHead>
                  <TableHead className="text-right">覆盖率</TableHead>
                  <TableHead>说明</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(data.requires ?? []).map((q) => (
                  <TableRow key={q.want}>
                    <TableCell className="font-mono text-xs">{q.want}</TableCell>
                    <TableCell>
                      <Badge
                        variant="outline"
                        className={
                          q.status === "satisfied"
                            ? "border-emerald-300 text-emerald-700"
                            : "border-rose-300 text-rose-700"
                        }
                      >
                        {q.status === "satisfied" ? "已满足" : q.status === "missing" ? "缺失" : "不可用"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {q.have} / {q.total}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {q.total > 0 ? `${(q.coverage * 100).toFixed(0)}%` : "—"}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{q.detail}</TableCell>
                  </TableRow>
                ))}
                {(data.requires ?? []).length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-muted-foreground">
                      该场景没有声明 requires
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <div className="grid gap-3 lg:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">外部输入（inputs）</CardTitle>
              <CardDescription>库**本来就不该有**的值，由调用方在运行时给出</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              {(data.inputs ?? []).length === 0 && (
                <p className="text-xs text-muted-foreground">该场景不需要外部输入。</p>
              )}
              {(data.inputs ?? []).map((inp) => (
                <div key={inp.name} className="text-sm">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs">{inp.name}</span>
                    <Badge variant="outline" className="border-slate-300 text-slate-700">
                      {inp.unit}
                    </Badge>
                    {(inp.min !== null && inp.min !== undefined) ||
                    (inp.max !== null && inp.max !== undefined) ? (
                      <span className="text-[11px] text-muted-foreground">
                        范围 {inp.min ?? "-∞"} ~ {inp.max ?? "+∞"}
                      </span>
                    ) : null}
                  </div>
                  <p className="mt-1 text-[11px] leading-snug text-muted-foreground">
                    {inp.description}
                  </p>
                </div>
              ))}
              <Separator />
              <p className="text-[11px] leading-relaxed text-muted-foreground">
                外部输入**始终标注未核验**：它与断言的区别是没有来源。
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">依赖公式</CardTitle>
              <CardDescription>算例未通过的公式**拒绝使用**</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              {(data.formulas ?? []).length === 0 && (
                <p className="text-xs text-muted-foreground">该场景不依赖公式。</p>
              )}
              {(data.formulas ?? []).map((f) => (
                <div key={f.name}>
                  <div className="flex items-center gap-2 text-sm">
                    <span className="font-mono text-xs">{f.name}</span>
                    <Badge variant="outline" className={formulaClass(f.status)}>
                      {formulaLabel(f.status)}
                    </Badge>
                  </div>
                  <ul className="mt-1 grid gap-1">
                    {(f.cases ?? []).map((c, i) => (
                      <li key={i} className="text-[11px] text-muted-foreground">
                        {c.passed ? "✓" : "✗"} {c.name}——{c.detail}
                      </li>
                    ))}
                    {(f.cases ?? []).length === 0 && (
                      <li className="text-[11px] text-amber-700">
                        没有算例：允许使用，但产出必须标注「公式未验证」
                      </li>
                    )}
                  </ul>
                </div>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">产出</CardTitle>
              <CardDescription>这个场景算什么</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-2">
              {(data.outputs ?? []).length === 0 ? (
                <p className="text-xs text-muted-foreground">未声明 outputs。</p>
              ) : (
                (data.outputs ?? []).map((o) => (
                  <div key={o} className="font-mono text-xs">
                    {o}
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  );
}

function formulaLabel(s: string): string {
  switch (s) {
    case "verified":
      return "算例全过";
    case "unverified":
      return "无算例";
    case "failed":
      return "算例未过";
    default:
      return "不存在";
  }
}

function formulaClass(s: string): string {
  switch (s) {
    case "verified":
      return "border-emerald-300 text-emerald-700";
    case "failed":
      return "border-rose-300 text-rose-700";
    default:
      return "border-amber-300 text-amber-700";
  }
}
