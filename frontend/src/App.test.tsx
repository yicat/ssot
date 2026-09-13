/**
 * 工作台的验收测试。
 *
 * 对应 docs/specs/workspace.spec.md 与既有那份核验规格中**界面能观测到**的条目：
 * 三层导航、空项目的提示、requires 逐项呈现、排序理由可见、冲突不裁决、批量先预览。
 * 领域规则本身在 Go 侧测。
 */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { useDecisionStore } from "./components/custom/Decision/store";
import { useReviewStore } from "./components/custom/Review/store";
import { useSessionStore } from "./components/custom/Session/store";
import { CallID, callMock } from "./test/mock-wails-runtime";

// ── 夹具 ────────────────────────────────────────────────────────────────────

function projectRef(over: Record<string, unknown> = {}) {
  return {
    dir: "projects/onmyoji",
    name: "onmyoji",
    path: "projects/onmyoji",
    description: "阴阳师",
    current: true,
    ...over,
  };
}

function sessionFixture(over: Record<string, unknown> = {}) {
  return {
    dir: "projects/onmyoji",
    name: "onmyoji",
    projectDir: "projects/onmyoji",
    description: "阴阳师",
    scenarios: [
      {
        name: "damage-calc",
        description: "伤害计算",
        inputs: 1,
        requiresMet: false,
        requiresTotal: 3,
        skipped: "",
      },
    ],
    scenario: "damage-calc",
    scenarioSkipped: [],
    ...over,
  };
}

function overviewFixture(over: Record<string, unknown> = {}) {
  return {
    dir: "projects/onmyoji",
    name: "onmyoji",
    path: "projects/onmyoji",
    description: "阴阳师",
    entities: [
      { entity: "shikigami", subjects: 268, assertions: 3216 },
      { entity: "skill", subjects: 774, assertions: 4576 },
    ],
    assertions: 7792,
    byStatus: { pending: 7524, verified: 268 },
    byConfidence: { L1: 2948, L2: 4568, L3: 268 },
    verifications: 268,
    conflicts: 0,
    decisions: 25,
    scenarioCount: 1,
    formulaCount: 2,
    entityTypes: 2,
    ...over,
  };
}

function scenarioOverviewFixture(over: Record<string, unknown> = {}) {
  return {
    name: "damage-calc",
    description: "伤害计算——给定式神与技能，估算期望伤害",
    runnable: false,
    requires: [
      { want: "shikigami.atk", status: "satisfied", have: 275, total: 275, coverage: 1, detail: "" },
      {
        want: "shikigami.crit_factor",
        status: "satisfied",
        have: 268,
        total: 275,
        coverage: 0.975,
        detail: "覆盖 268/275（98%），存在缺口",
      },
      {
        want: "skill.ratio",
        status: "satisfied",
        have: 395,
        total: 774,
        coverage: 0.51,
        detail: "覆盖 395/774（51%），存在缺口",
      },
    ],
    inputs: [
      {
        name: "def_reduction",
        description: "防御减免。**无权威来源**，必须由调用方提供",
        unit: "fraction",
        min: 0,
        max: 1,
      },
    ],
    formulas: [
      {
        name: "damage",
        status: "verified",
        cases: [{ name: "姑获鸟 伞剑1级", passed: true, detail: "通过" }],
      },
    ],
    outputs: ["damage-estimate"],
    missing: [],
    ...over,
  };
}

function itemFixture(over: Record<string, unknown> = {}) {
  return {
    id: "a1",
    entity: "shikigami",
    subject: "262",
    predicate: "atk",
    value: "3082 point",
    unit: "point",
    confidence: "L1",
    status: "pending",
    source: "huijiwiki",
    artifact: "Data:Attribute.json",
    anchor: "attributes[0].atk",
    revision: "revid:9263",
    ...over,
  };
}

function queueFixture(over: Record<string, unknown> = {}) {
  return {
    item: itemFixture(),
    tier: "影响面",
    reason: "被 1 条派生断言引用",
    score: 1010,
    sampled: false,
    ...over,
  };
}

function conflictFixture() {
  return {
    subject: "605_01",
    predicate: "character_id",
    claims: [
      itemFixture({ id: "c1", value: "606 point", artifact: "Data:Character/606.json" }),
      itemFixture({ id: "c2", value: "605 point", artifact: "Data:Character/605.json" }),
    ],
  };
}

