/**
 * 来源页的验收测试（docs/specs/document.spec.md 中界面能观测到的条目）。
 *
 * 这一页的存在理由只有一句：**每条断言的依据到底在哪。**
 * 因此测试盯着的都是「溯源能不能追到底」。
 */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { useReviewStore } from "./components/custom/Review/store";
import { useSessionStore } from "./components/custom/Session/store";
import { useSourcesStore } from "./components/custom/Sources/store";
import { CallID, callMock } from "./test/mock-wails-runtime";
import { expectBold, expectNoMarkers } from "./test/prose";
import { GlossaryCallID, glossaryFixture } from "./test/glossary-fixture";

const session = {
  dir: "projects/onmyoji",
  name: "onmyoji",
  projectDir: "projects/onmyoji",
  description: "阴阳师",
  scenarios: [
    { name: "damage-calc", description: "伤害计算", inputs: 0, requiresMet: true, requiresTotal: 1, skipped: "" },
  ],
  scenario: "damage-calc",
  scenarioSkipped: [],
};

function doc(over: Record<string, unknown> = {}) {
  return {
    revId: "rev1",
    docId: "doc1",
    source: "huijiwiki",
    title: "Data:Attribute.json",
    kind: "evidence",
    kindText: "依据",
    revision: "revid:9263",
    hash: "sha256:ed4dd4331b45693d",
    capturedAt: "2026-03-01T10:00:00Z",
    registeredAt: "2026-09-13T12:00:00Z",
    body: "",
    artifactPath: ".huiji/raw/Data_Attribute.json",
    appliesVersions: [],
    appliesScenarios: [],
    status: "unverified",
    statusText: "未核验",
    verifiedBy: null,
    method: "",
    reason: "",
    current: true,
    usageCount: 2948,
    supersededBy: "",
    ...over,
  };
}

