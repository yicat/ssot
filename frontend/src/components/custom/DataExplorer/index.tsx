/**
 * 数据页：断言库浏览 + schema + 数据质量。
 *
 * 「质量」在这里有确切含义，不是形容词：
 *   双向漂移 —— 数据里有而 schema 没声明的字段 / schema 声明而数据里从未出现的字段
 *   填充率   —— 每个已声明字段覆盖了多少主体
 * 三块都要能追溯到**具体字段名**：只给一个「质量分」既说不出哪里有洞，
 * 也无法在洞补上之后证明它真的补上了。
 */
import { useEffect, useState } from "react";

import { Badge } from "../../ui/badge";
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
import { Tabs, TabsList, TabsTrigger } from "../../ui/tabs";
import { CONFIDENCE_HINT, STATUS_CLASS, STATUS_LABEL } from "../Review/useReview";
import { useDataExplorer } from "./useDataExplorer";

export default function DataExplorer() {
  const d = useDataExplorer();
  const [entities, setEntities] = useState<string[]>([]);
  const [tab, setTab] = useState("assertions");

  useEffect(() => {
    setEntities(d.quality.map((q) => q.entity));
  }, [d.quality]);

  const page = d.page;

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_24rem] gap-4 p-4">
      <Card className="flex min-h-0 flex-col gap-0 py-0">
        <CardHeader className="flex-row items-center gap-3 border-b py-2">
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value="assertions">断言库</TabsTrigger>
              <TabsTrigger value="quality">数据质量</TabsTrigger>
              <TabsTrigger value="schema">定义</TabsTrigger>
            </TabsList>
          </Tabs>
          <div className="ml-auto flex items-center gap-2 text-xs">
            <Label htmlFor="data-entity" className="text-xs text-muted-foreground">
              实体
            </Label>
            <Select
              value={d.entity || "__all"}
              onValueChange={(v) => d.setEntity(v === "__all" ? "" : v)}
            >
              <SelectTrigger id="data-entity" size="sm" className="w-32" aria-label="实体">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all">全部实体</SelectItem>
                {entities.map((e) => (
                  <SelectItem key={e} value={e}>
                    {e}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Label htmlFor="data-status" className="text-xs text-muted-foreground">
              状态
            </Label>
            <Select
              value={d.status || "__all"}
              onValueChange={(v) => d.setStatus(v === "__all" ? "" : v)}
            >
              <SelectTrigger id="data-status" size="sm" className="w-28" aria-label="状态">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all">全部状态</SelectItem>
                {Object.keys(STATUS_LABEL).map((k) => (
                  <SelectItem key={k} value={k}>
                    {STATUS_LABEL[k]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Label htmlFor="data-confidence" className="text-xs text-muted-foreground">
              分级
            </Label>
            <Select
              value={d.confidence || "__all"}
              onValueChange={(v) => d.setConfidence(v === "__all" ? "" : v)}
            >
              <SelectTrigger id="data-confidence" size="sm" className="w-24" aria-label="分级">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all">全部分级</SelectItem>
                {["L1", "L2", "L3", "L4"].map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}　{CONFIDENCE_HINT[c]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              value={d.predicate}
              onChange={(e) => d.setPredicate(e.target.value)}
              placeholder="谓词"
              className="h-8 w-28"
              aria-label="谓词"
            />
          </div>
        </CardHeader>

        <CardContent className="min-h-0 flex-1 overflow-hidden px-0">
          <ScrollArea className="h-full">
            {tab === "assertions" && (
              <>
                <Table>
                  <TableHeader className="sticky top-0 bg-card">
                    <TableRow>
                      <TableHead>主体</TableHead>
                      <TableHead>谓词</TableHead>
                      <TableHead>取值</TableHead>
                      <TableHead>分级</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>来源</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(page?.items ?? []).map((a) => (
                      <TableRow
                        key={a.id}
                        onClick={() => void d.openSubject(a.entity, a.subject)}
                        className={
                          "cursor-pointer " +
                          (d.selected?.subject === a.subject ? "bg-secondary" : "")
                        }
                      >
                        <TableCell className="whitespace-nowrap">
                          {a.entity}/{a.subject}
                        </TableCell>
                        <TableCell className="whitespace-nowrap">{a.predicate}</TableCell>
                        <TableCell className="max-w-[16rem] truncate">{a.value}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className="border-slate-300 text-slate-700">
                            {a.confidence}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={STATUS_CLASS[a.status] ?? ""}>
                            {STATUS_LABEL[a.status] ?? a.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">{a.source}</TableCell>
                      </TableRow>
                    ))}
                    {(page?.items ?? []).length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                          该筛选下没有断言。
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
                <div className="flex items-center gap-3 border-t px-3 py-2 text-xs text-muted-foreground">
                  <span>
                    共 <b className="tabular-nums text-foreground">{page?.total ?? 0}</b> 条，
                    当前显示 {(page?.offset ?? 0) + 1}–
                    {(page?.offset ?? 0) + (page?.items?.length ?? 0)}
                  </span>
                  <span className="truncate">{page?.where}</span>
                  <div className="ml-auto flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={d.loading || (page?.offset ?? 0) === 0}
                      onClick={() => void d.reload(Math.max(0, (page?.offset ?? 0) - d.pageSize))}
                    >
                      上一页
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={
                        d.loading ||
                        (page?.offset ?? 0) + (page?.items?.length ?? 0) >= (page?.total ?? 0)
                      }
                      onClick={() => void d.reload((page?.offset ?? 0) + d.pageSize)}
                    >
                      下一页
                    </Button>
                  </div>
                </div>
              </>
            )}

            {tab === "quality" && (
              <div className="grid gap-3 p-3">
                {d.quality.length === 0 && (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    schema 中还没有声明任何实体。
                  </p>
                )}
                {d.quality.map((q) => (
                  <Card key={q.entity}>
                    <CardHeader className="py-3">
                      <CardTitle className="text-sm">
                        {q.entity}
                        <span className="ml-2 text-xs font-normal text-muted-foreground">
                          {q.subjects} 个主体 · {q.assertions} 条断言
                        </span>
                      </CardTitle>
                      {(q.undeclared?.length ?? 0) > 0 && (
                        <CardDescription className="text-amber-700">
                          数据里有而 schema 未声明：
                          {(q.undeclared ?? []).join("、")}
                          ——这些字段会被准入层过滤掉，也就是「接进来了但没入库」
                        </CardDescription>
                      )}
                      {(q.requiredMissing?.length ?? 0) > 0 && (
                        <CardDescription className="text-rose-700">
                          必填却一条数据都没有：
                          {(q.requiredMissing ?? []).join("、")}
                        </CardDescription>
                      )}
                    </CardHeader>
                    <CardContent className="px-0">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>字段</TableHead>
                            <TableHead>声明</TableHead>
                            <TableHead className="text-right">覆盖主体</TableHead>
                            <TableHead className="text-right">覆盖率</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(q.fields ?? []).map((f) => (
                            <TableRow key={f.key}>
                              <TableCell className="font-mono text-xs">
                                {f.key}
                                {f.required && (
                                  <span className="ml-1 text-rose-700" title="必填">
                                    *
                                  </span>
                                )}
                              </TableCell>
                              <TableCell>
                                {f.declared ? (
                                  <Badge variant="outline" className="border-slate-300 text-slate-700">
                                    已声明
                                  </Badge>
                                ) : (
                                  <Badge variant="outline" className="border-amber-300 text-amber-700">
                                    未声明
                                  </Badge>
                                )}
                              </TableCell>
                              <TableCell className="text-right tabular-nums">{f.subjects}</TableCell>
                              <TableCell className="text-right tabular-nums">
                                {(f.coverage * 100).toFixed(0)}%
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}

            {tab === "schema" && (
              <div className="grid gap-3 p-3">
                {d.schemas.length === 0 && (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    schema 中还没有声明任何实体。
                  </p>
                )}
                {d.schemas.map((es) => (
                  <Card key={es.entity}>
                    <CardHeader className="py-3">
                      <CardTitle className="text-sm">{es.entity}</CardTitle>
                      <CardDescription>
                        {es.description || "（未写描述）"} · schemaRev {es.schemaRev || "—"}
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="px-0">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>字段</TableHead>
                            <TableHead>类型</TableHead>
                            <TableHead>单位 / 目标</TableHead>
                            <TableHead>约束</TableHead>
                            <TableHead>说明</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(es.fields ?? []).map((f) => (
                            <TableRow key={f.key}>
                              <TableCell className="font-mono text-xs">{f.key}</TableCell>
                              <TableCell className="text-xs">{f.type}</TableCell>
                              <TableCell className="text-xs">
                                {f.unit || f.target || "—"}
                              </TableCell>
                              <TableCell className="text-xs">
                                {[
                                  f.identity ? "身份" : "",
                                  f.required ? "必填" : "",
                                  f.unique ? "唯一" : "",
                                ]
                                  .filter(Boolean)
                                  .join(" ") || "—"}
                              </TableCell>
                              <TableCell className="text-xs text-muted-foreground">
                                {f.description}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
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
          {!d.selected ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">点一条断言看它的主体</CardTitle>
              </CardHeader>
              <CardContent className="text-xs leading-relaxed text-muted-foreground">
                这里看的是**数据本身**，不是核验。核验在「核验」分区里，
                那里才记录「谁、何时、凭什么」。
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardHeader className="py-3">
                <CardTitle className="text-base">
                  {d.selected.entity}/{d.selected.subject}
                </CardTitle>
                <CardDescription>
                  该主体上的 {d.detail.length} 条断言——同一主体的全部说法都摆出来
                </CardDescription>
              </CardHeader>
              <CardContent className="grid gap-2">
                {d.detail.map((a) => (
                  <div key={a.id} className="border-l-2 pl-3 text-xs">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{a.predicate}</span>
                      <span className="tabular-nums">{a.value}</span>
                      <Badge variant="outline" className="ml-auto border-slate-300 text-slate-700">
                        {a.confidence}
                      </Badge>
                    </div>
                    <div className="text-muted-foreground">
                      {STATUS_LABEL[a.status] ?? a.status} · {a.artifact} {a.anchor} @ {a.revision}
                    </div>
                  </div>
                ))}
                <Separator />
                <p className="text-[11px] text-muted-foreground">
                  限定条件不同的断言会并存，互不覆盖——同一件事在不同条件下不同效果，
                  那不是指代问题。
                </p>
              </CardContent>
            </Card>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