function decisionFixture(over: Record<string, unknown> = {}) {
  const candidate = (o: Record<string, unknown> = {}) => ({
    index: 0,
    value: "33",
    unit: "percent",
    anchor: "skills[2].description#m0",
    context: "每次造成攻击33%伤害，",
    note: "",
    ...o,
  });
  return {
    id: "dabc123",
    entity: "skill",
    subject: "262_03",
    predicate: "ratio",
    reason: "描述中出现 2 个不同的伤害倍率（33%, 88%），无法确定取哪一个",
    context: "以伞为剑…每次造成攻击33%伤害，最后对敌方目标劈斩造成攻击88%伤害。",
    artifact: "Data:Character/262.json",
    revision: "revid:8053",
    source: "huijiwiki",
    status: "open",
    statusText: "待判定",
    impact: 0,
    impactReason: "候选仅 2 个，判断成本最低",
    candidates: [
      candidate(),
      candidate({
        index: 1,
        value: "88",
        anchor: "skills[2].description#m1",
        context: "最后对敌方目标劈斩造成攻击88%伤害。",
      }),
    ],
    resolution: null,
    deferredBy: "",
    deferReason: "",
    editable: true,
    ...over,
  };
}

/** 注册一整套后端返回值。 */
function backend(over: {
  projects?: unknown;
  session?: unknown;
  overview?: unknown;
  scenarioOverview?: unknown;
} = {}) {
  callMock
    .on(CallID.Projects, () => over.projects ?? [projectRef()])
    .on(CallID.Current, () => over.session ?? sessionFixture())
    .on(CallID.ProjectOverview, () => over.overview ?? overviewFixture())
    .on(CallID.ScenarioOverview, () => over.scenarioOverview ?? scenarioOverviewFixture())
    .on(CallID.ReviewStats, () => ({
      total: 2,
      byStatus: { pending: 2 },
      byEntity: { shikigami: 2 },
      byConfidence: { L1: 2 },
      verifications: 0,
      conflicts: 0,
    }))
    .on(CallID.Queue, () => [queueFixture()])
    .on(CallID.Conflicts, () => [])
    .on(CallID.History, () => [])
    .on(CallID.Decisions, () => [decisionFixture()])
    .on(CallID.DecisionStats, () => ({
      open: 1,
      deferred: 0,
      decided: 0,
      missing: 0,
      stale: 0,
    }));
}

/** 左栏导航里的一个分区。 */
function navGroup(title: string): HTMLElement {
  const nav = screen.getByRole("navigation");
  return within(nav).getByText(title).parentElement as HTMLElement;
}

/** 点击左栏某分区下的一个页面。 */
async function go(label: string, group: string) {
  await userEvent.click(within(navGroup(group)).getByRole("button", { name: label }));
}
beforeEach(() => {
  callMock.reset();
  useSessionStore.getState().reset();
  useReviewStore.getState().reset();
  useDecisionStore.getState().reset();
  backend();
});

afterEach(() => {
  callMock.reset();
});

// ── 会话与三层导航 ──────────────────────────────────────────────────────────

describe("会话", () => {
  // 浅色是明确要求：根容器与 <html> 都不应带 dark 类，
  // 否则 .dark 的令牌会盖掉 :root 的浅色令牌，界面又变回深色。
  it("浅色主题：没有任何元素带 dark 类", async () => {
    const { container } = render(<App />);
    await screen.findByText("SSOT 工作台");
    expect(container.querySelector(".dark")).toBeNull();
    expect(document.documentElement.className).not.toContain("dark");
    expect(document.body.className).not.toContain("dark");
  });

  it("顶栏显示当前项目与场景", async () => {
    render(<App />);
    expect(await screen.findByText("SSOT 工作台")).toBeInTheDocument();
    expect(await screen.findByLabelText("项目")).toBeInTheDocument();
    expect(screen.getByLabelText("场景")).toBeInTheDocument();
    // 同名项目靠完整路径区分
    expect(screen.getAllByText("projects/onmyoji").length).toBeGreaterThan(0);
  });

  // 「一个都没有」不是「出错了」——两者在界面上必须能区分。
  it("没有可用项目时明确说明，而不是留白", async () => {
    callMock.on(CallID.Projects, () => []);
    render(<App />);
    expect(await screen.findByText(/没有可用项目/)).toBeInTheDocument();
  });

  it("加载失败时把原因显示出来", async () => {
    callMock.on(CallID.Current, () => {
      throw new Error("加载项目 projects/onmyoji：schema 校验未通过");
    });
    render(<App />);
    expect(await screen.findByText(/schema 校验未通过/)).toBeInTheDocument();
  });

  it("切换项目失败时提示原因，且不把当前项目清掉", async () => {
    callMock
      .on(CallID.Projects, () => [
        projectRef(),
        projectRef({ dir: "projects/broken", name: "broken", path: "projects/broken" }),
      ])
      .on(CallID.Open, () => {
        throw new Error("加载项目 projects/broken：没有 project.yml");
      });

    render(<App />);
    await screen.findByLabelText("项目");
    await userEvent.click(screen.getByLabelText("项目"));
    await userEvent.click(await screen.findByRole("option", { name: /broken/ }));

    expect((await screen.findAllByText(/没有 project\.yml/)).length).toBeGreaterThan(0);
    // 原项目仍在
    expect(screen.getAllByText("projects/onmyoji").length).toBeGreaterThan(0);
  });

  it("左栏按层分区：项目 / 场景 / 核验", async () => {
    render(<App />);
    const nav = await screen.findByRole("navigation");
    expect(within(nav).getByText("项目")).toBeInTheDocument();
    expect(within(nav).getByText("场景：damage-calc")).toBeInTheDocument();
    expect(within(nav).getByText("核验")).toBeInTheDocument();
  });

  it("没有场景的项目：场景分区明确禁用并说明原因", async () => {
    callMock.on(CallID.Current, () => sessionFixture({ scenarios: [], scenario: null }));
    render(<App />);
    expect((await screen.findAllByText("该项目还没有场景")).length).toBeGreaterThan(0);
  });
});

