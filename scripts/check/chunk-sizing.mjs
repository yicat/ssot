/**
 * chunk-sizing.mjs —— 量「按不同目标大小切块，我们自己的文档会长成什么样」。
 *
 * 做什么：只读地扫一个 vault 的 markdown（`docs/` + `raw/`），按
 *   「标题 → 段落 → 句子」的次序合并成目标 token 大小的块（LightRAG 的段落语义切块思路），
 *   打印块数、块大小分布、以及「嵌入上限截断会丢掉多少」。
 *
 * 用途（这份脚本存在的理由）：决定「切块目标大小」与「嵌入模型上下文上限」这两个数
 *   应该是多少。定性争论没用——这里的数字才是依据（见 docs/specs/derived.spec.md §九.1）。
 *
 * token 估算法（写清楚，别当成真值）：
 *   - 汉字：1 字 ≈ 1 token（BERT WordPiece 对常用汉字基本单字成词）
 *   - 拉丁字母/数字串：4 字符 ≈ 1 token
 *   - 其它可见字符（标点等）：1 个 ≈ 1 token
 *   这是**估算**，实测参照：`暴击伤害提高20%`（9 字符）在 bge 词表下是 10 token（见
 *   docs/notes/embedding-spike.md §二）。要精确就得上真分词器，那是 spike 的活。
 *
 * 什么时候不该用：
 *   - 不要拿它当断言（不是测试）；它只报数，判定写在 spec 里。
 *   - 它不验召回质量——那是「同一批查询、两种配置，人工看结果」的活。
 *
 * 用法：node scripts/check/chunk-sizing.mjs --vault projects/demo --target 2000
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const args = process.argv.slice(2);
const argOf = (name, def) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 && args[i + 1] ? args[i + 1] : def;
};
const VAULT = resolve(argOf("vault", "projects/demo"));
const TARGET = Number(argOf("target", "2000"));
const OVERLAP = Number(argOf("overlap", "100"));
const TRUNC = Number(argOf("trunc", "512")); // 嵌入模型上限，用来算截断损失

/** 估 token 数（规则见文件头）。 */
export function estTokens(text) {
  let tokens = 0;
  let latinRun = 0;
  const flushLatin = () => {
    if (latinRun > 0) {
      tokens += Math.ceil(latinRun / 4);
      latinRun = 0;
    }
  };
  for (const ch of text) {
    const code = ch.codePointAt(0);
    const isHan = (code >= 0x3400 && code <= 0x9fff) || (code >= 0xf900 && code <= 0xfaff);
    const isLatin = /[A-Za-z0-9]/.test(ch);
    if (isHan) {
      flushLatin();
      tokens += 1;
    } else if (isLatin) {
      latinRun += 1;
    } else if (/\s/.test(ch)) {
      flushLatin();
    } else {
      flushLatin();
      tokens += 1;
    }
  }
  flushLatin();
  return tokens;
}

/** 把一个文件切成「段」：标题单独成段，其余按空行分段。 */
function paragraphs(md) {
  const out = [];
  let buf = [];
  const push = () => {
    const t = buf.join("\n").trim();
    if (t) out.push(t);
    buf = [];
  };
  for (const line of md.split(/\r?\n/)) {
    if (/^#{1,6}\s/.test(line)) {
      push();
      out.push(line.trim()); // 标题自己一段，切块时优先在这里断开
      continue;
    }
    if (line.trim() === "") push();
    else buf.push(line);
  }
  push();
  return out;
}

/** 超长段落按句子切。 */
function splitLong(text, limit) {
  const parts = text.split(/(?<=[。！？；.!?;])/);
  const out = [];
  let cur = "";
  for (const p of parts) {
    if (estTokens(cur + p) > limit && cur) {
      out.push(cur);
      cur = p;
    } else {
      cur += p;
    }
  }
  if (cur.trim()) out.push(cur);
  return out;
}

/** 按目标大小合并段落成块；标题处优先断开。 */
export function chunkMarkdown(md, target) {
  const paras = paragraphs(md).flatMap((p) => (estTokens(p) > target ? splitLong(p, target) : [p]));
  const chunks = [];
  let cur = [];
  let curTok = 0;
  const flush = () => {
    if (cur.length) chunks.push(cur.join("\n\n"));
    cur = [];
    curTok = 0;
  };
  for (const p of paras) {
    const t = estTokens(p);
    const isHeading = /^#{1,6}\s/.test(p);
    if (cur.length && (curTok + t > target || isHeading)) flush();
    cur.push(p);
    curTok += t;
  }
  flush();
  return chunks;
}

function walk(dir, out = []) {
  let entries;
  try {
    entries = readdirSync(dir);
  } catch {
    return out;
  }
  for (const e of entries) {
    if (e.startsWith(".")) continue;
    const p = join(dir, e);
    const st = statSync(p);
    if (st.isDirectory()) walk(p, out);
    else if (e.endsWith(".md")) out.push(p);
  }
  return out;
}

export function walkMarkdown(vault) {
  return [...walk(join(vault, "docs")), ...walk(join(vault, "raw"))];
}

// 被 import（比如召回对比实验）时只导出函数，不跑统计。
const isMain = process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMain) {
const files = walkMarkdown(VAULT);
if (files.length === 0) {
  console.error(`没找到 markdown：${VAULT}（用 --vault 指定）`);
  process.exit(1);
}

const sizes = [];
let totalChars = 0;
let lostChars = 0;
let overLimit = 0;
let headings = 0;

for (const f of files) {
  const md = readFileSync(f, "utf8");
  headings += (md.match(/^#{1,6}\s/gm) ?? []).length;
  for (const c of chunkMarkdown(md, TARGET)) {
    const t = estTokens(c);
    sizes.push(t);
    totalChars += c.length;
    if (t > TRUNC) {
      overLimit++;
      // 截断会丢掉的字符：按 token 比例估（同一段文字里，token 与字符大致同尺）
      lostChars += Math.round(c.length * (1 - TRUNC / t));
    }
  }
}

sizes.sort((a, b) => a - b);
const q = (p) => sizes[Math.min(sizes.length - 1, Math.floor(sizes.length * p))] ?? 0;
const sum = sizes.reduce((a, b) => a + b, 0);

console.log(`vault        ${VAULT}`);
console.log(`文档          ${files.length} 篇 md（标题行 ${headings} 个）`);
console.log(`切块目标      ${TARGET} token（重叠 ${OVERLAP} 未计入块大小）`);
console.log(`块数          ${sizes.length}`);
console.log(`块大小        min ${sizes[0]} / median ${q(0.5)} / p90 ${q(0.9)} / p99 ${q(0.99)} / max ${sizes[sizes.length - 1]}`);
console.log(`平均          ${Math.round(sum / sizes.length)} token`);
console.log(
  `> ${TRUNC} token 的块  ${overLimit}（${((overLimit / sizes.length) * 100).toFixed(1)}%）` +
    ` → 若按 ${TRUNC} 截断嵌入，丢掉约 ${((lostChars / totalChars) * 100).toFixed(1)}% 的字符`,
);
console.log(`向量内存      ${(sizes.length * 512 * 4 / 1048576).toFixed(1)} MB @512 维  ／  ${(sizes.length * 1024 * 4 / 1048576).toFixed(1)} MB @1024 维`);
}