function item(over: Record<string, unknown> = {}) {
  return {
    id: "a1",
    entity: "shikigami",
    subject: "262",
    predicate: "atk",
    value: "3082",
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
    .on(CallID.DocList, () => [doc()])
    .on(CallID.DocUnregistered, () => [])
    .on(CallID.DocHistory, () => [doc()])
    .on(CallID.DocUsage, () => [item()])
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

async function goSources() {
  render(<App />);
  await screen.findByRole("navigation");
  const btn = within(navGroup("项目")).getByRole("button", { name: "来源" });
  await waitFor(() => expect(btn).toBeEnabled());
  await userEvent.click(btn);
}

/** 到核验队列，并选中第一条（详情区里才有「原件」）。 */
async function goQueueAndPick(subject: RegExp) {
  render(<App />);
  await screen.findByRole("navigation");
  const btn = within(navGroup("核验")).getByRole("button", { name: "队列" });
  await waitFor(() => expect(btn).toBeEnabled());
  await userEvent.click(btn);
  await userEvent.click(await screen.findByText(subject));
}

function queueItem(over: Record<string, unknown> = {}) {
  return {
    item: item(over),
    tier: "影响面",
    reason: "被 1 条派生断言引用",
    score: 1010,
    sampled: false,
  };
}

beforeEach(() => {
  callMock.reset();
  useSessionStore.getState().reset();
  useSourcesStore.getState().reset();
  // 核验详情区的选中项也是会话状态：不清就会把上一条测试的断言留在详情里
  useReviewStore.getState().reset();
  backend();
});

afterEach(() => {
  callMock.reset();
});

describe("来源", () => {
  it("列出文档，并给出被多少条断言引用", async () => {
    await goSources();
    expect((await screen.findAllByText("Data:Attribute.json")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("依据").length).toBeGreaterThan(0);
    // 影响面：2948 条断言指着这份原文
    expect(screen.getByText("2948")).toBeInTheDocument();
    expect(screen.getAllByText("未核验").length).toBeGreaterThan(0);
  });

  // 摘要由内容算出——修订标识可以撒谎，摘要不会
  it("同时显示修订标识与内容摘要", async () => {
    await goSources();
    expect((await screen.findAllByText("revid:9263")).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/sha256:ed4dd433/).length).toBeGreaterThan(0);
  });

  it("详情显示归档路径与采集时间", async () => {
    await goSources();
    await waitFor(() => {
      expect(screen.getByText(/\.huiji\/raw\/Data_Attribute\.json/)).toBeInTheDocument();
    });
    expect(screen.getByText("采集时间")).toBeInTheDocument();
    expect(screen.getByText("登记时间")).toBeInTheDocument();
  });

  it("能看到引用它的断言", async () => {
    await goSources();
    expect((await screen.findAllByText(/引用它的断言/)).length).toBeGreaterThan(0);
    expect(screen.getByText(/姑获鸟/)).toBeInTheDocument();
  });

  // 验收：核验人必须是人、理由必填
  it("核验需要核验人与理由，缺任一项不发请求", async () => {
    let called = false;
    callMock.on(CallID.DocVerify, () => {
      called = true;
      return doc({ status: "verified", statusText: "已核验" });
    });

    await goSources();
    await userEvent.click(await screen.findByRole("button", { name: "核验" }));

    expect(await screen.findByText(/必须填写核验人/)).toBeInTheDocument();
    expect(called).toBe(false);
  });

  // 验收：方法只能是编审——一个人读过不构成多源
  it("核验时固定用编审方法", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.DocVerify, (a) => {
      args = a;
      return doc({ status: "verified", statusText: "已核验" });
    });

    await goSources();
    await userEvent.type(await screen.findByLabelText("核验人（必须是人）"), "ngnl5");
    await userEvent.type(screen.getByLabelText("理由（必填）"), "通读一遍，与游戏内一致");
    await userEvent.click(screen.getByRole("button", { name: "核验" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    expect(args[0]).toBe("rev1");
    expect(args[1]).toBe("ngnl5");
    expect(args[2]).toBe("editorial");
    expect(args[3]).toBe("通读一遍，与游戏内一致");
  });

  it("界面说明核验的是原文，不是原文里的事实", async () => {
    await goSources();
    expect(await screen.findByText(/「原文里的事实已被验证」/)).toBeInTheDocument();
    // 强调渲染为加粗，且页面上不留 markdown 记号
    expectBold("不是");
    expectNoMarkers();
    expect(screen.getByText(/一个人读过并背书不构成「多源」/)).toBeInTheDocument();
  });

  // 验收：还剩多少依据没登记必须可见
  it("未登记依据页把剩余量摆出来", async () => {
    callMock.on(CallID.DocUnregistered, () => [
      { artifact: "formula:crit_factor", assertions: 268 },
    ]);
    await goSources();
    await userEvent.click(await screen.findByRole("tab", { name: /未登记依据/ }));

    expect(await screen.findByText("formula:crit_factor")).toBeInTheDocument();
    expect(screen.getByText(/这些原件还没有对应的文档/)).toBeInTheDocument();
  });

  it("没有未登记依据时说明这正是这一页的意义", async () => {
    await goSources();
    await userEvent.click(await screen.findByRole("tab", { name: /未登记依据/ }));
    expect(await screen.findByText(/溯源的终点不该是一个字符串/)).toBeInTheDocument();
  });

  // 验收：修订历史保留，旧修订不被删
  it("修订历史列出全部版本，并标出当前", async () => {
    callMock.on(CallID.DocHistory, () => [
      doc({ revId: "rev2", revision: "revid:9300", current: true }),
      doc({
        revId: "rev1",
        revision: "revid:9263",
        current: false,
        status: "superseded",
        statusText: "已被新修订取代",
        supersededBy: "rev2",
      }),
    ]);
    await goSources();
    expect((await screen.findAllByText(/修订历史/)).length).toBeGreaterThan(0);
    expect(screen.getByText("已被新修订取代")).toBeInTheDocument();
    expect(screen.getAllByText("当前").length).toBeGreaterThan(0);
    expect(screen.getByText(/被 rev2 取代/)).toBeInTheDocument();
  });

  it("适用范围未声明时说明是全项目适用", async () => {
    await goSources();
    expect((await screen.findAllByText(/全项目适用/)).length).toBeGreaterThan(0);
  });

  // 验收：变更传导必须说出来
  it("登记新修订时把「多少条断言回到待核验」说出来", async () => {
    callMock.on(CallID.DocRegister, () => ({
      doc: doc({ revId: "rev9", revision: "v2" }),
      changed: true,
      reverted: false,
      requeued: 268,
    }));
    await goSources();
    await userEvent.type(await screen.findByLabelText("标题"), "防御减免为何无权威来源");
    await userEvent.type(screen.getByLabelText("原文"), "防御减免没有权威来源。");
    await userEvent.click(screen.getByRole("button", { name: "登记" }));

    expect(await screen.findByText(/268 条断言回到待核验/)).toBeInTheDocument();
  });

  // 自撰文档必须给正文——没有归档可依
  it("登记自撰文档时正文必填", async () => {
    await goSources();
    await userEvent.type(await screen.findByLabelText("标题"), "只有标题");
    expect(screen.getByRole("button", { name: "登记" })).toBeDisabled();
  });

  it("空列表说明它为什么存在", async () => {
    callMock.on(CallID.DocList, () => []);
    await goSources();
    expect(await screen.findByText(/还没有登记任何文档/)).toBeInTheDocument();
    expect(screen.getByText(/跑一次 sync 会把归档的原件登记进来/)).toBeInTheDocument();
  });

  // 验收：断言的任何呈现处都能跳到它的依据文档，并定位到那一份
  it("从断言点「查看依据」直接打开那一份，而不是打开列表首项", async () => {
    callMock
      .on(CallID.Queue, () => [queueItem({ artifact: "Data:Character/262.json" })])
      .on(CallID.DocList, () => [
        doc(),
        doc({
          revId: "rev2",
          docId: "doc2",
          title: "Data:Character/262.json",
          artifactPath: ".huiji/raw/Data_Character_262.json",
          usageCount: 5,
        }),
      ]);

    await goQueueAndPick(/姑获鸟/);
    await userEvent.click(await screen.findByTitle("查看依据：Data:Character/262.json"));

    // 打开的必须是 262 那一份——打开第一份等于没跳
    expect(await screen.findByText(/\.huiji\/raw\/Data_Character_262\.json/)).toBeInTheDocument();
    expect(screen.queryByText(/\.huiji\/raw\/Data_Attribute\.json/)).not.toBeInTheDocument();
  });

  // 验收：依据未登记时，入口明说「依据未登记」，而不是打开一个空白页
  it("依据未登记时说明是未登记，不是加载失败", async () => {
    callMock
      .on(CallID.Queue, () => [queueItem({ artifact: "formula:crit_factor" })])
      // 文档列表里没有 formula:crit_factor 这一份
      .on(CallID.DocList, () => [doc()]);

    await goQueueAndPick(/姑获鸟/);
    await userEvent.click(await screen.findByTitle("查看依据：formula:crit_factor"));

    expect(await screen.findByText(/还没有登记为文档/)).toBeInTheDocument();
    expect(screen.getAllByText(/formula:crit_factor/).length).toBeGreaterThan(0);
    expect(screen.getByText(/这不是加载失败/)).toBeInTheDocument();
  });

  // 派生断言没有原件：不显示空的原件入口
  it("没有原件的断言不显示依据入口", async () => {
    callMock
      .on(CallID.Queue, () => [queueItem({ artifact: "", confidence: "L3" })])
      .on(CallID.DocList, () => [doc()]);

    await goQueueAndPick(/姑获鸟/);
    expect(screen.queryByTitle(/^查看依据：/)).not.toBeInTheDocument();
  });
});
