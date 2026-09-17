/**
 * embed-ref.mjs —— 生成**参照数据**：让 transformers.js 吐 token id 与句向量，
 * 供 Go 侧（`internal/infrastructure/vembed`）逐条对拍。
 *
 * 做什么：
 *   1. 用 transformers.js 的 tokenizer（WordPiece）分词 → 写 `ids` / `mask`；
 *   2. 用 onnxruntime-node 跑同一个模型（int8）→ CLS 池化 + L2 归一 → 写 `vector`；
 *   3. 结果写进 `internal/infrastructure/vembed/testdata/parity.json`（**进仓库**，是基准）。
 *
 * 为什么要有它：P1 的验收标准是「与 transformers.js 逐位对拍」（`embedding-spike.md` §六）。
 *   参照必须由**另一套实现**产生——拿 Go 自己的输出当基准等于没验。
 *
 * ⚠️ 什么时候不该用：
 *   - 别拿它当性能基准（单条、没预热，量级参考而已；性能看 `embedding-spike.md` §六）。
 *   - **transformers.js 不在仓库依赖里**（只有 spike 环境有），所以这个脚本要**拷到那个目录里跑**：
 *       $probe = "$env:TEMP\tfjs-probe"; Copy-Item scripts\check\embed-ref.mjs $probe\
 *       node $probe\embed-ref.mjs --dir "$env:TEMP\embed-spike" --out <仓库>\internal\infrastructure\vembed\testdata\parity.json
 *     模型目录里要有：`model.onnx`、`tokenizer.json`、`config.json`（`embed-spike` 就是这样一个目录）。
 *   - 只在**换模型 / 换分词器 / 改池化方式**时重跑；平时它是冻结的基准，重跑等于换考卷。
 *
 * 语料（PARITY_TEXTS）说明：这里刻意混了中文、中英混排、全角标点、制表符与全角空格、
 *   Markdown 记号、非 BMP 生僻字、以及一条超 512 token 的长文本（验截断）。
 */
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import * as ort from "onnxruntime-node";
import { AutoTokenizer, env } from "@huggingface/transformers";

const args = process.argv.slice(2);
const argOf = (name, def) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 && args[i + 1] ? args[i + 1] : def;
};
const DIR = resolve(argOf("dir", process.env.TEMP + "\\embed-spike"));
const OUT = resolve(argOf("out", "internal/infrastructure/vembed/testdata/parity.json"));

/** 对拍语料：每条都要能说清「它在测什么」。 */
const PARITY_TEXTS = [
  { text: "暴击伤害提高20%", why: "spike 的参照句：中文 + 数字 + 百分号" },
  { text: "暴击伤害增加 20%", why: "与上一条只差一个字与一个空格，向量该很近" },
  { text: "今天天气不错", why: "与伤害无关的短句" },
  { text: "最终伤害 = 攻击 × 系数", why: "空格 + ASCII 标点（= × 是符号类，不是标点类）" },
  { text: "茨木童子的三技能「罗生门」造成 263% 伤害。", why: "中文标点（「」）与句号" },
  { text: "BGE-small-zh-v1.5 模型", why: "拉丁字母/数字/连字符/点：测 WordPiece 子词切分" },
  { text: "　全角空格　与制表符\t混排", why: "U+3000 与制表符：测 normalize 的空白归一" },
  { text: "a     b", why: "连续空格：预分词按空白切开" },
  { text: "## 标题不是井号", why: "Markdown 记号：井号是标点，该被单独切开" },
  { text: "**加粗** 与 `代码` 混排", why: "星号与反引号（ASCII 标点）" },
  { text: "emoji 😀 与生僻字：𠮷 龘", why: "非 BMP 字符与生僻汉字（可能落 [UNK]）" },
  { text: "伤害计算规则说明。".repeat(60), why: "超 512 token：测截断两边一致" },
];

const main = async () => {
  env.allowRemoteModels = false;
  env.localModelPath = dirname(DIR); // 目录名即模型名
  const modelName = DIR.split(/[\\/]/).filter(Boolean).pop();
  const tok = await AutoTokenizer.from_pretrained(modelName);

  const sess = await ort.InferenceSession.create(resolve(DIR, "model.onnx"));
  const outName = sess.outputNames[0];
  const toNum = (x) => JSON.parse(JSON.stringify(x, (k, v) => (typeof v === "bigint" ? Number(v) : v)));
  const norm = (v) => {
    const s = Math.sqrt(v.reduce((a, x) => a + x * x, 0));
    return v.map((x) => x / s);
  };

  const cases = [];
  for (const { text, why } of PARITY_TEXTS) {
    // 逐条跑、不 padding：与 Go 侧「一条一条喂」的路径一致，避免 padding 造出假差异。
    const enc = tok(text, { padding: false, truncation: true, max_length: 512 });
    const ids = toNum(enc.input_ids.tolist())[0];
    const mask = toNum(enc.attention_mask.tolist())[0];
    const shape = [1, ids.length];
    const feeds = {
      input_ids: new ort.Tensor("int64", BigInt64Array.from(ids.map(BigInt)), shape),
      attention_mask: new ort.Tensor("int64", BigInt64Array.from(mask.map(BigInt)), shape),
    };
    if (sess.inputNames.includes("token_type_ids")) {
      feeds.token_type_ids = new ort.Tensor("int64", new BigInt64Array(ids.length), shape);
    }
    const out = await sess.run(feeds);
    const hidden = out[outName];
    const dim = hidden.dims[2];
    const cls = norm(Array.from(hidden.data.slice(0, dim))); // CLS = 第一个 token
    cases.push({
      text,
      why,
      ids,
      mask,
      vector: cls.map((x) => Number(x.toFixed(7))),
      dim,
    });
    console.log(
      `  ${ids.length} id / ${dim} dim  ${JSON.stringify(text.slice(0, 18))}${text.length > 18 ? "…" : ""}`,
    );
  }

  const dim = cases[0].dim;
  const payload = {
    model: `${modelName}（int8，${dim} 维，CLS 池化 + L2 归一）`,
    generatedBy:
      "scripts/check/embed-ref.mjs（transformers.js + onnxruntime-node）；重跑等于换考卷，别随便跑",
    inputs: sess.inputNames,
    outputs: sess.outputNames,
    cases,
  };
  mkdirSync(dirname(OUT), { recursive: true });
  writeFileSync(OUT, JSON.stringify(payload));
  console.log(`写了 ${cases.length} 条 → ${OUT}`);
  console.log(`模型输入 ${sess.inputNames.join("/")}，输出 ${sess.outputNames.join("/")}，维度 ${dim}`);
};

main().catch((e) => {
  console.error("生成参照失败：", e);
  process.exit(1);
});
