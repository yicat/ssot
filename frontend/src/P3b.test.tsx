/**
 * 备选方案页的验收测试（docs/specs/alternatives.spec.md 中界面能观测到的条目）。
 *
 * 特别测「界面上**没有**什么」：没有推荐标记、没有默认选中项。
 * 一个预选中的方案会让人以为那就是推荐，而这一层要守的边界恰恰是
 * 「系统不替使用者做取舍」。
 */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { useAlternativesStore } from "./components/custom/Alternatives/store";
import { useSessionStore } from "./components/custom/Session/store";
import { CallID, callMock } from "./test/mock-wails-runtime";
import { expectBold, expectNoMarkers } from "./test/prose";
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
      inputs: 0,
      requiresMet: true,
      requiresTotal: 1,
      skipped: "",
    },
  ],
  scenario: "damage-calc",
  scenarioSkipped: [],
};

function plan(over: Record<string, unknown> = {}) {
  return {
    id: "p1",
    scenario: "damage-calc",
    title: "先刷御魂那条线",
    objective: "收益最大",
    constraints: ["不花勾玉"],
    assumptions: ["每天在线 30 分钟"],
    preference: "省时间",
    actions: ["刷御魂", "换暴击"],
    metrics: [
      { name: "资源", value: 100, unit: "point", lowerIsBetter: true },
      { name: "时间", value: 30, unit: "min", lowerIsBetter: true },
    ],
    opportunity: "放弃另一条线的进度",
    depends: ["assert:a1"],
    unverifiedRatio: 0.5,
    maxConfidence: "L2",
    sources: ["huijiwiki"],
    fromConflict: false,
    status: "candidate",
    statusText: "候选",
    executable: true,
    chosen: false,
    chosenBy: "",
    chooseReason: "",
    at: "2026-03-01T12:00:00Z",
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
    .on(CallID.AltList, () => [plan()])
    .on(CallID.AltEvaluate, () => ({
      plans: [plan()],
      pruned: [],
      merged: [],
      constraints: [],
      note: "",
      preferenceInferred: "",
    }))
    .on(CallID.AltRefresh, () => ({ checked: 1, invalid: [], stale: [] }))
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

async function goAlternatives() {
  render(<App />);
  await screen.findByRole("navigation");
  await userEvent.click(
    within(navGroup("场景：damage-calc")).getByRole("button", { name: "备选方案" }),
  );
}

beforeEach(() => {
  callMock.reset();
  useSessionStore.getState().reset();
  useAlternativesStore.getState().reset();
  backend();
});

afterEach(() => {
  callMock.reset();
});

describe("备选方案", () => {
  // 验收：每个方案都声明目标、约束、代价（含机会成本）与未核验比例。
  it("每个方案都自证：目标、代价、机会成本、未核验比例", async () => {
    await goAlternatives();
    expect(await screen.findByText("先刷御魂那条线")).toBeInTheDocument();
    expect(screen.getByText(/目标：收益最大/)).toBeInTheDocument();
    expect(screen.getByText(/机会成本：/)).toBeInTheDocument();
    expect(screen.getByText("放弃另一条线的进度")).toBeInTheDocument();
    expect(screen.getByText(/未核验 50%/)).toBeInTheDocument();
    expect(screen.getByText(/可信度上限/)).toBeInTheDocument();
    // 分级现在显示中文名 + 标识符，两边都要在
    expect(screen.getAllByText(/结构化/).length).toBeGreaterThan(0);
  });

  // 验收：可比维度并排呈现。
  it("把可比维度摊开", async () => {
    await goAlternatives();
    expect(await screen.findByText("资源")).toBeInTheDocument();
    expect(screen.getByText(/100/)).toBeInTheDocument();
    expect(screen.getAllByText(/点数/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("越小越好").length).toBeGreaterThan(0);
  });

  // 界面上没有推荐、没有默认选中——那会让人以为系统替你选了。
  it("没有推荐标记，也没有默认选中项", async () => {
    await goAlternatives();
    await screen.findByText("先刷御魂那条线");
    expect(screen.queryByText(/推荐/)).toBeNull();
    expect(screen.queryByText("你选的")).toBeNull();
    // 选定必须由人点
    expect(screen.getByRole("button", { name: /选定 先刷御魂那条线/ })).toBeInTheDocument();
  });

  // 验收：方案可信度不得高于其依赖断言的最低——界面不提供修改入口。
  it("界面上没有修改可信度的入口", async () => {
    await goAlternatives();
    await screen.findByText("先刷御魂那条线");
    expect(screen.queryByLabelText(/可信度/)).toBeNull();
    expect(screen.getByText(/可信度上限/)).toBeInTheDocument();
    // 分级现在显示中文名 + 标识符，两边都要在
    expect(screen.getAllByText(/结构化/).length).toBeGreaterThan(0);
  });

  // 验收：未声明偏好时不得给出单一方案。
  it("未声明偏好且只有一种偏好假设时，把原因显示出来", async () => {
    callMock.on(CallID.AltEvaluate, () => {
      throw new Error(
        "未声明偏好时不得给出单一方案：需要至少两个标注了不同偏好的备选，当前只有 1 种偏好（[省时间]）",
      );
    });
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "对比并剪枝" }));
    expect(await screen.findByText(/未声明偏好时不得给出单一方案/)).toBeInTheDocument();
  });

  // 验收：仅有一个方案时明确说明未发现实质不同的备选。
  it("只有一个方案时说清「没得比」", async () => {
    callMock.on(CallID.AltEvaluate, () => ({
      plans: [plan()],
      pruned: [],
      merged: [],
      constraints: [],
      note: "未发现实质不同的备选——这不是「这个方案最好」，而是「没得比」",
      preferenceInferred: "",
    }));
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "对比并剪枝" }));
    expect(await screen.findByText(/未发现实质不同的备选/)).toBeInTheDocument();
  });

  // 验收：全部方案被剪枝时，输出的是冲突的约束清单而非空结果。
  it("约束互相打架时列出清单，而不是空结果", async () => {
    callMock.on(CallID.AltEvaluate, () => ({
      plans: [],
      pruned: [],
      merged: [],
      constraints: ["不花勾玉（方案 [p1]）", "必须用勾玉（方案 [p2]）"],
      note: "没有方案满足全部约束：这不是算不出来，而是你的要求互相打架",
      preferenceInferred: "",
    }));
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "对比并剪枝" }));

    expect(await screen.findByText(/你的要求互相打架/)).toBeInTheDocument();
    expect(screen.getByText("不花勾玉（方案 [p1]）")).toBeInTheDocument();
    expect(screen.getByText("必须用勾玉（方案 [p2]）")).toBeInTheDocument();
  });

  // 验收：被支配的方案被剪枝，且说得出是被谁支配的。
  it("剪枝必须说得出是被谁支配的", async () => {
    callMock.on(CallID.AltEvaluate, () => ({
      plans: [plan({ id: "good", title: "更省的那条" })],
      pruned: [
        {
          plan: plan({ id: "bad", title: "更贵的那条" }),
          by: "good",
          dominance: "good 在所有维度上都不劣于它，且至少一个维度更优",
        },
      ],
      merged: [],
      constraints: [],
      note: "",
      preferenceInferred: "",
    }));
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "对比并剪枝" }));

    expect(await screen.findByText(/更贵的那条/)).toBeInTheDocument();
    expect(screen.getByText(/在所有维度上都不劣于它/)).toBeInTheDocument();
    expect(screen.getAllByText(/剪掉 1 个被支配的/).length).toBeGreaterThan(0);
  });

  // 验收：偏好推断只是建议，必须经显式确认才生效。
  it("偏好推断标注为建议，且不删除其他备选", async () => {
    callMock
      .on(CallID.AltList, () => [plan(), plan({ id: "p2", title: "省资源那条线", preference: "省资源" })])
      .on(CallID.AltEvaluate, () => ({
        plans: [plan(), plan({ id: "p2", title: "省资源那条线", preference: "省资源" })],
        pruned: [],
        merged: [],
        constraints: [],
        note: "",
        preferenceInferred: "省时间",
      }));
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "对比并剪枝" }));

    await screen.findByText(/这只是/);
    // 强调渲染为加粗，且页面上不留 markdown 记号
    expectBold("建议");
    expectNoMarkers();
    expect(screen.getByText("省资源那条线")).toBeInTheDocument();
  });

  // 验收：玩家选择与理由被持久化；选定者必须是人。
  it("选定需要人以及理由，缺任一项不发请求", async () => {
    let called = false;
    callMock.on(CallID.AltChoose, () => {
      called = true;
      return plan({ status: "chosen", statusText: "已选中", chosen: true });
    });

    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: /选定 先刷御魂那条线/ }));

    expect(await screen.findByText(/必须填写选定人/)).toBeInTheDocument();
    expect(called).toBe(false);
  });

  it("填齐后送出选定人与理由", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.AltChoose, (a) => {
      args = a;
      return plan({ status: "chosen", statusText: "已选中", chosen: true });
    });

    await goAlternatives();
    await userEvent.type(screen.getByLabelText("选定人（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "时间更短");
    await userEvent.click(screen.getByRole("button", { name: /选定 先刷御魂那条线/ }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    expect(args[0]).toBe("p1");
    expect(args[1]).toBe("ngnl5");
    expect(args[2]).toBe("时间更短");
  });

  // 验收：玩家选定方案后，其他备选仍可访问。
  it("选定之后其他备选仍在列表里", async () => {
    callMock
      .on(CallID.AltChoose, () => plan({ status: "chosen", statusText: "已选中", chosen: true }))
      .on(CallID.AltList, () => [
        plan({ status: "chosen", statusText: "已选中", chosen: true }),
        plan({ id: "p2", title: "省资源那条线", preference: "省资源" }),
      ]);

    await goAlternatives();
    await userEvent.type(screen.getByLabelText("选定人（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "时间更短");
    await userEvent.click(screen.getByRole("button", { name: /选定 先刷御魂那条线/ }));

    expect(await screen.findByText("你选的")).toBeInTheDocument();
    expect(screen.getByText("省资源那条线")).toBeInTheDocument();
  });

  // 验收：依据被驳回后，方案不可执行。
  it("依据失效的方案不可执行，选定按钮被禁用", async () => {
    callMock.on(CallID.AltList, () => [
      plan({ status: "invalid", statusText: "不可执行", executable: false }),
    ]);
    await goAlternatives();
    expect(await screen.findByText("不可执行")).toBeInTheDocument();
    expect(screen.getByText("依据已失效，不可执行")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /选定 先刷御魂那条线/ })).toBeDisabled();
  });

  // 验收：依据变更后方案被标记为「依据已变」，但仍可执行（是提示不是硬拦截）。
  it("检查依据把结果说出来", async () => {
    callMock.on(CallID.AltRefresh, () => ({ checked: 3, invalid: ["p1"], stale: ["p2"] }));
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "检查依据" }));
    expect(await screen.findByText(/检查 3 个，不可执行 1 个，依据已变 1 个/)).toBeInTheDocument();
  });

  it("空列表说明方案不是「最优解」，而是一组取舍", async () => {
    callMock.on(CallID.AltList, () => []);
    await goAlternatives();
    expect(await screen.findByText(/这个场景还没有方案/)).toBeInTheDocument();
    expect(screen.getByText(/一组取舍不同的备选/)).toBeInTheDocument();
  });

  // 界面不得提供「可信度」输入：它由依赖推导。
  it("提出方案时不收集可信度", async () => {
    await goAlternatives();
    await userEvent.click(await screen.findByRole("button", { name: "提出方案" }));
    expect(await screen.findByLabelText("目标")).toBeInTheDocument();
    expect(screen.getByLabelText("机会成本")).toBeInTheDocument();
    expect(screen.queryByLabelText(/可信度/)).toBeNull();
    expect(screen.getByText(/方案的可信度不在这里填/)).toBeInTheDocument();
  });
});
