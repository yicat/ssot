#!/usr/bin/env node
/**
 * huiji-to-vault.mjs —— 把灰机 wiki「Data:」命名空间的抓取导出转成一个 vault
 *
 * ## 做什么
 * 读 `.huiji/raw/*.json`（抓取缓存的原始导出）与 `.huiji/manifest.json`（每页的
 * `revid` / 远端时间戳 / 抓取时间），写进一个 vault（形态见 docs/specs/vault.spec.md）：
 *
 *   raw/式神/<名字>.md         一篇式神一页原文（JSON → markdown「原文视图」）
 *   raw/剧情/<故事名>.md        一篇剧情一页原文
 *   raw/来源/灰机wiki Data 转写说明.md   来源、转写规则、页数（本次运行生成）
 *   raw/_原始导出/*.json        抓来的 JSON **逐字节原样留存**（正文页的 sha256 指它）
 *   tables/*.csv               从整表 JSON 转出来的数据表（供 SQLite 派生索引查询）
 *
 * ## 适用范围
 * - 数据来自本仓库 `.huiji/`（灰机 wiki `yys.huijiwiki.com` 的 `Data:` 命名空间，ns=3500）
 * - vault 形态要求：两层根 `raw/` `docs/` + `tables/`，**文档必须在第 3 层**
 *   （所以分类文件夹是 `式神` / `剧情` / `来源`，来源记在 front matter，不占一层路径）
 * - 可重复运行：每次**整段重建**它自己生成的那几个目录与 CSV（见下）
 *
 * ## 什么时候不该用
 * - 它**不整理内容**：只做「原样转写 + 建表」，`docs/`（整理层）一个字都不碰——
 *   整理层是「我们认定的说法」，不能由 wiki 数据自动灌进来
 * - 它**不做增量合并**：重跑会删掉 `raw/式神`、`raw/剧情`、`raw/来源`、`raw/_原始导出`
 *   与它生成的那批 CSV，然后全部重写。手写的东西别放在这几个位置
 * - `.huiji/` 不存在（没抓过 / 换了机器）时它无从下手：先跑抓取，它自己不会联网
 * - 剧情体量很大（95k+ 行台词），单页可能几百 KB：界面打开这种页会慢，这是数据本身的问题
 *
 * ## 用法
 *   node scripts/ingest/huiji-to-vault.mjs                     # 默认 --vault projects/demo
 *   node scripts/ingest/huiji-to-vault.mjs --vault projects/x   # 换一个 vault
 *   node scripts/ingest/huiji-to-vault.mjs --only characters    # 只做某一类（调试用）
 *
 * 依赖：Node 内置模块，无第三方依赖（本机出网受限，别在这里引包）。
 */
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

/** 删除只走异步版：同步 fs.rmSync 在本机对中文目录名静默失败（见 purgeDir 的注释）。 */
const fsp = fs.promises;

// ── 参数 ──────────────────────────────────────────────────────────────
const argv = process.argv.slice(2);
const opt = {};
for (let i = 0; i < argv.length; i++) {
  const a = argv[i];
  if (!a.startsWith("--")) continue;
  const eq = a.indexOf("=");
  if (eq >= 0) opt[a.slice(2, eq)] = a.slice(eq + 1);
  else opt[a.slice(2)] = argv[++i];
}
const RAW_DIR = opt.raw ?? ".huiji/raw";
const VAULT = opt.vault ?? "projects/demo";
const MANIFEST = opt.manifest ?? path.join(path.dirname(RAW_DIR), "manifest.json");
const ONLY = opt.only ? new Set(String(opt.only).split(",")) : null;
const want = (k) => !ONLY || ONLY.has(k);

// ── 小工具 ────────────────────────────────────────────────────────────
const log = (...a) => console.log(...a);

