/**
 * 说明文字的断言助手。
 *
 * 界面上约定 `**这样**` 渲染为加粗（见 `components/custom/Prose`），
 * 因此一句话在 DOM 里会被拆成多个文本节点——`getByText(/整句/)` 匹配不到，
 * 而按元素逐个查又会写出很啰嗦的断言。这里把两件事固定下来：
 *
 *  1. 「这段文字是加粗的」怎么断言
 *  2. 「页面上没有漏渲染的 markdown 记号」怎么断言
 *
 * 第 2 条是真正要防的回归：记号写进 JSX 却忘了渲染，
 * 屏幕上就会出现一堆星号，而那不会让任何一条别的测试失败。
 */
import { screen } from "@testing-library/react";
import { expect } from "vitest";

/** 页面上存在这段**加粗**文字。 */
export function expectBold(text: string) {
  const hit = screen.getAllByText(text).filter((e) => e.tagName === "STRONG");
  expect(hit.length, `页面上没有加粗的「${text}」`).toBeGreaterThan(0);
}

/** 页面上没有漏渲染的 markdown 记号。 */
export function expectNoMarkers() {
  expect(document.body.textContent ?? "").not.toContain("**");
}
