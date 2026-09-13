// 关键词覆盖扫描：检查已同步的数据里是否包含某主题的内容。
// 用途：回答「我们现在手上的数据能不能支撑某个问题」——避免在数据缺失时开始设计。
//
// 用法：
//   node tools/wiki/scan.mjs 勾玉 御行达摩 蓝票
//   node tools/wiki/scan.mjs            # 使用内置的默认词表
//
// 注意：命中 ≠ 数据可得。例如「斗技」在 423 个页面里出现 109 次，
// 但全部来自式神传记文本，没有任何一条是斗技数据。命中只说明「值得去看」。

import fs from "node:fs";
import path from "node:path";
import { RAW, loadManifest } from "./lib.mjs";

const DEFAULT_TERMS = [
  "勾玉", "御行达摩", "黑蛋", "蓝票", "神秘的符咒", "现世符咒",
  "超鬼王", "斗技", "活动", "体力", "碎片", "召唤",
  "寮", "副本", "奖励", "兑换", "商店", "版本", "公告",
];

const terms = process.argv.slice(2).length ? process.argv.slice(2) : DEFAULT_TERMS;

if (!fs.existsSync(RAW)) {
  console.error(`未找到缓存目录 ${RAW}，请先运行 node tools/wiki/sync.mjs`);
  process.exit(1);
}

const files = fs.readdirSync(RAW);
const manifest = (() => {
  try {
    return loadManifest();
  } catch {
    return null;
  }
})();

// 标题 → 内容 的映射（用 manifest 拿原始标题，比净化后的文件名可读）
const titleOf = new Map();
if (manifest) {
  for (const p of manifest.pages) {
    titleOf.set(p.title.replace(/[:/\\]/g, "_"), p.title);
  }
}

console.log(`扫描 ${files.length} 个已同步页面`);
if (manifest) console.log(`同步时间 ${manifest.fetchedAt}`);
console.log();

const stats = new Map(terms.map((t) => [t, { files: 0, count: 0, sample: [] }]));

for (const f of files) {
  const s = fs.readFileSync(path.join(RAW, f), "utf8");
  for (const t of terms) {
    const n = s.split(t).length - 1;
    if (n > 0) {
      const st = stats.get(t);
      st.files++;
      st.count += n;
      if (st.sample.length < 3) st.sample.push(titleOf.get(f) ?? f);
    }
  }
}

const w = Math.max(...terms.map((t) => t.length), 4);
console.log(`${"关键词".padEnd(w + 2)} 文件数   总次数   样例`);
for (const t of terms) {
  const s = stats.get(t);
  const flag = s.files === 0 ? "  ← 完全没有" : "";
  console.log(
    `${t.padEnd(w + 2)} ${String(s.files).padStart(5)}  ${String(s.count).padStart(7)}   ${s.sample.join(", ").slice(0, 48)}${flag}`,
  );
}

const missing = terms.filter((t) => stats.get(t).files === 0);
console.log();
if (missing.length) {
  console.log(`⚠️  以下主题在已同步数据中完全不存在：${missing.join("、")}`);
  console.log("    → 若某个需求依赖这些主题，说明还需要同步其他数据源（当前只同步了 Data 命名空间）");
} else {
  console.log("所有关键词均有命中——但注意命中 ≠ 数据可得，仍需检查命中内容的性质");
}