// ── 项目概览 ────────────────────────────────────────────────────────────────

describe("项目概览", () => {
  it("给出规模、状态与分级分布，并写出未核验比例", async () => {
    render(<App />);
    expect((await screen.findAllByText("7792")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("7524").length).toBeGreaterThan(0);
    expect(screen.getByText(/占 97%/)).toBeInTheDocument();
    expect(screen.getByText("L3")).toBeInTheDocument();
  });

  // 「定义了却没有数据」正是最该被看见的缺口。
  it("以 schema 声明的实体为准，无数据的实体标出来", async () => {
    backend({
      overview: overviewFixture({
        entities: [
          { entity: "shikigami", subjects: 268, assertions: 3216 },
          { entity: "skill", subjects: 0, assertions: 0 },
        ],
      }),
    });
    render(<App />);
    expect(await screen.findByText("0（无数据）")).toBeInTheDocument();
  });

  it("没有场景的项目给出说明，而不是空白", async () => {
    backend({ overview: overviewFixture({ scenarioCount: 0 }) });
    render(<App />);
    expect(await screen.findByText(/该项目还没有场景/)).toBeInTheDocument();
  });
});

// ── 场景概览 ────────────────────────────────────────────────────────────────

describe("场景概览", () => {
  async function goScenario() {
    render(<App />);
    await screen.findByRole("navigation");
    await go("概览", "场景：damage-calc");
  }

  it("requires 逐项列出「现有/总数/覆盖率」，而不是只报一个总数", async () => {
    await goScenario();
    expect(await screen.findByText("shikigami.crit_factor")).toBeInTheDocument();
    expect(screen.getByText("268 / 275")).toBeInTheDocument();
    expect(screen.getByText("98%")).toBeInTheDocument();
    expect(screen.getAllByText(/存在缺口/).length).toBeGreaterThan(0);
  });

  it("外部输入带单位与范围——没有量纲的输入框就是歧义制造机", async () => {
    await goScenario();
    expect(await screen.findByText("def_reduction")).toBeInTheDocument();
    expect(screen.getByText("fraction")).toBeInTheDocument();
    expect(screen.getByText(/范围 0 ~ 1/)).toBeInTheDocument();
  });

  it("拒绝运行时逐项列出缺失原因", async () => {
    backend({
      scenarioOverview: scenarioOverviewFixture({
        runnable: false,
        missing: ["shikigami.crit_factor（missing）", "公式 damage 算例未通过"],
      }),
    });
    await goScenario();
    expect(await screen.findByText(/拒绝运行/)).toBeInTheDocument();
    expect(screen.getByText("shikigami.crit_factor（missing）")).toBeInTheDocument();
  });
});

// ── 核验 ────────────────────────────────────────────────────────────────────

describe("核验", () => {
  async function goQueue() {
    render(<App />);
    await screen.findByRole("navigation");
    await go("队列", "核验");
  }

  // 排在前面的理由必须写出来，否则人只能盲信排序。
  it("队列显示优先级类别与主导理由", async () => {
    await goQueue();
    expect(await screen.findByText("影响面")).toBeInTheDocument();
    expect(screen.getByText("被 1 条派生断言引用")).toBeInTheDocument();
  });

  it("抽检项被显式标出，不与普通项混淆", async () => {
    callMock.on(CallID.Queue, () => [queueFixture({ sampled: true })]);
    await goQueue();
    expect(await screen.findByText("抽检")).toBeInTheDocument();
  });

  it("选中后展示溯源——原件、位置、修订都要能看到", async () => {
    await goQueue();
    await userEvent.click(await screen.findByText("shikigami/262"));
    await waitFor(() => {
      expect(screen.getByText("Data:Attribute.json")).toBeInTheDocument();
    });
    expect(screen.getByText("attributes[0].atk")).toBeInTheDocument();
    expect(screen.getByText("revid:9263")).toBeInTheDocument();
  });

  it("批准需要批准者与理由——缺任一项不发请求", async () => {
    let called = false;
    callMock.on(CallID.Approve, () => {
      called = true;
      return null;
    });

    await goQueue();
    await userEvent.click(await screen.findByText("shikigami/262"));
    await userEvent.click(screen.getByRole("button", { name: "批准" }));

    expect((await screen.findAllByText(/必须填写批准者/)).length).toBeGreaterThan(0);
    expect(called).toBe(false);
  });

  it("填齐之后把批准者、方法与理由一并送出去", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.Approve, (a) => {
      args = a;
      return null;
    });

    await goQueue();
    await userEvent.click(await screen.findByText("shikigami/262"));
    await userEvent.type(screen.getByLabelText("批准者（必须是人）"), "yicat");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "与原文逐字比对一致");
    await userEvent.click(screen.getByRole("button", { name: "批准" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    const [, by, method, reason] = args as string[];
    expect(by).toBe("yicat");
    expect(method).toBe("editorial");
    expect(reason).toBe("与原文逐字比对一致");
  });

  it("后端拒绝时把原因显示出来，而不是静默失败", async () => {
    callMock.on(CallID.Approve, () => {
      throw new Error("批准者必须是**人**");
    });
    await goQueue();
    await userEvent.click(await screen.findByText("shikigami/262"));
    await userEvent.type(screen.getByLabelText("批准者（必须是人）"), "x");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "r");
    await userEvent.click(screen.getByRole("button", { name: "批准" }));

    expect(await screen.findAllByText(/批准者必须是/)).not.toHaveLength(0);
  });

  it("选实测方法时才要求填依据", async () => {
    await goQueue();
    await userEvent.click(await screen.findByText("shikigami/262"));
    expect(screen.queryByLabelText(/依据（实测必填/)).toBeNull();
    await userEvent.click(screen.getByLabelText("方法"));
    await userEvent.click(await screen.findByRole("option", { name: "实测" }));
    expect(await screen.findByLabelText(/依据（实测必填/)).toBeInTheDocument();
  });

  // 冲突必须成组呈现，且系统不替人裁决。
  it("冲突页把同一件事的多种说法并列摆出", async () => {
    callMock.on(CallID.Conflicts, () => [conflictFixture()]);
    render(<App />);
    await screen.findByRole("navigation");
    await go("冲突", "核验");

    expect(await screen.findByText("605_01.character_id")).toBeInTheDocument();
    expect(screen.getByText("606 point")).toBeInTheDocument();
    expect(screen.getByText("605 point")).toBeInTheDocument();
    expect(screen.getByText(/Data:Character\/606\.json/)).toBeInTheDocument();
    expect(screen.getByText(/Data:Character\/605\.json/)).toBeInTheDocument();
  });

  it("无冲突时明确说明「没有冲突不等于数据正确」", async () => {
    render(<App />);
    await screen.findByRole("navigation");
    await go("冲突", "核验");
    expect(await screen.findByText(/不说明数据已经正确/)).toBeInTheDocument();
  });

  // 批量最容易造成大面积错误，因此执行前必须先看见影响面。
  it("批量操作先预览影响面，不直接执行", async () => {
    let approved = false;
    callMock.on(CallID.BatchPreview, () => ({
      matched: 268,
      where: "实体=shikigami 且 谓词=id",
      sample: [itemFixture({ subject: "200", predicate: "id", value: "200 point" })],
    }));
    callMock.on(CallID.BatchApprove, () => {
      approved = true;
      return { matched: 268, applied: 268, failed: 0, firstErr: "" };
    });

    await goQueue();
    await userEvent.click(await screen.findByRole("button", { name: "预览影响面" }));
    expect(await screen.findByText("实体=shikigami 且 谓词=id")).toBeInTheDocument();
    expect(approved).toBe(false); // 预览阶段不得执行
  });
});

