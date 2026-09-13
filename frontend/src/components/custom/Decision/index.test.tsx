/**
 * 「待判定」视图的验收测试。
 *
 * 对应 docs/specs/decision.spec.md 的验收标准中**界面能观测到**的那几条：
 * 候选带上下文、都不对与暂缓是两件事、裁决人/理由必填、后端拒绝要显示出来。
 * 领域规则本身在 internal/domain/decision 与 internal/application/disambig 里测。
 */
import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import Decision from "./index";
import { useDecisionStore } from "./store";
import { CallID, callMock } from "../../../test/mock-wails-runtime";

function candidate(over: Record<string, unknown> = {}) {
  return {
    index: 0,
    value: "33",
    unit: "percent",
    anchor: "skills[0].description#m0",
    context: "造成攻击33%伤害",
    note: "",
    ...over,
  };
}

function decisionFixture(over: Record<string, unknown> = {}) {
  return {
    id: "dabc123",
    entity: "skill",
    subject: "262_03",
    predicate: "ratio",
    reason: "描述中出现 2 个伤害倍率（33%, 88%），无法确定取哪一个",
    context: "造成攻击33%伤害，若目标…则造成攻击88%伤害",
    artifact: "Data:Character/262.json",
    revision: "revid:8112",
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
        anchor: "skills[0].description#m1",
        context: "则造成攻击88%伤害",
      }),
    ],
    resolution: null,
    deferredBy: "",
    deferReason: "",
    editable: true,
    ...over,
  };
}

const statsFixture = {
  open: 1,
  deferred: 0,
  decided: 0,
  missing: 0,
  stale: 0,
};

function renderDecision() {
  // 真实页面把 flash 渲染在顶部；测试用同样的方式呈现，
  // 否则「错误必须显示出来」这条就测不到。
  function Harness() {
    const [flash, setFlash] = useState<{ ok: boolean; text: string } | null>(null);
    return (
      <>
        {flash && <div role="status">{flash.text}</div>}
        <Decision by="ngnl5" onByChange={() => {}} onFlash={setFlash} />
      </>
    );
  }
  return render(<Harness />);
}

beforeEach(() => {
  callMock.reset();
  useDecisionStore.getState().reset();
  callMock
    .on(CallID.Decisions, () => [decisionFixture()])
    .on(CallID.DecisionStats, () => statsFixture);
});

afterEach(() => {
  callMock.reset();
});