/** Windows 文件名禁用字符 → `_`（中文名照原样留着，人还要读它）。 */
const sanitizeName = (s) => String(s ?? "").replace(/[\\/:*?"<>|]/g, "_").trim();

/** 空值判定：wiki 里 `0` 与空串是「没填」的占位符，不是事实。 */
const isBlank = (v) => v === null || v === undefined || v === "" || v === "0";

/** 数组 → `、` 连接；对象 → JSON 文本（**不猜**怎么展开）。 */
function flat(v) {
  if (v === null || v === undefined) return "";
  if (Array.isArray(v)) return v.map(flat).filter((x) => x !== "").join("、");
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

/** 数组里的对象（如 `tags: [{name}]`）取有意义的那个字段。 */
const label = (v) => (v && typeof v === "object" ? flat(v.name ?? v.title ?? v.id ?? v) : flat(v));

/** `<br>` → 换行；其余逐字不动。 */
const brk = (s) => String(s ?? "").replace(/<br\s*\/?>/gi, "\n").replace(/&nbsp;/g, " ").replace(/\r\n?/g, "\n");

/** 多行文本 → markdown 硬换行（行尾两个空格），保住原文的换行结构。 */
function prose(s) {
  const lines = brk(s).split("\n").map((l) => l.trimEnd());
  while (lines.length && lines[0] === "") lines.shift();
  while (lines.length && lines[lines.length - 1] === "") lines.pop();
  return lines.map((l) => (l === "" ? "" : l + "  ")).join("\n");
}

/** GFM 表格里 `|` 要转义，否则列会错位。 */
const cell = (s) => flat(s).replace(/\|/g, "\\|").replace(/\n+/g, " ");

/** YAML 标量：可疑的一律加双引号（JSON 字符串是合法 YAML）。 */
function yamlScalar(v) {
  const s = String(v ?? "");
  if (s !== "" && !/[:#{}\[\],&*!|>'"%@`\n]/.test(s) && !/^[\s-]/.test(s)) return s;
  return JSON.stringify(s);
}

function frontMatter(fields) {
  const lines = Object.entries(fields)
    .filter(([, v]) => v !== undefined && v !== null)
    .map(([k, v]) => (Array.isArray(v) ? `${k}: [${v.map(yamlScalar).join(", ")}]` : `${k}: ${yamlScalar(v)}`));
  return ["---", ...lines, "---"].join("\n");
}

function csvCell(v) {
  const s = v === null || v === undefined ? "" : String(v);
  return /[",\n\r]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
}
function writeCSV(file, cols, rows) {
  const body = [cols.join(","), ...rows.map((r) => cols.map((c) => csvCell(r[c])).join(","))].join("\n");
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, body + "\n", "utf8");
  return rows.length;
}

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

const sha256 = (buf) => createHash("sha256").update(buf).digest("hex");

// ── manifest：页面 → revid / 时间戳 ───────────────────────────────────
const manifest = fs.existsSync(MANIFEST) ? readJSON(MANIFEST) : { source: "yys.huijiwiki.com", pages: [] };
const SITE = `https://${manifest.source}`;
const pageByFile = new Map();
for (const p of manifest.pages ?? []) pageByFile.set(sanitizeName(p.title), p);
const pageOf = (file) => pageByFile.get(file) ?? {};
const sourceURL = (file) => {
  const p = pageOf(file);
  return p.title ? `${SITE}/wiki/${p.title}` : SITE;
};

// ── 输入文件分类 ──────────────────────────────────────────────────────
const allFiles = fs.readdirSync(RAW_DIR).filter((f) => f.endsWith(".json"));
const charFiles = allFiles.filter((f) => /^Data_Character_\d+\.json$/.test(f)).sort();
const storyFiles = allFiles.filter((f) => /^Data_Story_.+\.json$/.test(f)).sort();

// ── 生成目录：整段重建 ────────────────────────────────────────────────
const OUT_CHAR = path.join(VAULT, "raw", "式神");
const OUT_STORY = path.join(VAULT, "raw", "剧情");
const OUT_SRC = path.join(VAULT, "raw", "来源");
const OUT_ORIG = path.join(VAULT, "raw", "_原始导出");
const OUT_TABLES = path.join(VAULT, "tables");

const dirs = [];
if (want("characters")) dirs.push(OUT_CHAR);
if (want("stories")) dirs.push(OUT_STORY);
if (want("source")) dirs.push(OUT_SRC);
if (want("originals")) dirs.push(OUT_ORIG);

/**
 * 清理生成目录：**删完必须验**。
 *
 * ⚠️ 两个都踩过的坑，别改回去：
 *  1. 必须用**异步** `fs.promises.rm`。同步的 `fs.rmSync` 在本机（Node 24 / Windows）
 *     对**中文目录名**会静默失败——既不抛错也不删（实测 `.tmp/式神dir`、`raw/剧情`
 *     都原样留着）；异步版对同一路径能删掉。
 *  2. 删完一定要 `existsSync` 再确认。第一版没验，结果上一轮的旧文件（改了命名规则的那批）
 *     跟新文件混在一起，现象只是「页数变多了」，很难看出是残留。
 * 所以：异步 rm → 重试 → 改名成 ASCII 再删 → 还不行就**当场报错**，绝不静默往下走。
 */
async function purgeDir(d) {
  if (!fs.existsSync(d)) {
    log(`清理 ${d}（本来就没有）`);
    return;
  }
  for (let attempt = 1; attempt <= 3; attempt++) {
    try {
      await fsp.rm(d, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
    } catch {
      /* 判决在下面的 existsSync，这里不抢先报错 */
    }
    if (!fs.existsSync(d)) {
      log(`清理 ${d}`);
      return;
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  // 退路：改成 ASCII 名再删（实测改名后同一条路径就能删掉）。
  const alt = path.join(path.dirname(d), `ingest-trash-${Date.now()}`);
  try {
    fs.renameSync(d, alt);
  } catch {
    /* 改名也不成就直接报错 */
  }
  if (fs.existsSync(alt)) {
    try {
      await fsp.rm(alt, { recursive: true, force: true });
    } catch {
      /* 下面统一验 */
    }
  }
  if (!fs.existsSync(d) && !fs.existsSync(alt)) {
    log(`清理 ${d}（改名后删）`);
    return;
  }
  throw new Error(`清理失败：${d} / ${alt} 还在——不往下跑，免得新旧文件混在一起`);
}
for (const d of dirs) await purgeDir(d);

/** 已用文件名去重：同名加 id 后缀（不覆盖、不丢页）。 */
function namer() {
  const used = new Set();
  return (base, id) => {
    let n = sanitizeName(base) || `未命名-${id}`;
    if (used.has(n)) n = `${n}（${id}）`;
    used.add(n);
    return n;
  };
}

// ── 1. 原始导出：逐字节原样留存 ───────────────────────────────────────
let copied = 0;
let copiedBytes = 0;
if (want("originals")) {
  fs.mkdirSync(OUT_ORIG, { recursive: true });
  for (const f of fs.readdirSync(RAW_DIR)) {
    const src = path.join(RAW_DIR, f);
    if (!fs.statSync(src).isFile()) continue;
    fs.copyFileSync(src, path.join(OUT_ORIG, f));
    copied++;
    copiedBytes += fs.statSync(src).size;
  }
  log(`原始导出：${copied} 个文件（${(copiedBytes / 1048576).toFixed(1)} MB）→ ${OUT_ORIG}`);
}

/** front matter 里指向原文件的那一行 + 该文件的 sha256。 */
function provenance(file, extra) {
  const buf = fs.readFileSync(path.join(RAW_DIR, file));
  const p = pageOf(file);
  return {
    source: "灰机wiki",
    source_url: sourceURL(file),
    revid: p.revid ?? "",
    fetched_at: manifest.fetchedAt ?? "",
    sha256: sha256(buf),
    json: `raw/_原始导出/${file}`,
    ...extra,
  };
}

// ── 2. 式神原文页 ─────────────────────────────────────────────────────
const DOC_LABELS = [
  ["real_name", "真名"], ["weapon", "武器"], ["mantra", "真言"], ["trick", "特技"],
  ["character", "性格"], ["height", "身高"], ["address", "住所"], ["oneself", "自称"],
  ["hobby", "爱好"], ["favorite_food", "喜欢的食物"], ["favorite_things", "喜欢的事物"],
  ["hate_food", "讨厌的食物"], ["hate_things", "讨厌的事物"],
  ["good_at", "擅长"], ["not_good_at", "不擅长"], ["weakness", "弱点"],
  ["interest", "兴趣"], ["merit", "优点"], ["demerits", "缺点"], ["motivation", "动机"],
  ["contrast_cute", "反差萌"], ["intimat", "亲密"], ["favorite_hero", "喜欢的式神"],
  ["hate_hero", "讨厌的式神"], ["jiban_gift", "羁绊礼物"], ["author", "作者"],
];

const STAGE = { before: "觉醒前", after: "觉醒后" };

function statsSection(stats) {
  const stages = Object.entries(stats ?? {}).filter(([, v]) => v && typeof v === "object");
  if (stages.length === 0) return "";
  const lines = ["## 属性", ""];
  for (const [stage, fields] of stages) {
    const items = Object.entries(fields).filter(([, v]) => v && typeof v === "object");
    if (items.length === 0) continue;
    lines.push(`**${STAGE[stage] ?? stage}**`, "");
    for (const [k, v] of items) lines.push(`- ${k}：${v.grade ?? ""} ${v.value ?? ""}`.trimEnd(), "");
  }
  return lines.join("\n").trimEnd() + "\n";
}

function characterDoc(j) {
  const name = flat(j.name?.cn) || flat(j.name?.jp) || `式神-${j.id}`;
  const L = [];
  // tag 取索引表（Data:CharacterIndex.json）里的稀有度与功能定位；索引里没有这个 id 时退回 grade。
  // 去重是必要的：稀有度可能同时出现在两处，重复的 tag 在导航里就是两条一样的入口。
  const tags = [...new Set(
    j.__indexTags ?? ["原始层", "式神", ...(j.grade && j.grade !== "0" ? [j.grade] : [])],
  )];
  L.push(frontMatter(provenance(j.__file, { title: name, tags, status: "draft" })));
  L.push("");
  L.push("> 原始层：本文由灰机 wiki 的 Data 页**原样转写**（`<br>` 转成换行，其余逐字未改）。");
  L.push("> 原文件见 `raw/_原始导出/`（sha256 在 front matter），行尾 `^块id` 供整理层指回来。");
  L.push("");

  const info = [
    ["中文名", j.name?.cn], ["日文名", j.name?.jp],
    ["稀有度", j.grade && j.grade !== "0" ? j.grade : ""],
    ["性别", j.gender], ["实装日期", j.date], ["实装年份", j.year && j.year !== 0 ? j.year : ""],
    ["CV（中）", j.cv?.cn], ["CV（日）", j.cv?.jp],
  ].filter(([, v]) => !isBlank(v));
  if (info.length) {
    L.push("## 基本信息", "", "| 字段 | 值 |", "| --- | --- |");
    for (const [k, v] of info) L.push(`| ${k} | ${cell(v)} |`);
    L.push("");
  }

  if (!isBlank(j.introduction)) {
    L.push("## 简介", "", prose(j.introduction) + " ^简介", "");
  }

  const skills = j.skills ?? [];
  if (skills.length) {
    L.push("## 技能", "");
    for (const s of skills) {
      L.push(`### ${flat(s.name) || "（无名技能）"}（${flat(s.id)}）`, "");
      const body = !isBlank(s.description) ? s.description : s.brief;
      if (!isBlank(body)) L.push(prose(body) + ` ^技能-${flat(s.id)}`, "");
      const meta = [];
      if (!isBlank(s.cost)) meta.push(`耗火 ${s.cost}`);
      const tags = (s.tags ?? []).map(label).filter(Boolean);
      if (tags.length) meta.push(`标签：${tags.join("、")}`);
      if (!isBlank(s.brief) && !isBlank(s.description)) meta.push(`短述：${flat(s.brief)}`);
      if (s.passive) meta.push("被动");
      if (s.firstMove) meta.push("先手");
      if (meta.length) L.push(meta.join(" ｜ "), "");
      if ((s.upgrades ?? []).length) {
        L.push("**升级效果**", "");
        for (const u of s.upgrades) L.push(`- Lv.${flat(u.level)}：${flat(u.effect)}`);
        L.push("");
      }
      if ((s.tips ?? []).length) {
        L.push("**技能说明**", "");
        for (const t of s.tips) L.push(`- ${flat(t.text ?? t)}`);
        L.push("");
      }
    }
  }

  const zhuanji = j.zhuanji ?? [];
  if (zhuanji.length) {
    L.push("## 传记", "");
    for (const z of zhuanji) {
      const unlock = isBlank(z.unlock) ? "" : `（解锁：${flat(z.unlock)}）`;
      L.push(`### ${flat(z.title) || `传记${z.index}`}${unlock}`, "");
      L.push(prose(z.text) + ` ^传记-${flat(z.index)}`, "");
    }
  }

  const huijuan = j.huijuan ?? [];
  if (huijuan.length) {
    L.push("## 绘卷", "");
    for (const h of huijuan) {
      L.push(`### ${flat(h.index)}. ${flat(h.title)}`, "");
      L.push(prose(h.text) + ` ^绘卷-${flat(h.index)}`, "");
    }
  }

  const doc = j.document ?? {};
  const docLines = DOC_LABELS.filter(([k]) => !isBlank(doc[k])).map(([k, zh]) => `- ${zh}：${flat(doc[k])}`);
  const extraKeys = Object.keys(doc).filter((k) => !DOC_LABELS.some(([kk]) => kk === k) && !isBlank(doc[k]));
  for (const k of extraKeys) docLines.push(`- ${k}：${flat(doc[k])}`);
  if (docLines.length) {
    L.push("## 档案", "", ...docLines, "", "^档案", "");
  }

  const sheets = j.Voice ?? [];
  if (sheets.length) {
    L.push("## 语音", "");
    for (const sh of sheets) {
      const t = [flat(sh.type), flat(sh.sp_skin)].filter(Boolean).join(" / ");
      L.push(`### ${t || flat(sh.id)}`, "");
      for (const v of sh.voices ?? []) L.push(`- ${flat(v.act_name)}：${flat(v.txt_ch)}`);
      L.push("");
    }
  }

  const stats = statsSection(j.stats);
  if (stats) L.push(stats);

  return L.join("\n").replace(/\n{3,}/g, "\n\n").trimEnd() + "\n";
}

let charCount = 0;
if (want("characters")) {
  fs.mkdirSync(OUT_CHAR, { recursive: true });
  const indexById = new Map();
  const idxFile = path.join(RAW_DIR, "Data_CharacterIndex.json");
  if (fs.existsSync(idxFile)) {
    for (const c of readJSON(idxFile).characters ?? []) indexById.set(String(c.id), c);
  }
  const nameOf = namer();
  for (const f of charFiles) {
    const j = readJSON(path.join(RAW_DIR, f));
    j.__file = f;
    const idx = indexById.get(String(j.id));
    if (idx) j.__indexTags = ["原始层", "式神", ...(idx.rarity ? [idx.rarity] : []), ...(idx.tags ?? [])];
    const name = flat(j.name?.cn) || flat(j.name?.jp) || `式神-${j.id}`;
    fs.writeFileSync(path.join(OUT_CHAR, `${nameOf(name, j.id)}.md`), characterDoc(j), "utf8");
    charCount++;
  }
  log(`式神原文：${charCount} 页 → ${OUT_CHAR}`);
}

// ── 3. 剧情原文页 ─────────────────────────────────────────────────────
const STEP_PREFIX = { choice: "选择", video: "影像", image: "图像", effect: "演出", music: "音乐", delay: "停顿", movie: "动画" };

function stepLines(steps, indent) {
  const pad = "  ".repeat(indent);
  const out = [];
  for (const st of steps ?? []) {
    if (st.kind === "line") {
      const cls = isBlank(st.class) ? "" : `〔${flat(st.class)}〕`;
      out.push(`${pad}- **${flat(st.speaker) || "旁白"}**：${cls}${flat(st.text)}`);
    } else if (st.kind === "choice") {
      out.push(`${pad}- **【选择】**${flat(st.text)}`);
      for (const o of st.options ?? []) {
        out.push(`${pad}  - ▸ ${flat(o.index)}. ${flat(o.text)}`);
        out.push(...stepLines(o.steps, indent + 2));
      }
    } else {
      const p = STEP_PREFIX[st.kind] ?? st.kind;
      const detail = flat(st.text) || flat(st.path) || flat(st.speaker);
      out.push(`${pad}- 〔${p}〕${detail}`);
    }
  }
  return out;
}

/** 剧情名：原文 `story_name` 有 28 篇是空的，退回**第一章标题**（再空才用编号）。 */
const storyName = (entry) =>
  flat(entry.story_name) || flat(entry.chapters?.[0]?.chapter_title) || `剧情-${entry.story_id}`;

function storyDoc(entry, file) {
  const name = storyName(entry);
  const L = [];
  L.push(frontMatter(provenance(file, {
    title: name,
    tags: ["原始层", "剧情"],
    status: "draft",
  })));
  L.push("");
  L.push(`> 原始层：本文由灰机 wiki 的 Data 页**原样转写**（台词与选项逐条搬，未改写）。`);
  L.push("> 原文件见 `raw/_原始导出/`（sha256 在 front matter）。");
  L.push("");
  L.push(`## ${name}`, "");
  L.push(`- 剧情编号：\`${flat(entry.story_id)}\``);
  L.push("");
  for (const c of entry.chapters ?? []) {
    L.push(`### ${flat(c.chapter_id)}. ${flat(c.chapter_title)}`, "");
    L.push(`^章节-${flat(c.chapter_id)}`, "");
    if (!isBlank(c.chapter_desc)) L.push(prose(c.chapter_desc), "");
    if (!isBlank(c.series)) L.push(`- 线别：${flat(c.series)}`, "");
    L.push(...stepLines(c.steps, 0), "");
  }
  return L.join("\n").replace(/\n{3,}/g, "\n\n").trimEnd() + "\n";
}

let storyCount = 0;
let storySteps = 0;
if (want("stories")) {
  fs.mkdirSync(OUT_STORY, { recursive: true });
  const nameOf = namer();
  for (const f of storyFiles) {
    const j = readJSON(path.join(RAW_DIR, f));
    for (const entry of j.story ?? []) {
      const name = storyName(entry);
      fs.writeFileSync(path.join(OUT_STORY, `${nameOf(name, entry.story_id)}.md`), storyDoc(entry, f), "utf8");
      storyCount++;
      for (const c of entry.chapters ?? []) {
        const count = (function n(steps) {
          return (steps ?? []).reduce((a, s) => a + 1 + (s.options ?? []).reduce((b, o) => b + n(o.steps), 0), 0);
        })(c.steps);
        storySteps += count;
      }
    }
  }
  log(`剧情原文：${storyCount} 页（${storySteps} 条步骤）→ ${OUT_STORY}`);
}

// ── 4. 数据表（整表 JSON → CSV） ──────────────────────────────────────
const MADE_TABLES = [];

function table(file, cols, rows) {
  // 列名不许重复：派生索引建表会把重名列直接判错（`duplicate column name`）。
  // 这里当场炸掉比让索引那边静默跳过整张表好——静默跳过会让人以为这张表「本来就没有」。
  const dup = cols.filter((c, i) => cols.indexOf(c) !== i);
  if (dup.length) throw new Error(`${file} 的列名重复：${[...new Set(dup)].join("、")}`);
  const n = writeCSV(path.join(OUT_TABLES, file), cols, rows);
  MADE_TABLES.push({ file, cols, rows: n });
  log(`  表 ${file}：${n} 行（${cols.length} 列）`);
  return n;
}

if (want("tables")) {
  const charJSONs = charFiles.map((f) => {
    const j = readJSON(path.join(RAW_DIR, f));
    j.__file = f;
    return j;
  });
  const nameOfChar = new Map(charJSONs.map((j) => [String(j.id), flat(j.name?.cn) || flat(j.name?.jp)]));
  /** 名字里有 `|` 会顶坏 markdown 表；CSV 不受影响，但统一用同一套清洗。 */
  const clean = (v) => flat(v).replace(/\r?\n+/g, " ").trim();

  // 式神索引（Data:CharacterIndex.json）
  const idxPath = path.join(RAW_DIR, "Data_CharacterIndex.json");
  if (fs.existsSync(idxPath)) {
    const items = readJSON(idxPath).characters ?? [];
    const sub = [...new Set(items.flatMap((c) => Object.keys(c.extraTags ?? {})))].sort();
    const cols = ["id", "name", "pageTitle", "rarity", "year", "gender", "tags", "nickname", ...sub];
    table("式神索引.csv", cols, items.map((c) => {
      const row = {
        id: c.id, name: c.name, pageTitle: c.pageTitle, rarity: c.rarity, year: c.year,
        gender: flat(c.gender), tags: flat(c.tags), nickname: flat(c.nickname),
      };
      for (const k of sub) row[k] = clean(c.extraTags?.[k]);
      return row;
    }));
  }

  // 式神属性（Data:Attribute.json）
  const attrPath = path.join(RAW_DIR, "Data_Attribute.json");
  if (fs.existsSync(attrPath)) {
    const items = readJSON(attrPath).attributes ?? [];
    const cols = ["id", "name", "rarity", "atk", "hp", "def", "spd", "cri", "crid", "efh", "efr"];
    table("式神属性.csv", cols, items);
  }

  // 式神档案（各 Data:Character/<id>.json 的 document 块）
  // `document` 自己也带 hero_id / id / name：与外层那两列重名，会让索引建表直接失败，先剔掉。
  const docKeys = [...new Set(charJSONs.flatMap((j) => Object.keys(j.document ?? {})))]
    .filter((k) => k !== "hero_id" && k !== "hero_name" && k !== "id")
    .sort();
  table("式神档案.csv", ["hero_id", "hero_name", ...docKeys], charJSONs.map((j) => {
    const row = { hero_id: j.id, hero_name: nameOfChar.get(String(j.id)) ?? "" };
    for (const k of docKeys) row[k] = clean(j.document?.[k]);
    return row;
  }));

  // 式神技能 + 技能升级
  const skillRows = [];
  const upgradeRows = [];
  for (const j of charJSONs) {
    for (const s of j.skills ?? []) {
      skillRows.push({
        hero_id: j.id, hero_name: nameOfChar.get(String(j.id)) ?? "",
        skill_id: flat(s.id), skill_name: clean(s.name), skill_kind: clean(s.kind), skill_cost: s.cost ?? "",
        skill_brief: clean(s.brief), skill_description: clean(s.description),
        skill_tags: (s.tags ?? []).map(label).join("、"),
        skill_effectDesc: flat(s.effectDesc), skill_tips: flat(s.tips),
        passive: s.passive ?? "", first_move: s.firstMove ?? "",
      });
      for (const u of s.upgrades ?? []) {
        upgradeRows.push({
          hero_id: j.id, hero_name: nameOfChar.get(String(j.id)) ?? "",
          skill_id: flat(s.id), skill_name: clean(s.name), level: u.level, effect: clean(u.effect),
        });
      }
    }
  }
  table("式神技能.csv",
    ["hero_id", "hero_name", "skill_id", "skill_name", "skill_kind", "skill_cost", "skill_brief",
      "skill_description", "skill_tags", "skill_effectDesc", "skill_tips", "passive", "first_move"],
    skillRows);
  table("技能升级.csv", ["hero_id", "hero_name", "skill_id", "skill_name", "level", "effect"], upgradeRows);

  // 技能说明（Data:SkillTips.json）
  const tipsPath = path.join(RAW_DIR, "Data_SkillTips.json");
  if (fs.existsSync(tipsPath)) {
    const items = Object.values(readJSON(tipsPath).tips ?? {});
    table("技能说明.csv", ["order", "id", "skill_tips_name", "skill_tips_text", "skill_tips_icon"], items.map((t) => ({
      order: t.order, id: t.id, skill_tips_name: clean(t.skill_tips_name),
      skill_tips_text: clean(t.skill_tips_text), skill_tips_icon: t.skill_tips_icon ?? "",
    })));
  }

  // 增益减益（Data:SkillBuffs.json，按 id 的映射）
  const buffPath = path.join(RAW_DIR, "Data_SkillBuffs.json");
  if (fs.existsSync(buffPath)) {
    const items = Object.values(readJSON(buffPath).buff ?? {});
    table("增益减益.csv", ["id", "name", "type", "kind", "buffDesc", "icon", "iconEx"], items.map((b) => ({
      id: b.id, name: clean(b.name), type: clean(b.type), kind: clean(b.kind),
      buffDesc: clean(b.buffDesc), icon: b.icon ?? "", iconEx: b.iconEx ?? "",
    })));
  }

  // 御魂（Data:Soul.json）
  const soulPath = path.join(RAW_DIR, "Data_Soul.json");
  if (fs.existsSync(soulPath)) {
    const items = readJSON(soulPath).soul ?? [];
    table("御魂.csv", ["id", "name", "type", "Tag", "suit", "desc", "way", "effect"], items.map((s) => ({
      id: s.id, name: clean(s.name), type: s.type, Tag: clean(s.Tag), suit: clean(s.suit),
      desc: clean(s.desc), way: clean(s.way), effect: flat(s.effect),
    })));
  }

  // 皮肤（Data:Skin.json，按式神 id 的映射）
  const skinPath = path.join(RAW_DIR, "Data_Skin.json");
  if (fs.existsSync(skinPath)) {
    const rows = [];
    for (const [id, entry] of Object.entries(readJSON(skinPath))) {
      for (const s of entry.skins ?? []) {
        rows.push({
          hero_id: entry.id ?? id, hero_name: clean(entry.name), skin_id: s.skin_id,
          skin_name: clean(s.skin_name), skin_way: clean(s.skin_way), date: s.date ?? "", type: clean(s.type),
        });
      }
    }
    table("皮肤.csv", ["hero_id", "hero_name", "skin_id", "skin_name", "skin_way", "date", "type"], rows);
  }

  // 语音（各 Data:Character/<id>.json 的 Voice 块）
  const voiceRows = [];
  for (const j of charJSONs) {
    for (const sh of j.Voice ?? []) {
      for (const v of sh.voices ?? []) {
        voiceRows.push({
          hero_id: j.id, hero_name: nameOfChar.get(String(j.id)) ?? "",
          sheet_id: sh.id, sp_skin: clean(sh.sp_skin), sheet_type: clean(sh.type),
          voice_id: v.id, act_name: clean(v.act_name), txt_ch: clean(v.txt_ch),
        });
      }
    }
  }
  table("语音.csv",
    ["hero_id", "hero_name", "sheet_id", "sp_skin", "sheet_type", "voice_id", "act_name", "txt_ch"],
    voiceRows);

  // 礼物（Data:Gift.json）
  const giftPath = path.join(RAW_DIR, "Data_Gift.json");
  if (fs.existsSync(giftPath)) {
    const items = readJSON(giftPath).gift ?? [];
    table("礼物.csv", ["id", "name", "desc", "getway"], items.map((g) => ({
      id: g.id, name: clean(g.name), desc: clean(g.desc), getway: clean(g.getway),
    })));
  }

  // 月度插画（Data:Monthillustration.json，按序号的映射）
  const miPath = path.join(RAW_DIR, "Data_Monthillustration.json");
  if (fs.existsSync(miPath)) {
    const rows = [];
    for (const [i, e] of Object.entries(readJSON(miPath))) {
      if (!e || typeof e !== "object") continue;
      rows.push({
        index: i, hero_id: e.hero_id, hero_name: clean(e.hero_name), date: e.date, skin_id: e.skin_id,
        pic: e.pic, chapter_count: (e.chapters ?? []).length, manhua: e.manhua ?? "",
      });
    }
    table("月度插画.csv", ["index", "hero_id", "hero_name", "date", "skin_id", "pic", "chapter_count", "manhua"], rows);
  }

  // 剧情目录（各 Data:Story_<id>.json 的章节清单）
  const storyRows = [];
  for (const f of storyFiles) {
    const j = readJSON(path.join(RAW_DIR, f));
    for (const s of j.story ?? []) {
      for (const c of s.chapters ?? []) {
        const count = (function n(steps) {
          return (steps ?? []).reduce((a, x) => a + (x.kind === "line" ? 1 : 0) + (x.options ?? []).reduce((b, o) => b + n(o.steps), 0), 0);
        })(c.steps);
        storyRows.push({
          story_id: s.story_id, story_name: clean(s.story_name), chapter_id: c.chapter_id,
          chapter_title: clean(c.chapter_title), series: clean(c.series), lines: count,
          chapter_desc: clean(c.chapter_desc), source_json: f,
        });
      }
    }
  }
  table("剧情目录.csv",
    ["story_id", "story_name", "chapter_id", "chapter_title", "series", "lines", "chapter_desc", "source_json"],
    storyRows);
}

// ── 5. 来源与转写说明（本次运行生成，所以数字必须来自本次） ─────────────
if (want("source")) {
  fs.mkdirSync(OUT_SRC, { recursive: true });
  const tableList = MADE_TABLES.map((t) => `| \`${t.file}\` | ${t.rows} | ${t.cols.length} |`).join("\n");
  const body = `---
title: 灰机 wiki Data 导出：来源与转写说明
tags: [原始层, 来源]
status: draft
---

> 原始层：这一页**不是抓来的内容**，是本次导入自己写的来源说明——所以它没有
> \`source_url\` / \`revid\` / \`sha256\`（抓来的是它下面那些页与 \`raw/_原始导出/\`）。

## 来源

- 站点：${SITE}（灰机 wiki 的「阴阳师」分站）
- 命名空间：\`Data:\`（manifest 记 ns=${manifest.ns ?? "?"}）
- 抓取时间：${manifest.fetchedAt ?? "（manifest 缺失）"}
- 页数：${manifest.count ?? allFiles.length} 页（manifest 记录），本地文件 ${fs.readdirSync(RAW_DIR).length} 个
- 每页的 \`revid\`（远端版本）与 \`fetched_at\`（抓取时间）在该页 front matter 里

## 目录

| 位置 | 是什么 |
|---|---|
| \`raw/式神/*.md\` | 一篇式神一页原文（${charCount || "本次未生成"} 页） |
| \`raw/剧情/*.md\` | 一篇剧情一页原文（${storyCount || "本次未生成"} 页） |
| \`raw/_原始导出/*.json\` | 抓来的 JSON **逐字节原样留存**（非 .md，不出现在文件树里） |

## 转写规则（\`.json\` → 这里的 \`.md\`）

1. 一页原文 ↔ 一个文件：\`Data:Character/604.json\` → \`raw/式神/不相狐禅.md\`
2. \`<br>\` 转成换行（markdown 行尾两个空格 = 硬换行）；**其余逐字不改**
3. JSON 的键渲染成小标题与列表，**没有增删任何事实**
4. 关键段落行尾带 \`^块id\`（\`^简介\` / \`^技能-<技能id>\` / \`^传记-<序号>\` / \`^绘卷-<序号>\` / \`^档案\` / \`^章节-<章节id>\`）：
   整理层用 \`[[raw/式神/不相狐禅#^技能-604_03]]\` 指回来，这就是**主张级溯源**
5. \`document\` 里值为 \`0\` 或空串的字段当占位符跳过（wiki 上没填）
6. 页面状态一律 \`draft\`：原文**未经核验**——「默认一切未核验，但必须可见」
7. 分类落在路径上（\`式神\` / \`剧情\` / \`来源\`），**来源记在 front matter**——
   界面文件树最多三层（layer + 分类文件夹 + 文档），再套一层 \`灰机wiki/\` 就超层了
8. 剧名：原文 \`story_name\` 有 28 篇是空的，页名退回**第一章标题**（表 \`剧情目录.csv\` 里
   仍按原文留空，看 \`chapter_title\`）

## 数据表（tables/，来自整表 JSON）

| 表 | 行 | 列 |
|---|---|---|
${tableList || "| （本次未生成） | | |"}

字段含义与「什么时候不该用它」见 \`tables/README.md\`。
`;
  fs.writeFileSync(path.join(OUT_SRC, "灰机wiki Data 转写说明.md"), body, "utf8");
  log(`来源说明 → ${path.join(OUT_SRC, "灰机wiki Data 转写说明.md")}`);
}

log(`\n完成：vault = ${VAULT}`);