// ── 待判定 ──────────────────────────────────────────────────────────────────

describe("待判定", () => {
  async function goDecision() {
    render(<App />);
    await screen.findByRole("navigation");
    await go("待判定", "核验");
  }

  it("列出事项，并写出「为什么排在前面」", async () => {
    await goDecision();
    expect(await screen.findByText("skill/262_03")).toBeInTheDocument();
    expect(screen.getByText("候选仅 2 个，判断成本最低")).toBeInTheDocument();
  });

  // 只给一个数字，人无法判断——上下文是候选的必填部分。
  it("每个候选都带原文片段，不必回去翻原件", async () => {
    await goDecision();
    expect(await screen.findByText("每次造成攻击33%伤害，")).toBeInTheDocument();
    expect(screen.getByText("最后对敌方目标劈斩造成攻击88%伤害。")).toBeInTheDocument();
    expect(screen.getByText("skills[2].description#m1")).toBeInTheDocument();
  });

  it("裁决选中候选时把序号、裁决人与理由一并送出去", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.ResolveDecision, (a) => {
      args = a;
      return { applied: true, status: "decided", assertionId: "a123", message: "已裁决" };
    });

    await goDecision();
    await userEvent.click(await screen.findByText("最后对敌方目标劈斩造成攻击88%伤害。"));
    await userEvent.type(screen.getByLabelText("裁决人（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "主伤害那一句是 88%");
    await userEvent.click(screen.getByRole("button", { name: "裁决选中候选" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    const [id, choice, by, method, reason] = args as [string, number, string, string, string];
    expect(id).toBe("dabc123");
    expect(choice).toBe(1);
    expect(by).toBe("ngnl5");
    expect(method).toBe("editorial");
    expect(reason).toBe("主伤害那一句是 88%");
  });

  // 「都不对」是结论，「暂缓」是未处理，两者必须分别可点、不互相顶替。
  it("「都不对」送出 -1，而不是「选了第 0 个」", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.ResolveDecision, (a) => {
      args = a;
      return { applied: true, status: "decided", assertionId: "", message: "已判定候选均不成立" };
    });

    await goDecision();
    await userEvent.type(screen.getByLabelText("裁决人（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "两处都是条件分支");
    await userEvent.click(screen.getByRole("button", { name: "都不对" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    expect(args[1]).toBe(-1);
  });

  it("理由为空时拦在前端，不把请求发出去", async () => {
    let called = false;
    callMock.on(CallID.ResolveDecision, () => {
      called = true;
      return { applied: true, status: "decided", assertionId: "a1", message: "" };
    });

    await goDecision();
    await userEvent.type(screen.getByLabelText("裁决人（必须是人）"), "ngnl5");
    await userEvent.click(screen.getByRole("button", { name: "裁决选中候选" }));

    expect((await screen.findAllByText(/必须说明理由/)).length).toBeGreaterThan(0);
    expect(called).toBe(false);
  });

  it("已裁决事项不再可操作，并说明为什么不支持改判", async () => {
    callMock.on(CallID.Decisions, () => [
      decisionFixture({
        status: "decided",
        statusText: "已裁决",
        editable: false,
        resolution: {
          choice: 1,
          chosenValue: "88 percent",
          by: "human:ngnl5",
          reason: "主伤害那一句是 88%",
          method: "编审",
          evidence: "",
          at: "2025-03-01T12:00:00Z",
          assertionId: "a123",
        },
      }),
    ]);

    await goDecision();
    await waitFor(() => expect(screen.getByText(/a123/)).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "裁决选中候选" })).toBeNull();
    expect(screen.getByText(/改判不是覆盖/)).toBeInTheDocument();
    // 未选中的候选仍然在——「当时还有哪些说法」是可追溯性的一部分
    expect(screen.getByText("每次造成攻击33%伤害，")).toBeInTheDocument();
  });

  it("空队列也要说明「没有歧义不等于数据完整」", async () => {
    callMock.on(CallID.Decisions, () => []);
    callMock.on(CallID.DecisionStats, () => ({
      open: 0,
      deferred: 0,
      decided: 0,
      missing: 0,
      stale: 0,
    }));

    await goDecision();
    expect(await screen.findByText(/没有需要人看的待判定事项/)).toBeInTheDocument();
    expect(screen.getByText(/不说明数据已经完整/)).toBeInTheDocument();
  });
});