describe("待判定视图", () => {
  it("列出事项，并写出「为什么排在前面」", async () => {
    renderDecision();
    expect(await screen.findByText("skill/262_03")).toBeInTheDocument();
    expect(screen.getByText("候选仅 2 个，判断成本最低")).toBeInTheDocument();
  });

  // 只给一个数字，人无法判断——上下文是候选的必填部分。
  it("每个候选都带原文片段，不必回去翻原件", async () => {
    renderDecision();
    expect(await screen.findByText("造成攻击33%伤害")).toBeInTheDocument();
    expect(screen.getByText("则造成攻击88%伤害")).toBeInTheDocument();
    expect(screen.getByText("skills[0].description#m1")).toBeInTheDocument();
  });

  it("歧义所在的原文整段可见", async () => {
    renderDecision();
    expect(
      await screen.findByText("造成攻击33%伤害，若目标…则造成攻击88%伤害"),
    ).toBeInTheDocument();
  });

  // 「都不对」是结论，「暂缓」是未处理，两者必须分别可点、不互相顶替。
  it("「都不对」与「暂缓」是两个不同的动作", async () => {
    let resolveArgs: unknown[] = [];
    let deferred = false;
    callMock
      .on(CallID.ResolveDecision, (args) => {
        resolveArgs = args;
        return { applied: true, status: "decided", assertionId: "", message: "已判定候选均不成立" };
      })
      .on(CallID.DeferDecision, () => {
        deferred = true;
        return null;
      });

    renderDecision();
    await userEvent.type(
      await screen.findByPlaceholderText(/主伤害那一句是 88%/),
      "主体那一段才是",
    );
    await userEvent.click(await screen.findByRole("button", { name: "都不对" }));

    await waitFor(() => expect(resolveArgs.length).toBeGreaterThan(0));
    // choice = -1 表示都不对；不得被当成「选了第 0 个」
    expect(resolveArgs[1]).toBe(-1);
    expect(deferred).toBe(false);
    expect(await screen.findByText(/已判定候选均不成立/)).toBeInTheDocument();
  });

  it("裁决选中候选时把序号、裁决人与理由一并送出去", async () => {
    let args: unknown[] = [];
    callMock.on(CallID.ResolveDecision, (a) => {
      args = a;
      return { applied: true, status: "decided", assertionId: "a123", message: "已裁决" };
    });

    renderDecision();
    await userEvent.click(await screen.findByText("则造成攻击88%伤害"));
    await userEvent.type(
      screen.getByPlaceholderText(/主伤害那一句是 88%/),
      "主伤害那一句是 88%",
    );
    await userEvent.click(screen.getByRole("button", { name: "裁决选中候选" }));

    await waitFor(() => expect(args.length).toBeGreaterThan(0));
    const [id, choice, by, method, reason] = args as [string, number, string, string, string];
    expect(id).toBe("dabc123");
    expect(choice).toBe(1);
    expect(by).toBe("ngnl5");
    expect(method).toBe("editorial");
    expect(reason).toBe("主伤害那一句是 88%");
  });

  it("理由为空时拦在前端，不把请求发出去", async () => {
    let called = false;
    callMock.on(CallID.ResolveDecision, () => {
      called = true;
      return { applied: true, status: "decided", assertionId: "a1", message: "" };
    });

    renderDecision();
    await userEvent.click(await screen.findByRole("button", { name: "裁决选中候选" }));

    await waitFor(() => {
      expect(screen.getByText(/必须说明理由/)).toBeInTheDocument();
    });
    expect(called).toBe(false);
  });

  it("后端拒绝时把原因显示出来，而不是静默失败", async () => {
    callMock.on(CallID.ResolveDecision, () => {
      throw new Error("选中的候选未通过准入：schema 未声明该谓词");
    });

    renderDecision();
    await userEvent.type(
      await screen.findByPlaceholderText(/主伤害那一句是 88%/),
      "取 88",
    );
    await userEvent.click(screen.getByRole("button", { name: "裁决选中候选" }));

    expect(await screen.findByText(/schema 未声明该谓词/)).toBeInTheDocument();
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

    renderDecision();
    // 裁决结论与「未选中的候选」都要还在——「当时还有哪些说法」是可追溯性的一部分
    await waitFor(() => expect(screen.getByText(/a123/)).toBeInTheDocument());
    expect(screen.getAllByText("88 percent").length).toBeGreaterThan(0);
    expect(screen.getByText("造成攻击33%伤害")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "裁决选中候选" })).toBeNull();
    expect(screen.getByText(/改判不是覆盖/)).toBeInTheDocument();
  });

  // 「需复核」是必须回到队列里的状态：原选项在新修订里没了。
  it("需复核事项仍在队列里且标出来", async () => {
    callMock.on(CallID.Decisions, () => [
      decisionFixture({ status: "stale", statusText: "需复核", impactReason: "被引用 3 次" }),
    ]);
    callMock.on(CallID.DecisionStats, () => ({ ...statsFixture, open: 0, stale: 1 }));

    renderDecision();
    await waitFor(() => expect(screen.getAllByText("需复核").length).toBeGreaterThan(0));
    expect(screen.getByText("需复核 1")).toBeInTheDocument();
    // 仍需人处理，因此裁决入口必须还在
    expect(screen.getByRole("button", { name: "裁决选中候选" })).toBeInTheDocument();
  });

  it("空队列也要说明「没有歧义不等于数据完整」", async () => {
    callMock.on(CallID.Decisions, () => []);
    callMock.on(CallID.DecisionStats, () => ({ ...statsFixture, open: 0 }));

    renderDecision();
    expect(await screen.findByText(/没有需要人看的待判定事项/)).toBeInTheDocument();
    expect(screen.getByText(/不说明数据已经完整/)).toBeInTheDocument();
  });
});
