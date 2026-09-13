/**
 * 来源页：不能变成数据的那部分事实源。
 *
 * 这一页回答的是：**每条断言的依据到底在哪。** 此前溯源的终点是一个字符串——
 * 指不到任何东西，也不知道它变没变。现在它指向一份文档。
 *
 * 界面上刻意有的东西：
 *  - 每份文档**被多少条断言引用**——影响面决定了它值不值得核验
 *  - **依据未登记**的原件数——剩余多少必须可见，不能靠人感觉
 *  - 修订历史：更正只追加，旧修订保留
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
import { Tabs, TabsList, TabsTrigger } from "../../ui/tabs";
import { Actor, AssertionStatus, Confidence, Field, Subject } from "../Term";
import { DOC_STATUS_CLASS, useSourcesStore } from "./store";
import { useSources } from "./useSources";

const KINDS = [
  { value: "evidence", label: "依据", hint: "原文本身，断言从它抽取" },
  { value: "rule", label: "规则", hint: "机制说明，解释世界怎么运转" },
  { value: "explanation", label: "说明", hint: "为什么这样定义、为什么这么算" },
  { value: "change", label: "变更", hint: "版本叙事、公告" },
  { value: "unmodeled", label: "未建模", hint: "引擎表达不了但必须记录的整体" },
];

export default function Sources() {
  const s = useSources(null);
  const by = useSourcesStore((x) => x.by);
  const setBy = useSourcesStore((x) => x.setBy);
  const [tab, setTab] = useState("list");

  const selected = s.docs.find((d) => d.revId === s.selectedRevId) ?? null;
  const unregisteredTotal = s.unregistered.reduce((n, r) => n + r.assertions, 0);

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_30rem] gap-4 p-4">
      <Card className="flex min-h-0 flex-col gap-0 py-0">
        <CardHeader className="flex-row items-center gap-3 border-b py-2">
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value="list">文档（{s.docs.length}）</TabsTrigger>
              <TabsTrigger value="unregistered">
                未登记依据{unregisteredTotal > 0 ? `（${unregisteredTotal}）` : ""}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <div className="ml-auto flex items-center gap-2">
            <Button size="sm" variant="outline" disabled={s.loading} onClick={() => void s.reload()}>
              刷新
            </Button>
          </div>
        </CardHeader>

        <CardContent className="min-h-0 flex-1 overflow-hidden px-0">
          <ScrollArea className="h-full">
            {tab === "list" && (
              <>
                <Table>
                  <TableHeader className="sticky top-0 bg-card">
                    <TableRow>
                      <TableHead>类型</TableHead>
                      <TableHead>文档</TableHead>
                      <TableHead>修订</TableHead>
                      <TableHead className="text-right">被引用</TableHead>
                      <TableHead>核验</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {s.docs.map((d) => (
                      <TableRow
                        key={d.revId}
                        onClick={() => void s.open(d.revId)}
                        className={"cursor-pointer " + (s.selectedRevId === d.revId ? "bg-secondary" : "")}
                      >
                        <TableCell className="whitespace-nowrap">
                          <Badge variant="outline" className="border-slate-300 text-slate-700">
                            {d.kindText}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs">
                          <div className="font-mono">{d.title}</div>
                          <div className="text-muted-foreground">
                            {d.source}
                            {(d.appliesScenarios ?? []).length > 0
                              ? ` · 仅限 ${(d.appliesScenarios ?? []).join("、")}`
                              : " · 全项目适用"}
                          </div>
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-xs">
                          {d.revision}
                          <div className="font-mono text-[10px] text-muted-foreground">{d.hash}</div>
                        </TableCell>
                        <TableCell className="text-right tabular-nums">{d.usageCount}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={DOC_STATUS_CLASS[d.status] ?? ""}>
                            {d.statusText}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                    {s.docs.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="py-10 text-center text-muted-foreground">
                          <div>还没有登记任何文档。</div>
                          <div className="mt-1 text-xs">
                            跑一次 sync 会把归档的原件登记进来；机制说明这类人写的文档在这里手工登记。
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </>
            )}

            {tab === "unregistered" && (
              <div className="p-3">
                {s.unregistered.length === 0 ? (
                  <div className="py-8 text-center text-sm text-muted-foreground">
                    <div>没有未登记的依据——每条断言的溯源都指得到一份文档。</div>
                    <div className="mt-1 text-xs">
                      这正是这一页存在的意义：**溯源的终点不该是一个字符串**。
                    </div>
                  </div>
                ) : (
                  <>
                    <p className="mb-3 text-xs text-muted-foreground">
                      这些原件还没有对应的文档，因此「它在讲什么、还在不在、变没变」都无从判断。
                    </p>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>原件</TableHead>
                          <TableHead className="text-right">引用它的断言</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {s.unregistered.map((r) => (
                          <TableRow key={r.artifact}>
                            <TableCell className="font-mono text-xs">{r.artifact}</TableCell>
                            <TableCell className="text-right tabular-nums">{r.assertions}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </>
                )}
              </div>
            )}
          </ScrollArea>
        </CardContent>
      </Card>

      <ScrollArea className="min-h-0">
        <div className="grid gap-3 pr-1">
          {!selected ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">从左侧选一份文档</CardTitle>
              </CardHeader>
              <CardContent className="text-xs leading-relaxed text-muted-foreground">
                文档与断言并列，**不参与计算，只作为依据**。
                <br />
                它的价值在于：任何一条断言被质疑时，能回到原文。
                <br />
                <Separator className="my-3" />
                可拆成 (主体, 谓词, 取值) 的东西进断言库；
                **拆了就失真或根本拆不了的**，留在这里。
              </CardContent>
            </Card>
          ) : (
            <>
              <Card>
                <CardHeader className="py-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline" className="border-slate-300 text-slate-700">
                      {selected.kindText}
                    </Badge>
                    <Badge variant="outline" className={DOC_STATUS_CLASS[selected.status] ?? ""}>
                      {selected.statusText}
                    </Badge>
                    {!selected.current && (
                      <Badge variant="outline" className="border-slate-300 text-slate-500">
                        历史修订
                      </Badge>
                    )}
                  </div>
                  <CardTitle className="break-all font-mono text-sm">{selected.title}</CardTitle>
                  <CardDescription>
                    {selected.source} · 修订 {selected.revision} · 被 {selected.usageCount} 条断言引用
                  </CardDescription>
                </CardHeader>
                <CardContent className="grid gap-1 text-xs">
                  <Row k="内容摘要" v={selected.hash} mono />
                  <Row k="采集时间" v={selected.capturedAt} />
                  <Row k="登记时间" v={selected.registeredAt} />
                  <Row k="归档路径" v={selected.artifactPath || "（自撰文档，正文在库里）"} mono />
                  <Row
                    k="适用范围"
                    v={
                      (selected.appliesScenarios ?? []).length > 0
                        ? `仅限 ${(selected.appliesScenarios ?? []).join("、")}`
                        : "全项目适用"
                    }
                  />
                  {(selected.appliesVersions ?? []).length > 0 && (
                    <Row k="适用版本" v={(selected.appliesVersions ?? []).join("、")} />
                  )}
                  <Row
                    k="核验"
                    v={selected.verifiedBy ? <Actor id={`${selected.verifiedBy.kind}:${selected.verifiedBy.id}`} /> : "尚未核验"}
                  />
                  {selected.reason && <Row k="理由" v={selected.reason} />}
                </CardContent>
              </Card>

              {selected.body && (
                <Card>
                  <CardHeader className="py-3">
                    <CardTitle className="text-sm">原文</CardTitle>
                    <CardDescription>正文不可改——更正只能登记新修订</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded bg-muted/60 p-2 text-[11px] leading-relaxed">
                      {selected.body}
                    </pre>
                  </CardContent>
                </Card>
              )}

              {/* 核验：人 + 理由，方法固定为编审 */}
              {selected.status === "unverified" && (
                <Card>
                  <CardHeader className="py-3">
                    <CardTitle className="text-sm">核验这份原文</CardTitle>
                    <CardDescription>
                      核验的是「这份原文可信」，**不是**「原文里的事实已被验证」——两者必须分开。
                      一个人读过并背书不构成「多源」，因此方法只能是编审。
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-2">
                    <div className="grid gap-1">
                      <Label htmlFor="doc-by" className="text-xs text-muted-foreground">
                        核验人（必须是人）
                      </Label>
                      <Input
                        id="doc-by"
                        value={by}
                        onChange={(e) => setBy(e.target.value)}
                        placeholder="你的名字"
                        className="h-8"
                      />
                    </div>
                    <div className="grid gap-1">
                      <Label htmlFor="doc-reason" className="text-xs text-muted-foreground">
                        理由（必填）
                      </Label>
                      <Input
                        id="doc-reason"
                        value={s.reason}
                        onChange={(e) => s.setReason(e.target.value)}
                        placeholder="例：通读一遍，与游戏内一致"
                        className="h-8"
                      />
                    </div>
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        className="flex-1"
                        disabled={s.loading}
                        onClick={() => void s.verify(selected.revId)}
                      >
                        核验
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1 border-rose-300 text-rose-700"
                        disabled={s.loading}
                        onClick={() => void s.reject(selected.revId)}
                      >
                        驳回
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              )}

              {/* 引用它的断言 */}
              <Card>
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">引用它的断言（{s.usage.length}）</CardTitle>
                  <CardDescription>
                    关联方式是**文档标题**——断言的溯源里写的原件标识就是它
                  </CardDescription>
                </CardHeader>
                <CardContent className="px-0">
                  {s.usage.length === 0 ? (
                    <p className="px-4 text-xs text-muted-foreground">
                      还没有断言引用这份文档。它可能是一份人写的说明，
                      或者它的断言还没被接入。
                    </p>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>主体</TableHead>
                          <TableHead>字段</TableHead>
                          <TableHead>取值</TableHead>
                          <TableHead>分级</TableHead>
                          <TableHead>状态</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {s.usage.slice(0, 50).map((a) => (
                          <TableRow key={a.id}>
                            <TableCell className="whitespace-nowrap text-xs">
                              <Subject entity={a.entity} subject={a.subject} />
                            </TableCell>
                            <TableCell className="text-xs">
                              <Field entity={a.entity} predicate={a.predicate} short />
                            </TableCell>
                            <TableCell className="max-w-[10rem] truncate text-xs">{a.value}</TableCell>
                            <TableCell className="text-xs">
                              <Confidence value={a.confidence} />
                            </TableCell>
                            <TableCell className="text-xs">
                              <AssertionStatus value={a.status} />
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                  {s.usage.length > 50 && (
                    <p className="px-4 pt-2 text-[11px] text-muted-foreground">
                      …另有 {s.usage.length - 50} 条
                    </p>
                  )}
                </CardContent>
              </Card>

              {/* 修订历史 */}
              <Card>
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">修订历史（{s.history.length}）</CardTitle>
                  <CardDescription>更正只追加——旧修订保留，引用它的断言仍指向它</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-2">
                  {s.history.map((h) => (
                    <button
                      key={h.revId}
                      type="button"
                      onClick={() => void s.open(h.revId)}
                      className="grid gap-0.5 border-l-2 pl-3 text-left text-xs hover:bg-secondary/40"
                    >
                      <div className="flex items-center gap-2">
                        <span>{h.revision}</span>
                        <Badge variant="outline" className={DOC_STATUS_CLASS[h.status] ?? ""}>
                          {h.statusText}
                        </Badge>
                        {h.current && (
                          <Badge variant="outline" className="border-sky-300 text-sky-700">
                            当前
                          </Badge>
                        )}
                      </div>
                      <div className="font-mono text-[10px] text-muted-foreground">
                        {h.hash} · {h.registeredAt}
                      </div>
                      {h.supersededBy && (
                        <div className="text-[10px] text-muted-foreground">
                          被 {h.supersededBy} 取代
                        </div>
                      )}
                    </button>
                  ))}
                </CardContent>
              </Card>

              <RegisterForm disabled={s.loading} onSubmit={s.register} />
            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

function Row({ k, v, mono = false }: { k: string; v: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex gap-2">
      <dt className="w-20 shrink-0 text-muted-foreground">{k}</dt>
      <dd className={"break-all " + (mono ? "font-mono text-[11px]" : "")}>{v}</dd>
    </div>
  );
}

/**
 * 登记一份自撰文档。
 *
 * 这是「不可拆的事实源」进系统的入口：机制说明、依据声明、版本叙事。
 */
function RegisterForm({
  disabled,
  onSubmit,
}: {
  disabled: boolean;
  onSubmit: (input: import("../../../../bindings/github.com/ngnl5/ssot/internal/api/models").DocInput) => void;
}) {
  const [title, setTitle] = useState("");
  const [kind, setKind] = useState("explanation");
  const [revision, setRevision] = useState("v1");
  const [body, setBody] = useState("");
  const [scenarios, setScenarios] = useState("");

  const canSubmit = title.trim() && revision.trim() && body.trim();

  return (
    <Card>
      <CardHeader className="py-3">
        <CardTitle className="text-sm">登记一份自撰文档</CardTitle>
        <CardDescription>
          机制说明、依据声明、版本叙事——**拆不出 (主体, 谓词, 取值) 的东西留在这里**，
          不要为了「变成数据」而强行拆。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-2">
        <Input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="标题（例：防御减免为何无权威来源）"
          className="h-8"
          aria-label="标题"
        />
        <div className="flex gap-2">
          <Select value={kind} onValueChange={setKind}>
            <SelectTrigger className="h-8 w-32" aria-label="类型">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {KINDS.map((k) => (
                <SelectItem key={k.value} value={k.value}>
                  {k.label}　{k.hint}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            value={revision}
            onChange={(e) => setRevision(e.target.value)}
            placeholder="修订"
            className="h-8 w-24"
            aria-label="修订"
          />
        </div>
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="原文（必填——自撰文档没有归档可依，正文只能存在这里）"
          aria-label="原文"
          className="min-h-24 w-full rounded-md border border-input bg-transparent p-2 text-xs"
        />
        <Input
          value={scenarios}
          onChange={(e) => setScenarios(e.target.value)}
          placeholder="适用场景（逗号分隔；留空表示全项目适用）"
          className="h-8"
          aria-label="适用场景"
        />
        <Button
          size="sm"
          disabled={disabled || !canSubmit}
          onClick={() =>
            onSubmit({
              source: "内部",
              title: title.trim(),
              kind,
              revision: revision.trim(),
              body,
              artifactPath: "",
              contentHash: "",
              // 自撰文档的采集时间就是写下它的时刻——它不是从别处抓来的
              capturedAt: new Date().toISOString(),
              appliesVersions: [],
              appliesScenarios: scenarios
                .split(/[,，\n]/)
                .map((x) => x.trim())
                .filter(Boolean),
            } as unknown as import("../../../../bindings/github.com/ngnl5/ssot/internal/api/models").DocInput)
          }
        >
          登记
        </Button>
        <p className="text-[11px] leading-relaxed text-muted-foreground">
          登记之后**正文不可改**：更正只能登记新修订，旧修订保留。
          改了修订，引用这份文档的断言会回到待核验——变更不会被静默吞掉。
        </p>
      </CardContent>
    </Card>
  );
}
