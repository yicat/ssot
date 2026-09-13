// 数据体检：对已同步的本地缓存跑校验，不联网。
//
// 用法：node tools/wiki/verify.mjs
// 退出码：0 = 无 ERROR；1 = 存在 ERROR（可用于 CI）

import {
  loadRaw,
  loadJson,
  loadManifest,
  characterPages,
} from "./lib.mjs";

const problems = [];
const error = (m) => { problems.push({ level: "ERROR", m }); console.log(`  ❌ ${m}`); };
const warn = (m) => { problems.push({ level: "WARN", m }); console.log(`  ⚠️  ${m}`); };
const ok = (m) => console.log(`  ✅ ${m}`);

const manifest = loadManifest();
const charPages = characterPages(manifest);

console.log(`数据体检  |  源 ${manifest.source}  |  抓取于 ${manifest.fetchedAt}\n`);

// ── 1. 清单概览 ─────────────────────────────────────────────────────────────
console.log("[1] 清单概览");
console.log(`  页面 ${manifest.count}  |  缺失内容 ${manifest.pages.filter((p) => p.missing).length}`);
const groups = {};
for (const p of manifest.pages) {
  const g = p.title.replace(/^Data:/, "").split(/[/.]/)[0];
  groups[g] = (groups[g] || 0) + 1;
}
console.log(
  "  " +
    Object.entries(groups)
      .sort((a, b) => b[1] - a[1])
      .map(([k, v]) => `${k}=${v}`)
      .join("  "),
);

// ── 2. 穷尽性（三方：索引 / 属性表 / 角色文件）────────────────────────────────
console.log("\n[2] 穷尽性检查");
const index = loadJson("Data:CharacterIndex.json");
const attr = loadJson("Data:Attribute.json");
if (!index || !attr) {
  error("缺少 Data:CharacterIndex.json 或 Data:Attribute.json，无法做穷尽性检查");
} else {
  const idxIds = new Set(index.characters.map((c) => c.id));
  const attrIds = new Set(attr.attributes.map((a) => a.id));
  const fileIds = new Set(charPages.map((p) => p.id));
  console.log(`  CharacterIndex ${idxIds.size}  |  Attribute ${attrIds.size}  |  Character 文件 ${fileIds.size}`);

  const orphanFiles = [...fileIds].filter((i) => !idxIds.has(i));
  const missingFiles = [...idxIds].filter((i) => !fileIds.has(i));
  const idxNotAttr = [...idxIds].filter((i) => !attrIds.has(i));
  const attrNotIdx = [...attrIds].filter((i) => !idxIds.has(i));

  if (missingFiles.length) error(`索引有 ${missingFiles.length} 个角色但无对应文件: ${missingFiles.join(",")}`);
  else ok("索引中的每个角色都有对应文件");

  if (attrNotIdx.length) error(`Attribute 有 ${attrNotIdx.length} 个 id 不在索引中: ${attrNotIdx.join(",")}`);
  else ok("Attribute 的 id 全部在索引中");

  if (idxNotAttr.length) warn(`索引有 ${idxNotAttr.length} 个角色不在 Attribute 中: ${idxNotAttr.join(",")}`);
  else ok("索引中的每个角色都有属性数据");

  // 多出的文件不一定错——可能是别的实体类别（如阴阳师主角）
  if (orphanFiles.length) {
    console.log(`  ℹ️  有文件但不在索引中的 ${orphanFiles.length} 个 id，逐一归因：`);
    for (const id of orphanFiles) {
      const o = loadJson(`Data:Character/${id}.json`);
      const name = o?.name?.cn ?? "?";
      console.log(`       ${id} ${name}`);
    }
    console.log(`        → 若为「阴阳师主角」等非式神实体，属**合法类别差异**，不是错误`);
  } else {
    ok("无多余文件");
  }
}

