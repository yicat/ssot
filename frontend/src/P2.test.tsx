/**
 * P2 三个页面的验收测试：数据 / 公式 / 场景运行。
 *
 * 对应 docs/specs/workspace.spec.md 与既有规格中**界面能观测到**的条目。
 * 领域规则在 Go 侧测，这里只测「界面上能不能看见」。
 */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { useDataStore } from "./components/custom/DataExplorer/store";
import { useRunStore } from "./components/custom/ScenarioRun/store";
import { useSessionStore } from "./components/custom/Session/store";
import { CallID, callMock } from "./test/mock-wails-runtime";
import { GlossaryCallID, glossaryFixture } from "./test/glossary-fixture";

// ── 夹具 ────────────────────────────────────────────────────────────────────

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
      requiresMet: false,
      requiresTotal: 3,
      skipped: "",
    },
  ],
  scenario: "damage-calc",
  scenarioSkipped: [],
};

const overview = {
  dir: "projects/onmyoji",
  name: "onmyoji",
  path: "projects/onmyoji",
  description: "阴阳师",
  entities: [{ entity: "shikigami", subjects: 268, assertions: 3216 }],
  assertions: 7792,
  byStatus: { pending: 7524 },
  byConfidence: { L1: 2948 },
  verifications: 268,
  conflicts: 0,
  decisions: 25,
  scenarioCount: 1,
  formulaCount: 2,
  entityTypes: 1,
};

function item(over: Record<string, unknown> = {}) {
  return {
    id: "a1",
    entity: "skill",
    subject: "262_03",
    predicate: "ratio",
    value: "33 percent",
    unit: "percent",
    confidence: "L2",
    status: "pending",
    source: "huijiwiki",
    artifact: "Data:Character/262.json",
    anchor: "skills[2].description",
    revision: "revid:8053",
    ...over,
  };
}

const quality = [
  {
    entity: "skill",
    subjects: 774,
    assertions: 4576,
    fields: [
      { key: "id", declared: true, required: true, coverage: 1, subjects: 774 },
      { key: "ratio", declared: true, required: false, coverage: 0.51, subjects: 395 },
      { key: "voice", declared: false, required: false, coverage: 0.02, subjects: 15 },
    ],
    undeclared: ["voice"],
    unused: [],
    requiredMissing: [],
  },
];

const schemas = [
  {
    entity: "skill",
    description: "技能",
    schemaRev: "1",
    fields: [
      {
        key: "id",
        description: "技能唯一 ID",
        type: "text",
        unit: "",
        required: true,
        identity: true,
        unique: false,
        target: "",
        values: [],
        min: null,
        max: null,
      },
      {
        key: "character_id",
        description: "所属式神",
        type: "ref",
        unit: "",
        required: true,
        identity: false,
        unique: false,
        target: "shikigami",
        values: [],
        min: null,
        max: null,
      },
    ],
  },
];

const formulas = [
  {
    name: "damage",
    version: "1",
    description: "伤害估算",
    result: "atk * ratio * crit_factor * def_reduction",
    params: [
      { name: "atk", type: "number", unit: "point", note: "面板攻击" },
      { name: "def_reduction", type: "number", unit: "fraction", note: "防御减免" },
    ],
    bindings: [{ name: "atk", path: "atk" }],
    status: "verified",
    cases: [{ name: "姑获鸟 伞剑1级", passed: true, got: "1972.48 point", want: "1972.48 point", detail: "通过" }],
    error: "",
    file: "projects/onmyoji/formulas/damage.formula.yml",
    usedBy: ["damage-calc"],
  },
  {
    name: "draft",
    version: "0",
    description: "没有算例的草稿",
    result: "atk * 2",
    params: [],
    bindings: [],
    status: "unverified",
    cases: [],
    error: "",
    file: "projects/onmyoji/formulas/draft.formula.yml",
    usedBy: [],
  },
];

