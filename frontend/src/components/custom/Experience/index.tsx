/**
 * 经验页：场景下累积的判断，以及产生它们的会话记录。
 *
 * 界面上刻意**没有**的东西，与界面上有的东西同样重要：
 *  - 没有「修改级别」：级别由参与者与依据推导，不给人填错的机会
 *  - 没有「删除条目」：驳回只是改状态，条目保留
 *  - 没有「编辑发言」：会话记录是依据，可删改的依据不是依据
 */
import { useState } from "react";

import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../ui/card";
import { Input } from "../../ui/input";
import { Label } from "../../ui/label";
import { ScrollArea } from "../../ui/scroll-area";
import { Separator } from "../../ui/separator";
import { Tabs, TabsList, TabsTrigger } from "../../ui/tabs";
import type { ProposalInput } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useSessionStore } from "../Session/store";
import { useExperience } from "./useExperience";
import { useGlossary } from "../Session/glossary";
import { Actor, Dep } from "../Term";
import { KINDS, LEVEL_CLASS, STATUS_CLASS } from "./store";

export default function Experience({ scenario }: { scenario: string | null }) {
  const e = useExperience(scenario);
  // 使用者身份是**会话级**的：在核验里填一次，这里不该再填一次。
  const by = useSessionStore((s) => s.by);
  const g = useGlossary();
  const setBy = useSessionStore((s) => s.setBy);
  const [tab, setTab] = useState("entries");
  const [title, setTitle] = useState("");

  if (!scenario) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        当前项目没有场景，或尚未选择场景。经验挂在场景下——它围绕用途，
        与上下文中立的派生断言不同。
      </div>
    );
  }

  const selected = e.entries.find((x) => x.id === e.selectedId) ?? null;
  const session = e.sessions.find((x) => x.id === e.sessionId) ?? e.sessions[0] ?? null;

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[1fr_28rem] gap-4 p-4">
      <Card className="flex min-h-0 flex-col gap-0 py-0">
        <CardHeader className="flex-row items-center gap-3 border-b py-2">
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value="entries">经验（{e.entries.length}）</TabsTrigger>
              <TabsTrigger value="conflicts">
                冲突{e.conflicts.length > 0 ? `（${e.conflicts.length}）` : ""}
              </TabsTrigger>
              <TabsTrigger value="sessions">会话记录（{e.sessions.length}）</TabsTrigger>
            </TabsList>
          </Tabs>
          <div className="ml-auto flex items-center gap-2">
            <Button size="sm" variant="outline" disabled={e.loading} onClick={() => void e.refresh()}>
              重算依赖
            </Button>
            <Button size="sm" variant="outline" disabled={e.loading} onClick={() => void e.reload()}>
              刷新
            </Button>
          </div>
        </CardHeader>

        <CardContent className="min-h-0 flex-1 overflow-hidden px-0">
          <ScrollArea className="h-full">
            {tab === "entries" && (
              <table className="w-full text-sm">
                <tbody>
                  {e.entries.map((x) => (
                    <tr
                      key={x.id}
                      onClick={() => e.select(x.id)}
                      className={
                        "cursor-pointer border-b [&>td]:px-3 [&>td]:py-2 " +
                        (e.selectedId === x.id ? "bg-secondary" : "")
                      }
                    >
                      <td className="w-16">
                        <Badge variant="outline" className={LEVEL_CLASS[x.level] ?? ""}>
                          {x.level} 级
                        </Badge>
                      </td>
                      <td>
                        <div>{x.statement}</div>
                        <div className="text-[11px] text-muted-foreground">
                          {x.topic} · {x.kindText} · <Actor id={`${x.proposedBy.kind}:${x.proposedBy.id}`} />
                        </div>
                      </td>
                      <td className="w-24 text-right">
                        <Badge variant="outline" className={STATUS_CLASS[x.status] ?? ""}>
                          {x.statusText}
                        </Badge>
                      </td>
                    </tr>
                  ))}
                  {e.entries.length === 0 && (
                    <tr>
                      <td className="px-3 py-10 text-center text-muted-foreground">
                        这个场景还没有经验。
                        <div className="mt-1 text-xs">
                          经验从会话里来——先开一段会话记录，再把判断记下来。
                        </div>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            )}

            {tab === "conflicts" && (
              <div className="p-3">
                {e.conflicts.length === 0 && (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    未发现冲突。这只说明「同一话题下没有两种不同说法」，不说明判断都成立。
                  </p>
                )}
                {e.conflicts.map((g) => (
                  <Card key={`${g.scenario}|${g.topic}`} className="mb-3 border-orange-300">
                    <CardHeader className="py-2">
                      <CardTitle className="text-sm font-medium">{g.topic}</CardTitle>
                      <CardDescription>
                        <strong>系统不替你裁决</strong>——两条相反的经验并存，因为它们是两个判断
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      {(g.entries ?? []).map((x) => (
                        <div key={x.id} className="flex items-start gap-3 border-l-2 pl-3">
                          <div className="flex-1">
                            <div className="flex items-center gap-2">
                              <Badge variant="outline" className={LEVEL_CLASS[x.level] ?? ""}>
                                {x.level} 级
                              </Badge>
                              <span className="text-sm">{x.statement}</span>
                            </div>
                            <div className="text-[11px] text-muted-foreground">
                              <Actor id={`${x.proposedBy.kind}:${x.proposedBy.id}`} /> ·{" "}
                              依据 {x.sessionId} 第 {x.anchor} 句
                            </div>
                          </div>
                          <Button size="sm" variant="outline" onClick={() => e.select(x.id)}>
                            看这条
                          </Button>
                        </div>
                      ))}
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}

            {tab === "sessions" && (
              <div className="grid gap-3 p-3">
                <div className="flex items-end gap-2">
                  <div className="grid flex-1 gap-1">
                    <Label htmlFor="session-title" className="text-xs text-muted-foreground">
                      新会话标题
                    </Label>
                    <Input
                      id="session-title"
                      value={title}
                      onChange={(ev) => setTitle(ev.target.value)}
                      placeholder="例：暴击系数的口径讨论"
                      className="h-8"
                    />
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={e.loading || !title.trim()}
                    onClick={() => {
                      void e.openSession(title);
                      setTitle("");
                    }}
                  >
                    开一段
                  </Button>
                </div>
                {e.sessions.length === 0 && (
                  <p className="py-6 text-center text-sm text-muted-foreground">
                    还没有会话记录。它是经验的<strong>依据</strong>——没有它，经验无从回溯到
                    「哪次对话的哪一句」。
                  </p>
                )}
                {e.sessions.map((x) => (
                  <Card key={x.id} className={e.sessionId === x.id ? "border-sky-300" : ""}>
                    <CardHeader className="py-2">
                      <CardTitle className="text-sm">{x.title}</CardTitle>
                      <CardDescription>
                        {x.id} · {x.at} · {x.turns?.length ?? 0} 段
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      {(x.turns ?? []).map((t) => (
                        <div key={t.seq} className="border-l-2 pl-3 text-xs">
                          <div className="text-muted-foreground">
                            #{t.seq} {t.role.kind}:{t.role.id}
                          </div>
                          <div>{t.text}</div>
                        </div>
                      ))}
                      <Separator />
                      <div className="flex items-end gap-2">
                        <Input
                          value={e.sessionId === x.id ? e.draft : ""}
                          onChange={(ev) => {
                            e.selectSession(x.id);
                            e.setDraft(ev.target.value);
                          }}
                          placeholder="继续说……"
                          className="h-8"
                          aria-label={`追加发言 ${x.id}`}
                        />
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={e.loading}
                          onClick={() => void e.appendTurn(x.id, "human")}
                        >
                          以我的身份追加
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={e.loading}
                          onClick={() => void e.appendTurn(x.id, "agent")}
                        >
                          记一句 agent 的
                        </Button>
                      </div>
                      <p className="text-[11px] text-muted-foreground">
                        只能追加。<strong>可删改的依据不是依据</strong>——经验指向的
                        「第 3 段」必须永远是当初那句话。
                      </p>
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
          {!selected ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">从左侧选一条经验</CardTitle>
              </CardHeader>
              <CardContent className="text-xs leading-relaxed text-muted-foreground">
                经验是<strong>判断</strong>，不是事实：它围绕用途，带提出者与批准者。
                <br />
                责任级别由参与者与依据推导——同时有人与 agent 参与是 1 级，
                只有 agent 且无推导链是 4 级。界面不提供修改入口，
                因为级别错会让整层可信度失真。
              </CardContent>
            </Card>
          ) : (
            <>
              <Card>
                <CardHeader className="py-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline" className={LEVEL_CLASS[selected.level] ?? ""}>
                      {selected.level} 级 · {selected.levelText}
                    </Badge>
                    <Badge variant="outline" className={STATUS_CLASS[selected.status] ?? ""}>
                      {selected.statusText}
                    </Badge>
                    <Badge variant="outline" className="border-slate-300 text-slate-700">
                      {selected.kindText}
                    </Badge>
                  </div>
                  <CardTitle className="text-base leading-snug">{selected.statement}</CardTitle>
                  <CardDescription>{selected.topic}</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-2 text-xs">
                  {selected.rationale && (
                    <p className="text-muted-foreground">{selected.rationale}</p>
                  )}
                  <dl className="grid gap-1 border-t pt-3">
                    <Row
                      k="可信度上限"
                      v={`${selected.maxConfidence}　${g.confidence(selected.maxConfidence).label}`}
                    />
                    <Row k="提出者" v={g.actor(`${selected.proposedBy.kind}:${selected.proposedBy.id}`)} />
                    {(selected.collaborators ?? []).length > 0 && (
                      <Row
                        k="参与者"
                        v={(selected.collaborators ?? [])
                          .map((c) => g.actor(`${c.kind}:${c.id}`))
                          .join("、")}
                      />
                    )}
                    <Row
                      k="批准者"
                      v={
                        selected.approvedBy
                          ? `${selected.approvedBy.kind}:${selected.approvedBy.id}`
                          : "尚未批准"
                      }
                    />
                    <Row k="依据" v={`会话 ${selected.sessionId} 第 ${selected.anchor} 句`} />
                    <Row k="时间" v={selected.at} />
                    {selected.supersedes && <Row k="取代" v={selected.supersedes} />}
                  </dl>
                  {selected.approveReason && (
                    <p className="text-muted-foreground">理由：{selected.approveReason}</p>
                  )}
                  {(selected.chain ?? []).length > 0 && (
                    <div>
                      <div className="mb-1 text-muted-foreground">推导链</div>
                      <ul className="grid gap-0.5 font-mono text-[11px]">
                        {(selected.chain ?? []).map((c) => (
                          <li key={c}><Dep value={c} /></li>
                        ))}
                      </ul>
                    </div>
                  )}
                  {selected.sampleSize > 0 && (
                    <p className="text-muted-foreground">
                      样本量 {selected.sampleSize}
                      {selected.sampleFrom ? `（${selected.sampleFrom}）` : ""}
                    </p>
                  )}
                </CardContent>
              </Card>

              {selected.status === "candidate" || selected.status === "stale" ? (
                <Card>
                  <CardHeader className="py-3">
                    <CardTitle className="text-sm">确认</CardTitle>
                    <CardDescription>
                      批准者必须是<strong>人</strong>。agent 可以提出候选与取证，但不能自己决定什么算数。
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-2">
                    <div className="grid gap-1">
                      <Label htmlFor="exp-by" className="text-xs text-muted-foreground">
                        批准者（必须是人）
                      </Label>
                      <Input
                        id="exp-by"
                        value={by}
                        onChange={(ev) => setBy(ev.target.value)}
                        placeholder="你的名字"
                        className="h-8"
                      />
                    </div>
                    <div className="grid gap-1">
                      <Label htmlFor="exp-reason" className="text-xs text-muted-foreground">
                        理由（必填）
                      </Label>
                      <Input
                        id="exp-reason"
                        value={e.reason}
                        onChange={(ev) => e.setReason(ev.target.value)}
                        placeholder="例：与实测一致，依据充分"
                        className="h-8"
                      />
                    </div>
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        className="flex-1"
                        disabled={e.loading}
                        onClick={() => void e.approve(selected.id)}
                      >
                        批准
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1 border-rose-300 text-rose-700"
                        disabled={e.loading}
                        onClick={() => void e.reject(selected.id)}
                      >
                        驳回
                      </Button>
                    </div>
                    <p className="text-[11px] leading-relaxed text-muted-foreground">
                      驳回只改状态，<strong>条目保留</strong>——驳回是审计轨迹，不是删除。
                    </p>
                  </CardContent>
                </Card>
              ) : (
                <Card>
                  <CardContent className="pt-4 text-xs text-muted-foreground">
                    {selected.status === "effective"
                      ? "已生效。改判不是覆盖——以新条目取代它，旧条目保留可查。"
                      : "该状态不再参与确认。"}
                  </CardContent>
                </Card>
              )}

              <ProposeForm
                scenario={scenario}
                sessionId={session?.id ?? ""}
                by={by}
                disabled={e.loading}
                onSubmit={(proposal) => void e.propose(proposal)}
              />
            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <div className="flex gap-2">
      <dt className="w-20 shrink-0 text-muted-foreground">{k}</dt>
      <dd className="break-all">{v}</dd>
    </div>
  );
}

/** 提出候选经验。界面不收集级别——它由参与者与依据推导。 */
function ProposeForm({
  scenario,
  sessionId,
  by,
  disabled,
  onSubmit,
}: {
  scenario: string;
  sessionId: string;
  by: string;
  disabled: boolean;
  onSubmit: (proposal: ProposalInput) => void;
}) {
  const [topic, setTopic] = useState("");
  const [statement, setStatement] = useState("");
  const [kind, setKind] = useState("judgment");
  const [sampleSize, setSampleSize] = useState("");
  const [anchor, setAnchor] = useState("1");
  const [asAgent, setAsAgent] = useState(false);

  const canSubmit = Boolean(topic.trim() && statement.trim() && sessionId && (asAgent || by.trim()));

  return (
    <Card>
      <CardHeader className="py-3">
        <CardTitle className="text-sm">记一条判断</CardTitle>
        <CardDescription>
          提出的是<strong>候选</strong>，不会直接生效。级别由「谁参与 + 有没有依据」推导。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-2">
        <Input
          value={topic}
          onChange={(ev) => setTopic(ev.target.value)}
          placeholder="话题（冲突比较键，同一话题的不同说法才算冲突）"
          className="h-8"
          aria-label="话题"
        />
        <Input
          value={statement}
          onChange={(ev) => setStatement(ev.target.value)}
          placeholder="判断本身"
          className="h-8"
          aria-label="判断本身"
        />
        <div className="flex gap-2">
          <select
            aria-label="类型"
            className="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
            value={kind}
            onChange={(ev) => setKind(ev.target.value)}
          >
            {KINDS.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </select>
          {kind === "summary" && (
            <Input
              value={sampleSize}
              onChange={(ev) => setSampleSize(ev.target.value)}
              placeholder="样本量（总结型必填）"
              className="h-8"
              aria-label="样本量"
            />
          )}
        </div>
        <div className="flex gap-2">
          <Input
            value={anchor}
            onChange={(ev) => setAnchor(ev.target.value)}
            placeholder="会话中的位置（第几段）"
            className="h-8"
            aria-label="锚点"
          />
          <label className="flex items-center gap-1 text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={asAgent}
              onChange={(ev) => setAsAgent(ev.target.checked)}
            />
            以 agent 身份提
          </label>
        </div>
        <Button
          size="sm"
          disabled={disabled || !canSubmit}
          onClick={() =>
            onSubmit({
              scenario,
              topic: topic.trim(),
              kind,
              statement: statement.trim(),
              rationale: "",
              chain: [],
              sampleSize: Number(sampleSize) || 0,
              sampleFrom: "",
              conditions: "",
              preference: "",
              sessionId,
              anchor: anchor.trim(),
              proposedByKind: asAgent ? "agent" : "human",
              proposedById: asAgent ? "dsh" : by.trim(),
              collaborators: [],
            } as ProposalInput)
          }
        >
          提出候选
        </Button>
        <p className="text-[11px] leading-relaxed text-muted-foreground">
          推导型必须给推导链（<code className="rounded bg-muted px-1 font-mono text-[11px]">assert:&lt;ID&gt;</code> 或 <code className="rounded bg-muted px-1 font-mono text-[11px]">formula:&lt;名&gt;@&lt;版本&gt;</code>）；
          总结型必须给样本量——没有样本量的「经验」只是意见。
        </p>
      </CardContent>
    </Card>
  );
}