// ── 3. schema 漂移（双向）───────────────────────────────────────────────────
console.log("\n[3] schema 漂移检查");
const schema = loadRaw("Data:Character.schema");
if (!schema) {
  error("缺少 Data:Character.schema");
} else {
  const declared = JSON.parse(schema).childrens.map((c) => c.key);
  const actual = new Set();
  for (const p of charPages) {
    const o = loadJson(p.title);
    if (o) Object.keys(o).forEach((k) => actual.add(k));
  }
  const actualList = [...actual].filter((k) => k !== "_hjschema");
  const undeclared = actualList.filter((k) => !declared.includes(k));
  const unused = declared.filter((k) => !actualList.includes(k));
  console.log(`  schema 声明 ${declared.length} 字段，记录实际出现 ${actualList.length} 字段`);
  console.log(`  schema: ${declared.join(", ")}`);
  console.log(`  实际  : ${actualList.join(", ")}`);
  if (undeclared.length) warn(`数据超出 schema（未声明）: ${undeclared.join(", ")}`);
  else ok("无未声明字段");
  if (unused.length) warn(`schema 声明但数据从未出现: ${unused.join(", ")}`);
}

// ── 4. 字段覆盖率 ───────────────────────────────────────────────────────────
console.log("\n[4] 字段覆盖率（合法缺失 vs 漏录，需人工判定）");
const counts = {};
let parsed = 0;
for (const p of charPages) {
  const o = loadJson(p.title);
  if (!o) continue;
  parsed++;
  for (const k of Object.keys(o)) counts[k] = (counts[k] || 0) + 1;
}
console.log(`  可解析 ${parsed} 条`);
for (const [k, v] of Object.entries(counts).sort((a, b) => a[1] - b[1])) {
  const pct = Math.round((v / parsed) * 100);
  console.log(`  ${k.padEnd(16)} ${String(v).padStart(4)}/${parsed}  ${String(pct).padStart(3)}%${pct < 100 ? "  ← 非全覆盖" : ""}`);
}

// ── 5. 跨源一致性（含量纲归一）──────────────────────────────────────────────
console.log("\n[5] 跨源一致性：Character.stats vs Attribute");
const statWithValues = charPages.filter((p) => {
  const o = loadJson(p.title);
  return o?.stats?.after && Object.keys(o.stats.after).length > 0;
});
console.log(`  Character 中有真实 stats 的角色: ${statWithValues.length} / ${charPages.length}`);
console.log(`  → stats 字段基本未使用，真源为 Data:Attribute.json`);

const UNIT = { crit: 100, crit_dmg: 100 }; // Character 用百分数，Attribute 用小数
const MAP = { atk: "atk", hp: "hp", def: "def", spd: "spd", crit: "cri", crit_dmg: "crid" };
let compared = 0, mismatched = 0;
for (const p of statWithValues) {
  const o = loadJson(p.title);
  const c = o.stats.after;
  const a = attr?.attributes.find((x) => x.id === p.id);
  if (!a) {
    warn(`${p.id} ${o.name?.cn} 有 stats 但 Attribute 中无对应条目，无法比对`);
    continue;
  }
  const diffs = [];
  let fields = 0;
  for (const [ck, ak] of Object.entries(MAP)) {
    const cv = c[ck]?.value;
    if (cv === undefined || cv === null) continue;
    const av = UNIT[ck] ? a[ak] * UNIT[ck] : a[ak];
    fields++;
    compared++;
    if (Math.abs(cv - av) > 1e-6) diffs.push(`${ck}: ${cv} ≠ ${av}`);
  }
  if (diffs.length) {
    mismatched++;
    error(`${p.id} ${o.name?.cn} 跨源不一致: ${diffs.join("; ")}`);
  } else if (fields === 0) {
    // 零字段比对不能判为一致——空对象/字段缺失必须显式报出，否则是假阳性
    warn(`${p.id} ${o.name?.cn} stats.after 存在但无任何可比对字段 → 无法判定一致性`);
  } else {
    ok(`${p.id} ${o.name?.cn} 跨源一致（比对 ${fields} 个字段，已做量纲归一）`);
  }
}
if (statWithValues.length > 0 && compared === 0) {
  warn("所有候选记录都不含可比对字段——跨源一致性实际上未被验证");
}
if (compared === 0) console.log("  （无可比对数据）");

// ── 汇总 ────────────────────────────────────────────────────────────────────
const errs = problems.filter((p) => p.level === "ERROR").length;
const warns = problems.length - errs;
console.log(`\n汇总：ERROR ${errs}  WARN ${warns}`);
process.exit(errs > 0 ? 1 : 0);