function runSetup(over: Record<string, unknown> = {}) {
  return {
    scenario: "damage-calc",
    entity: "shikigami",
    subjects: ["262"],
    subject: null,
    runnable: true,
    requires: [
      { want: "skill.ratio", status: "satisfied", have: 395, total: 774, coverage: 0.51, detail: "存在缺口" },
    ],
    missing: [],
    inputs: [
      {
        name: "def_reduction",
        description: "防御减免。无权威来源",
        unit: "fraction",
        min: 0,
        max: 1,
      },
    ],
    refs: [
      {
        name: "ratio",
        entity: "skill",
        via: "character_id",
        description: "技能的一级伤害倍率",
        candidates: ["262_01", "262_03"],
      },
    ],
    ...over,
  };
}

function runResult(over: Record<string, unknown> = {}) {
  return {
    ok: true,
    scenario: "damage-calc",
    subject: "262",
    result: "1972.48 point",
    resultUnit: "point",
    bindings: [
      { name: "atk", value: "3082", unit: "point" },
      { name: "def_reduction", value: "0.5", unit: "fraction" },
    ],
    notes: ["本产出依赖 2 项未核验输入，结论仅供参考"],
    unverifiedRatio: 0.5,
    unverified: [
      { id: "a1", subject: "shikigami/262", predicate: "atk", value: "3082 point", status: "pending", confidence: "L1" },
      { id: "a2", subject: "skill/262_01", predicate: "ratio", value: "80 percent", status: "pending", confidence: "L2" },
    ],
    external: [{ name: "def_reduction", value: "0.5 fraction", why: "无权威来源" }],
    requires: [],
    missing: [],
    message: "已算出 1972.48 point",
    at: "2026-09-13T15:00:00Z",
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
    .on(CallID.ProjectOverview, () => overview)
    .on(CallID.DataAssertions, () => ({
      items: [item()],
      total: 7792,
      offset: 0,
      limit: 100,
      where: "（无条件——这会选中全部断言）",
    }))
    .on(CallID.DataQuality, () => quality)
    .on(CallID.DataSchema, () => schemas)
    .on(CallID.DataSubjectDetail, () => [item()])
    .on(CallID.FormulaList, () => formulas)
    .on(CallID.ScenarioRunSetup, () => runSetup())
    .on(CallID.ScenarioRun, () => runResult())
    .on(CallID.ScenarioOverview, () => ({
      name: "damage-calc",
      description: "伤害计算",
      runnable: true,
      requires: [],
      inputs: [],
      formulas: [],
      outputs: [],
      missing: [],
    }))
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

async function go(label: string, group: string) {
  const btn = within(navGroup(group)).getByRole("button", { name: label });
  // 场景分区在会话加载完成前是禁用的。等它可用再点——点了等于没点的话，
  // 失败会指向断言那一行，而真正的原因在导航还没就绪。
  await waitFor(() => expect(btn).toBeEnabled());
  await userEvent.click(btn);
}

beforeEach(() => {
  callMock.reset();
  useSessionStore.getState().reset();
  useDataStore.getState().reset();
  useRunStore.getState().reset();
  backend();
});

afterEach(() => {
  callMock.reset();
});

// ── 数据页 ──────────────────────────────────────────────────────────────────

describe("数据页", () => {
  async function goData() {
    render(<App />);
    await screen.findByRole("navigation");
    await go("数据", "项目");
  }

  it("断言库列出断言，并给出「共几条 / 当前显示第几到第几」", async () => {
    await goData();
    // 主体现在显示「名字（标识）」——中文在前，标识在后
    expect(await screen.findByText(/天翔鹤斩/)).toBeInTheDocument();
    expect(screen.getAllByText(/262_03/).length).toBeGreaterThan(0);
    expect(screen.getByText(/共/)).toBeInTheDocument();
    expect(screen.getAllByText("7792").length).toBeGreaterThan(0);
  });

  it("点一条断言能看到它的主体上的全部断言", async () => {
    await goData();
    await userEvent.click(await screen.findByText(/天翔鹤斩/));
    await waitFor(() => {
      expect(screen.getByText(/该主体上的 1 条断言/)).toBeInTheDocument();
    });
  });

  // 双向漂移必须能追溯到具体字段名：只给一个「质量分」说不出哪里有洞。
  it("数据质量指出「数据里有而 schema 未声明」的字段", async () => {
    await goData();
    await userEvent.click(await screen.findByRole("tab", { name: "数据质量" }));
    expect(await screen.findByText(/数据里有而 schema 未声明/)).toBeInTheDocument();
    expect(screen.getByText("未声明")).toBeInTheDocument();
    expect(screen.getByText(/接进来了但没入库/)).toBeInTheDocument();
  });

  it("数据质量给出每个字段的覆盖率，必填字段标出来", async () => {
    await goData();
    await userEvent.click(await screen.findByRole("tab", { name: "数据质量" }));
    // 字段显示中文名 + 标识
    expect(await screen.findByText(/技能唯一 ID/)).toBeInTheDocument();
    expect(screen.getAllByText("id").length).toBeGreaterThan(0);
    expect(screen.getByText("51%")).toBeInTheDocument();
    expect(screen.getByText("*")).toBeInTheDocument();
  });

  it("定义页展示字段的类型、单位与约束", async () => {
    await goData();
    await userEvent.click(await screen.findByRole("tab", { name: "定义" }));
    // 字段、类型、目标实体都要有中文
    expect((await screen.findAllByText(/所属式神/)).length).toBeGreaterThan(0);
    expect(screen.getAllByText("character_id").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/式神/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/引用/).length).toBeGreaterThan(0);
    expect(screen.getByText("身份 必填")).toBeInTheDocument();
  });
});

// ── 公式页 ──────────────────────────────────────────────────────────────────

describe("公式页", () => {
  async function goFormulas() {
    render(<App />);
    await screen.findByRole("navigation");
    await go("公式", "项目");
  }

  it("显示公式原文与算例结果——不透明的公式没法被审阅", async () => {
    await goFormulas();
    expect(await screen.findByText("atk * ratio * crit_factor * def_reduction")).toBeInTheDocument();
    expect(screen.getByText("姑获鸟 伞剑1级")).toBeInTheDocument();
    expect(screen.getByText("算例全过")).toBeInTheDocument();
  });

  // 把「无算例」显示成绿色等于把未验证的公式伪装成验证过的。
  it("没有算例的公式标为「无算例（未验证）」，与已验证区分开", async () => {
    await goFormulas();
    expect(await screen.findByText("无算例（未验证）")).toBeInTheDocument();
    expect(screen.getByText(/产出必须标注「公式未验证」/)).toBeInTheDocument();
  });

  it("显示公式被哪些场景使用", async () => {
    await goFormulas();
    expect(await screen.findByText(/被场景 damage-calc 使用/)).toBeInTheDocument();
  });
});

// ── 场景运行 ────────────────────────────────────────────────────────────────

describe("场景运行", () => {
  async function goRun() {
    render(<App />);
    await screen.findByRole("navigation");
    await go("运行", "场景：damage-calc");
  }

  // 没有量纲的输入框就是歧义制造机：单位来自声明，不让手写。
  it("外部输入标出声明单位与范围", async () => {
    await goRun();
    expect(await screen.findByText("def_reduction")).toBeInTheDocument();
    expect(screen.getByText("fraction")).toBeInTheDocument();
    expect(screen.getByText("0 ~ 1")).toBeInTheDocument();
  });

  it("引用列出候选，而不是让人凭记忆敲 ID", async () => {
    await goRun();
    // 先选主体，候选才出现
    await userEvent.click(screen.getByLabelText("主体"));
    await userEvent.click(await screen.findByRole("option", { name: "262" }));
    await waitFor(() => {
      expect(screen.getByLabelText("ratio")).toBeInTheDocument();
    });
  });

  it("requires 未满足时挡住运行并逐项说明", async () => {
    callMock.on(CallID.ScenarioRunSetup, () =>
      runSetup({ runnable: false, missing: ["skill.ratio（missing）"] }),
    );
    await goRun();
    expect(await screen.findByText("skill.ratio（missing）")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "运行场景" })).toBeDisabled();
  });

  // 产出必须带未核验比例：只给结果不给可信度，正是「看起来已经核验过」的来源。
  it("产出带上未核验比例与未核验依赖清单", async () => {
    callMock.on(CallID.ScenarioRun, () => runResult());
    await goRun();
    await userEvent.click(screen.getByLabelText("主体"));
    await userEvent.click(await screen.findByRole("option", { name: "262" }));
    await userEvent.type(await screen.findByLabelText(/def_reduction/), "0.5");
    await userEvent.click(screen.getByLabelText("ratio"));
    await userEvent.click(await screen.findByRole("option", { name: "262_01" }));
    await userEvent.click(screen.getByRole("button", { name: "运行场景" }));

    expect(await screen.findByText("1972.48 point")).toBeInTheDocument();
    expect(screen.getByText(/未核验 50%/)).toBeInTheDocument();
    // 依赖的断言显示「主体名 + 字段中文名」
    expect(screen.getByText(/姑获鸟/)).toBeInTheDocument();
    expect(screen.getAllByText(/攻击/).length).toBeGreaterThan(0);
    expect(screen.getByText(/外部输入——\*\*始终未核验\*\*/)).toBeInTheDocument();
  });

  // 运行失败时保留已填输入：那正是人最需要保留刚才填了什么的时候。
  it("运行被拒绝时保留已填输入并显示原因", async () => {
    callMock.on(CallID.ScenarioRun, () =>
      runResult({ ok: false, result: "", message: "场景 damage-calc 的 requires 未满足，拒绝运行", missing: ["skill.ratio（missing）"] }),
    );
    await goRun();
    await userEvent.click(screen.getByLabelText("主体"));
    await userEvent.click(await screen.findByRole("option", { name: "262" }));
    await userEvent.type(await screen.findByLabelText(/def_reduction/), "0.5");
    await userEvent.click(screen.getByLabelText("ratio"));
    await userEvent.click(await screen.findByRole("option", { name: "262_01" }));
    await userEvent.click(screen.getByRole("button", { name: "运行场景" }));

    expect(await screen.findByText("拒绝运行")).toBeInTheDocument();
    // 输入还在
    expect(screen.getByLabelText(/def_reduction/)).toHaveValue("0.5");
  });
});
// ── 词表 ────────────────────────────────────────────────────────────────────

describe("界面上的标识符都带中文名", () => {
  // 用户的原话是「变量没有 label，看不懂什么意思」。这条测试直接盯着那件事：
  // 标识符可以出现（要能对账），但不能**只有**标识符。
  it("字段与单位都以中文名打头", async () => {
    render(<App />);
    await screen.findByRole("navigation");
    await go("数据", "项目");

    // 字段：一级伤害倍率 + skill.ratio
    expect(await screen.findByText(/一级伤害倍率/)).toBeInTheDocument();
    expect(screen.getAllByText("skill.ratio").length).toBeGreaterThan(0);
    // 分级：结构化 + L2
    expect(screen.getAllByText(/结构化/).length).toBeGreaterThan(0);
  });

  it("主体显示名字，标识符退居其后", async () => {
    render(<App />);
    await screen.findByRole("navigation");
    await go("数据", "项目");
    // 姑获鸟（标识 262_03 是技能；这里断言技能名）
    expect(await screen.findByText(/天翔鹤斩/)).toBeInTheDocument();
  });

});
