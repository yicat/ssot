import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { CallID, callMock } from "./test/mock-wails-runtime";

/** 后端返回的最小数据。 */
function statsFixture(pending = 2, conflicts = 0) {
  return {
    total: pending,
    byStatus: { pending },
    byEntity: { shikigami: pending },
    byConfidence: { L1: pending },
    verifications: 0,
    conflicts,
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

beforeEach(() => {
  callMock.reset();
  callMock
    .on(CallID.ProjectDir, () => "projects/onmyoji")
    .on(CallID.Stats, () => statsFixture())
    .on(CallID.Queue, () => [queueFixture()])
    .on(CallID.Conflicts, () => [])
    .on(CallID.DecisionStats, () => ({
      open: 0,
      deferred: 0,
      decided: 0,
      missing: 0,
      stale: 0,
    }))
    .on(CallID.History, () => []);
});

afterEach(() => {
  callMock.reset();
});

describe("核验工作台", () => {
  it("显示项目路径与未核验数量——未核验必须一眼可见", async () => {
    render(<App />);
    expect(await screen.findByText("projects/onmyoji")).toBeInTheDocument();
    expect(await screen.findByText("核验工作台")).toBeInTheDocument();
    // 「待核验」在筛选下拉里也有，因此按出现次数断言
    expect((await screen.findAllByText("待核验")).length).toBeGreaterThan(0);
  });

  // 排在前面的理由必须写出来，否则人只能盲信排序。
  it("队列显示优先级类别与主导理由", async () => {
    render(<App />);
    expect(await screen.findByText("影响面")).toBeInTheDocument();
    expect(screen.getByText("被 1 条派生断言引用")).toBeInTheDocument();
  });

  it("抽检项被显式标出，不与普通项混淆", async () => {
    callMock.on(CallID.Queue, () => [queueFixture({ sampled: true })]);
    render(<App />);
    expect(await screen.findByText("抽检")).toBeInTheDocument();
  });

  it("选中后展示溯源——必须能看到来自哪个原件、哪个位置、哪个修订", async () => {
    render(<App />);
    await userEvent.click(await screen.findByText("shikigami/262"));
    await waitFor(() => {
      expect(screen.getByText("Data:Attribute.json")).toBeInTheDocument();
    });
    expect(screen.getByText("attributes[0].atk")).toBeInTheDocument();
    expect(screen.getByText("revid:9263")).toBeInTheDocument();
  });

  it("批准需要批准者与理由——缺任一项后端会拒绝", async () => {
    let sawArgs: unknown[] = [];
    callMock.on(CallID.Approve, (args) => {
      sawArgs = args;
      return null;
    });

    render(<App />);
    await userEvent.click(await screen.findByText("shikigami/262"));
    await userEvent.type(screen.getByPlaceholderText("你的名字"), "yicat");
    await userEvent.type(
      screen.getByPlaceholderText("例：与原文逐字比对一致"),
      "与原文逐字比对一致",
    );
    await userEvent.click(screen.getByRole("button", { name: "批准" }));

    await waitFor(() => {
      expect(sawArgs.length).toBeGreaterThan(0);
    });
    const [, by, method, reason] = sawArgs as string[];
    expect(by).toBe("yicat");
    expect(method).toBe("editorial");
    expect(reason).toBe("与原文逐字比对一致");
  });

  it("后端拒绝时把原因显示出来，而不是静默失败", async () => {
    callMock.on(CallID.Approve, () => {
      throw new Error("批准者必须是**人**");
    });

    render(<App />);
    await userEvent.click(await screen.findByText("shikigami/262"));
    await userEvent.click(screen.getByRole("button", { name: "批准" }));

    expect(await screen.findByText(/批准者必须是/)).toBeInTheDocument();
  });

  it("选实测方法时才要求填依据", async () => {
    render(<App />);
    await userEvent.click(await screen.findByText("shikigami/262"));

    expect(screen.queryByPlaceholderText(/版本 2026-09/)).toBeNull();

    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "核验方法" }),
      "measurement",
    );
    expect(await screen.findByPlaceholderText(/版本 2026-09/)).toBeInTheDocument();
  });

  // 冲突必须成组呈现，且系统不替人裁决。
  it("冲突页签把同一件事的多种说法并列摆出", async () => {
    callMock.on(CallID.Conflicts, () => [conflictFixture()]);
    render(<App />);

    await userEvent.click(await screen.findByRole("button", { name: "冲突" }));

    expect(await screen.findByText("605_01.character_id")).toBeInTheDocument();
    expect(screen.getByText("606 point")).toBeInTheDocument();
    expect(screen.getByText("605 point")).toBeInTheDocument();
    // 两条说法各自的出处都要能看到
    expect(screen.getByText(/Data:Character\/606\.json/)).toBeInTheDocument();
    expect(screen.getByText(/Data:Character\/605\.json/)).toBeInTheDocument();
  });

  it("无冲突时明确说明「没有冲突不等于数据正确」", async () => {
    render(<App />);
    await userEvent.click(await screen.findByRole("button", { name: "冲突" }));
    expect(await screen.findByText(/不说明数据已经正确/)).toBeInTheDocument();
  });

  // 批量最容易造成大面积错误，因此执行前必须先看见影响面。
  it("批量操作先预览影响面，不直接执行", async () => {
    let previewed = false;
    let approved = false;
    callMock.on(CallID.BatchPreview, () => {
      previewed = true;
      return {
        matched: 268,
        where: "实体=shikigami 且 谓词=id",
        sample: [itemFixture({ subject: "200", predicate: "id", value: "200 point" })],
      };
    });
    callMock.on(CallID.BatchApprove, () => {
      approved = true;
      return { matched: 268, applied: 268, failed: 0, firstErr: "" };
    });

    render(<App />);
    await userEvent.click(await screen.findByRole("button", { name: "预览影响面" }));

    expect(await screen.findByText("268")).toBeInTheDocument();
    expect(screen.getByText("实体=shikigami 且 谓词=id")).toBeInTheDocument();
    expect(previewed).toBe(true);
    expect(approved).toBe(false); // 预览阶段不得执行

    await userEvent.click(screen.getByRole("button", { name: "批准全部" }));
    await waitFor(() => {
      expect(approved).toBe(true);
    });
    expect(await screen.findByText(/批量批准：成功 268/)).toBeInTheDocument();
  });
});
