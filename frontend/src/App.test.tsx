import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import App from "./App";
import { CallID, callMock } from "./test/mock-wails-runtime";

/** 后端返回的最小数据。 */
function statsFixture(pending = 2) {
  return {
    total: pending,
    byStatus: { pending },
    byEntity: { shikigami: pending },
    byConfidence: { L1: pending },
    verifications: 0,
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

beforeEach(() => {
  callMock.reset();
  callMock
    .on(CallID.ProjectDir, () => "projects/onmyoji")
    .on(CallID.Stats, () => statsFixture())
    .on(CallID.Pending, () => [itemFixture()])
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
    // 状态徽章把「待核验」显式标出，而不是藏起来
    expect(await screen.findAllByText("待核验")).not.toHaveLength(0);
  });

  it("列出待核验断言并显示其分级与取值", async () => {
    render(<App />);
    expect(await screen.findByText("shikigami/262")).toBeInTheDocument();
    expect(screen.getByText("3082 point")).toBeInTheDocument();
    expect(screen.getByText("L1")).toBeInTheDocument();
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

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "核验方法" }), "measurement");
    expect(await screen.findByPlaceholderText(/版本 2026-09/)).toBeInTheDocument();
  });

  it("驳回后显示结果并刷新队列", async () => {
    let rejected = false;
    callMock.on(CallID.Reject, () => {
      rejected = true;
      return null;
    });
    callMock.on(CallID.Stats, () => statsFixture(rejected ? 0 : 2));

    render(<App />);
    await userEvent.click(await screen.findByText("shikigami/262"));
    await userEvent.type(screen.getByPlaceholderText("你的名字"), "yicat");
    await userEvent.type(screen.getByPlaceholderText("例：与原文逐字比对一致"), "取值可疑");
    await userEvent.click(screen.getByRole("button", { name: "驳回" }));

    // 精确匹配操作结果提示（「已驳回」在下拉选项里也有，不能只按文本找）
    expect(await screen.findByText(/已驳回\s+262\.atk/)).toBeInTheDocument();
    expect(rejected).toBe(true);
  });
});
