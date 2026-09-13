/**
 * 经验页的验收测试（docs/specs/experience.spec.md 中界面能观测到的条目）。
 *
 * 特别测「界面上**没有**什么」：没有修改级别的入口、没有删除条目、
 * 没有编辑发言——这三件事不是遗漏，是规则。
 */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { useExperienceStore } from "./components/custom/Experience/store";
import { useSessionStore } from "./components/custom/Session/store";
import { CallID, callMock } from "./test/mock-wails-runtime";
import { GlossaryCallID, glossaryFixture } from "./test/glossary-fixture";

const session = {
  dir: "projects/onmyoji",
  name: "onmyoji",
  projectDir: "projects/onmyoji",
  description: "阴阳师",
  scenarios: [
    {
      name: "damage-calc",
      description: "伤害计算",
      inputs: 1,
      requiresMet: true,
      requiresTotal: 1,
      skipped: "",
    },
  ],
  scenario: "damage-calc",
  scenarioSkipped: [],
};

function entry(over: Record<string, unknown> = {}) {
  return {
    id: "e1",
    scenario: "damage-calc",
    topic: "倍率口径",
    kind: "judgment",
    kindText: "判定型",
    statement: "倍率取主伤害那一段",
    rationale: "与技能主句一致",
    chain: [],
    sampleSize: 0,
    sampleFrom: "",
    conditions: "",
    preference: "",
    level: 2,
    levelText: "人工确认",
    maxConfidence: "L2",
    proposedBy: { kind: "human", id: "ngnl5" },
    collaborators: [],
    approvedBy: null,
    approveReason: "",
    sessionId: "s1",
    anchor: "1",
    status: "candidate",
    statusText: "候选",
    usable: false,
    at: "2026-03-01T12:00:00Z",
    supersedes: "",
    conflictsWith: [],
    ...over,
  };
}

function sessionView(over: Record<string, unknown> = {}) {
  return {
    id: "s1",
    scenario: "damage-calc",
    title: "暴击系数口径讨论",
    at: "2026-03-01T11:00:00Z",
    turns: [
      { seq: 1, role: { kind: "human", id: "ngnl5" }, text: "暴击系数到底怎么算？", at: "2026-03-01T11:00:00Z" },
      { seq: 2, role: { kind: "agent", id: "dsh" }, text: "按 1 + cri × crid", at: "2026-03-01T11:01:00Z" },
    ],
    ...over,
  };
}

function backend() {
  callMock
    .on(CallID.Projects, () => [
      { dir: "projects/onmyoji", name: "onmyoji", path: "projects/onmyoji", description: "阴阳师", current: true },
    ])
    .on(CallID.Current, () => session)
    .on(GlossaryCallID, () => glossaryFixture())
    .on(CallID.ProjectOverview, () => ({
      dir: "projects/onmyoji",
      name: "onmyoji",
      path: "projects/onmyoji",
      description: "阴阳师",
      entities: [],
      assertions: 0,
      byStatus: {},
      byConfidence: {},
      verifications: 0,
      conflicts: 0,
      decisions: 0,
      scenarioCount: 1,
      formulaCount: 0,
      entityTypes: 0,
    }))
    .on(CallID.ExpList, () => [entry()])
    .on(CallID.ExpConflicts, () => [])
    .on(CallID.ExpSessions, () => [sessionView()])
    .on(CallID.ExpRefresh, () => ({ checked: 1, staled: [], recompute: [] }))
    .on(CallID.ReviewStats, () => ({
      total: 0,
      byStatus: {},
      byEntity: {},
      byConfidence: {},
      verifications: 0,
      conflicts: 0,
    }))
    .on(CallID.Queue, () => [])
    .on(CallID.Conflicts, () => [])
    .on(CallID.History, () => [])
    .on(CallID.Decisions, () => [])
    .on(CallID.DecisionStats, () => ({ open: 0, deferred: 0, decided: 0, missing: 0, stale: 0 }));
}

