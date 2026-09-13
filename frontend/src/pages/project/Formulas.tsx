/**
 * 公式页：公式清单与算例验证状态。
 *
 * 三级状态必须一眼可分：
 *   算例全过 -> 已验证，可用
 *   没有算例 -> 允许用，但产出必须标注「公式未验证」
 *   算例未过 -> **拒绝使用**
 *
 * 把「无算例」显示成绿色，等于把未验证的公式伪装成验证过的。
 */
import { useEffect, useState } from "react";

import { List } from "../../../bindings/github.com/ngnl5/ssot/internal/api/formulaservice";
import type { FormulaView } from "../../../bindings/github.com/ngnl5/ssot/internal/api/models";
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

const STATUS_LABEL: Record<string, string> = {
  verified: "算例全过",
  unverified: "无算例（未验证）",
  failed: "算例未过（拒绝使用）",
  unreadable: "读不出来",
};

const STATUS_CLASS: Record<string, string> = {
  verified: "border-emerald-300 text-emerald-700",
  unverified: "border-amber-300 text-amber-700",
  failed: "border-rose-300 text-rose-700",
  unreadable: "border-rose-300 text-rose-700",
};

export default function FormulaList() {
  const [items, setItems] = useState<FormulaView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    List()
      .then((v) => {
        setItems(v ?? []);
        setError(null);
      })
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="p-6 text-sm text-muted-foreground">加载中…</div>;
  if (error) return <div className="p-6 text-sm text-rose-700">加载公式失败：{error}</div>;

  return (
    <ScrollArea className="min-h-0 flex-1">
      <div className="grid gap-4 p-4">
        <div>
          <h1 className="text-lg font-semibold">公式</h1>
          <p className="text-xs text-muted-foreground">
            「不能错」的最后一环是算对。数据再准，公式算错，结果就是错的——
            因此公式必须透明、可测、且状态可见。
          </p>
        </div>

        {items.length === 0 && (
          <Card className="border-dashed">
            <CardContent className="pt-4 text-sm text-muted-foreground">
              这个项目还没有公式。公式放在项目的 formulas/ 目录下。
            </CardContent>
          </Card>
        )}

        {items.map((f) => (
          <Card key={f.file}>
            <CardHeader className="py-3">
              <div className="flex flex-wrap items-center gap-2">
                <CardTitle className="text-base">{f.name}</CardTitle>
                <Badge variant="outline" className={STATUS_CLASS[f.status] ?? ""}>
                  {STATUS_LABEL[f.status] ?? f.status}
                </Badge>
                {f.version && (
                  <span className="text-xs text-muted-foreground">版本 {f.version}</span>
                )}
                {(f.usedBy?.length ?? 0) > 0 && (
                  <span className="text-xs text-muted-foreground">
                    被场景 {(f.usedBy ?? []).join("、")} 使用
                  </span>
                )}
              </div>
              <CardDescription>{f.description}</CardDescription>
              <div className="text-[11px] text-muted-foreground">{f.file}</div>
            </CardHeader>
            <CardContent className="grid gap-3">
              {f.error && (
                <div className="rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-800">
                  {f.error}
                </div>
              )}

              {!f.error && (
                <>
                  {/* 公式原文必须显示：不透明的公式没法被审阅 */}
                  <div>
                    <div className="mb-1 text-xs text-muted-foreground">结果表达式</div>
                    <pre className="overflow-x-auto rounded bg-muted/60 p-2 text-xs">
                      {f.result}
                    </pre>
                  </div>

                  <div className="grid gap-3 lg:grid-cols-2">
                    <div>
                      <div className="mb-1 text-xs text-muted-foreground">
                        参数（静态类型，撰写期检查用）
                      </div>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>名称</TableHead>
                            <TableHead>类型</TableHead>
                            <TableHead>单位</TableHead>
                            <TableHead>说明</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(f.params ?? []).map((p) => (
                            <TableRow key={p.name}>
                              <TableCell className="font-mono text-xs">{p.name}</TableCell>
                              <TableCell className="text-xs">{p.type}</TableCell>
                              <TableCell className="text-xs">{p.unit || "—"}</TableCell>
                              <TableCell className="text-xs text-muted-foreground">
                                {p.note}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>

                    <div>
                      <div className="mb-1 text-xs text-muted-foreground">
                        取值来源（绑定名 → 路径）
                      </div>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>绑定</TableHead>
                            <TableHead>来源路径</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(f.bindings ?? []).map((b) => (
                            <TableRow key={b.name}>
                              <TableCell className="font-mono text-xs">{b.name}</TableCell>
                              <TableCell className="font-mono text-xs">{b.path}</TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  </div>

                  <Separator />

                  <div>
                    <div className="mb-1 text-xs text-muted-foreground">
                      算例（{f.cases?.length ?? 0} 条）
                    </div>
                    {(f.cases ?? []).length === 0 ? (
                      <p className="text-xs text-amber-700">
                        没有算例。允许使用，但**产出必须标注「公式未验证」**——
                        这不等于已验证。
                      </p>
                    ) : (
                      <ul className="grid gap-1">
                        {(f.cases ?? []).map((c, i) => (
                          <li key={i} className="text-xs">
                            <span className={c.passed ? "text-emerald-700" : "text-rose-700"}>
                              {c.passed ? "✓" : "✗"}
                            </span>{" "}
                            {c.name}
                            <span className="text-muted-foreground">　{c.detail}</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                </>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </ScrollArea>
  );
}