function navGroup(title: string): HTMLElement {
  const nav = screen.getByRole("navigation");
  return within(nav).getByText(title).parentElement as HTMLElement;
}

async function goExperience() {
  render(<App />);
  await screen.findByRole("navigation");
  await userEvent.click(
    within(navGroup("场景：damage-calc")).getByRole("button", { name: "经验" }),
  );
}

beforeEach(() => {
  callMock.reset();
  useSessionStore.getState().reset();
  useExperienceStore.getState().reset();
  backend();
});

afterEach(() => {
  callMock.reset();
});

describe("经验", () => {
  it("列出经验并显示责任级别与状态", async () => {
    await goExperience();
    expect((await screen.findAllByText("倍率取主伤害那一段")).length).toBeGreaterThan(0);
    expect(screen.getByText("2 级")).toBeInTheDocument();
    expect(screen.getByText(/人工确认/)).toBeInTheDocument();
    expect(screen.getAllByText("候选").length).toBeGreaterThan(0);
  });

  // 级别是算出来的，界面不提供修改入口——级别错会让整层可信度失真。
  it("界面上没有修改责任级别的入口", async () => {
    await goExperience();
    await screen.findAllByText("倍率取主伤害那一段");
    expect(screen.queryByText(/修改级别/)).toBeNull();
    expect(screen.queryByLabelText(/级别/)).toBeNull();
    // 但级别与可信度上限必须可读
    expect(screen.getByText("可信度上限")).toBeInTheDocument();
    expect(screen.getAllByText(/L2/).length).toBeGreaterThan(0);
  });

  it("展示依据：会话记录与其中的位置", async () => {
    await goExperience();
    expect(await screen.findByText(/会话 s1 第 1 句/)).toBeInTheDocument();
  });

  it("每个 3/4 级经验都带着它的级别，不被当成确定事实", async () => {
    callMock.on(CallID.ExpList, () => [
      entry({
        id: "e4",
        statement: "agent 凭印象给出的判断",
        proposedBy: { kind: "agent", id: "dsh" },
        level: 4,
        levelText: "agent 产出·推断",
        maxConfidence: "L4",
      }),
    ]);
    await goExperience();
    expect(await screen.findByText("4 级")).toBeInTheDocument();
    expect(screen.getByText(/agent 产出·推断/)).toBeInTheDocument();
  });

  // 验收：批准者与理由都得填；缺任一项不发请求。
  it("批准需要批准者与理由", async () => {
    let called = false;
    callMock.on(CallID.ExpApprove, () => {
      called = true;
      return entry({ status: "effective", statusText: "生效" });
    });

    await goExperience();
    await userEvent.click(await screen.findByRole("button", { name: "批准" }));

    expect(await screen.findByText(/必须填写批准者/)).toBeInTheDocument();
    expect(called).toBe(false);
  });

  it("填齐后送出批准者与理由", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.ExpApprove, (a) => {
      args = a;
      return entry({ status: "effective", statusText: "生效" });
    });

    await goExperience();
    await userEvent.type(screen.getByLabelText("批准者（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "与实测一致");
    await userEvent.click(screen.getByRole("button", { name: "批准" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    expect(args[0]).toBe("e1");
    expect(args[1]).toBe("ngnl5");
    expect(args[2]).toBe("与实测一致");
  });

  // 验收：驳回只改状态，条目保留。
  it("驳回后条目仍在列表里", async () => {
    // 先返回候选、驳回之后再返回已驳回：否则一开始就没有确认入口，
    // 测到的会是「驳回没生效」，而不是「驳回之后条目还在」。
    let rejected = false;
    const rejectedEntry = entry({ status: "rejected", statusText: "已驳回" });
    callMock
      .on(CallID.ExpReject, () => {
        rejected = true;
        return rejectedEntry;
      })
      .on(CallID.ExpList, () => (rejected ? [rejectedEntry] : [entry()]));

    await goExperience();
    await userEvent.type(screen.getByLabelText("批准者（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "依据不足");
    await userEvent.click(screen.getByRole("button", { name: "驳回" }));

    expect((await screen.findAllByText("已驳回")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("倍率取主伤害那一段").length).toBeGreaterThan(0);
    // 已驳回的条目不再参与确认
    expect(screen.queryByRole("button", { name: "批准" })).toBeNull();
  });

  // 验收：冲突经验并存且被标记，不自动择一。
  it("冲突页把两种说法并列摆出，并说明系统不裁决", async () => {
    callMock
      .on(CallID.ExpList, () => [
        entry({ id: "e1", statement: "倍率取主伤害那一段" }),
        entry({ id: "e2", statement: "倍率取最后一段劈斩" }),
      ])
      .on(CallID.ExpConflicts, () => [
        {
          scenario: "damage-calc",
          topic: "倍率口径",
          entries: [
            entry({ id: "e1", statement: "倍率取主伤害那一段" }),
            entry({ id: "e2", statement: "倍率取最后一段劈斩" }),
          ],
        },
      ]);

    await goExperience();
    await userEvent.click(await screen.findByRole("tab", { name: /冲突/ }));

    expect(screen.getAllByText("倍率取主伤害那一段").length).toBeGreaterThan(0);
    expect(screen.getAllByText("倍率取最后一段劈斩").length).toBeGreaterThan(0);
    expect(screen.getByText(/系统不替你裁决/)).toBeInTheDocument();
  });

  // 会话记录是依据：只能追加，没有编辑与删除。
  it("会话记录只能追加，没有编辑与删除入口", async () => {
    await goExperience();
    await userEvent.click(await screen.findByRole("tab", { name: /会话记录/ }));

    expect(await screen.findByText("暴击系数到底怎么算？")).toBeInTheDocument();
    expect(screen.getByText("按 1 + cri × crid")).toBeInTheDocument();
    // 参与者逐段记录——否则无法判断哪句是谁说的
    expect(screen.getByText(/#1 human:ngnl5/)).toBeInTheDocument();
    expect(screen.getByText(/#2 agent:dsh/)).toBeInTheDocument();

    expect(screen.getByRole("button", { name: "以我的身份追加" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "记一句 agent 的" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /编辑/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /删除/ })).toBeNull();
    expect(screen.getByText(/可删改的依据不是依据/)).toBeInTheDocument();
  });

  it("追加发言时把参与者与内容一并送出", async () => {
    let args: unknown[] = [];
    callMock
      .on(CallID.ExpAppendTurn, (a) => {
        args = a;
        return sessionView();
      })
      .on(CallID.ExpSessions, () => [sessionView()]);

    await goExperience();
    await userEvent.click(await screen.findByRole("tab", { name: /会话记录/ }));
    // 使用者身份是会话级的：在核验里填过一次，经验页就不必再填。
    useSessionStore.getState().setBy("ngnl5");
    await userEvent.type(await screen.findByLabelText(/追加发言/), "补充一句：实测一致");
    await userEvent.click(screen.getByRole("button", { name: "以我的身份追加" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    expect(args[0]).toBe("s1");
    expect(args[1]).toBe("human");
    expect(args[3]).toBe("补充一句：实测一致");
  });

  // 验收：重算依赖——依赖被驳回则失效、依赖公式变更则待重算。
  it("重算依赖把结果说出来", async () => {
    callMock.on(CallID.ExpRefresh, () => ({ checked: 3, staled: ["e1"], recompute: ["e2"] }));
    await goExperience();
    await userEvent.click(await screen.findByRole("button", { name: "重算依赖" }));
    expect(await screen.findByText(/检查 3 条，失效 1 条，待重算 1 条/)).toBeInTheDocument();
  });

  it("空列表说明「没有经验」以及这意味着什么", async () => {
    callMock.on(CallID.ExpList, () => []);
    await goExperience();
    expect(await screen.findByText(/这个场景还没有经验/)).toBeInTheDocument();
    expect(screen.getByText(/经验从会话里来/)).toBeInTheDocument();
  });
});
