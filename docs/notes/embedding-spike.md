# 本地中文 embedding 实测：bge-small-zh + ONNX Runtime（免 cgo）

日期：2026-09-17。**结论：可行，已验证；但按简化后的方案暂不引入，只留端口。**

为什么暂不引入：`vault.spec.md` 的召回靠**双链 + 标签 + SQLite 全文检索**，
核验链本身不依赖向量。这份记录是为了将来真要做时**不用重新踩一遍**。

## 一、选型与体积（实测）

| 部件 | 用的是 | 体积 |
|---|---|---|
| 模型 | `Xenova/bge-small-zh-v1.5` 的 `onnx/model_quantized.onnx`（int8） | **22.9 MB** |
| 词表 | 同仓库 `vocab.txt`（**不需要** `tokenizer.json`，见坑 2） | 0.1 MB |
| 运行时 | 官方 `onnxruntime.dll`（win-x64 CPU v1.30.0；整包 82.6MB，**只要里面的这一个文件**） | **15.7 MB** |
| 合计 | | **约 39 MB** |

模型规格（`config.json`）：512 维 / 4 层 / vocab 21128 / 最长 512 token。
许可：底座 BAAI 是 **MIT**；那个 ONNX 导出仓库自己**没声明 license**，重分发要注明来源。

## 二、实测数字

```
推理：30 次共 24ms，平均 0.8 ms/条（含分词）
会话创建（加载 22.9MB 模型）：63ms
分词：与 HF 的 JS 实现逐条一致
向量：cos(Go, transformers.js 参照) = 1.000000    maxAbsDiff = 0.000e+00   ← 逐位一致
分离度：sim(相似对)=0.9407  sim(不相似)=0.3373（两边完全相同）
```

管道细节：

- 模型输入要三个：`input_ids` / `attention_mask` / **`token_type_ids`**；输出 `last_hidden_state`。
- **池化用 CLS + L2 归一**（分离度优于 mean：0.9407 vs 0.9208）。
- 分词示例：`暴击伤害提高20%` → `[101, 3274, 1140, 839, 2154, 2990, 7770, 8113, 110, 102]`。

## 三、三个坑（重踩一次要花很久，所以写下来）

1. **`shota3506/onnxruntime-purego` 在 Windows 上根本编译不过**。
   purego 在 Windows **故意不提供 `Dlopen`**——`dlfcn.go:38-39` 明说要改用
   `golang.org/x/sys/windows.LoadLibrary`；`Dlopen`/`RTLD_*` 只在 `dlfcn_{android,darwin,linux,freebsd}.go` 里。
   那个库是按 Linux/macOS 写的。
   → 用 **`GetcharZp/onnxruntime_purego`**（MIT）：它有 `dll_windows.go`（`//go:build windows` + `syscall.LoadLibrary`）
   与 `dll_unix.go` 的平台拆分，本机实测编译并跑通。
2. **`sugarme/tokenizer` 的 `NewTokenizerFromFile` 是空壳**：
   `tokenizer.go:716-719` 只有 `// TODO: implement` + `return`，**永远返回 nil**。
   照文档用它只会拿到 nil 然后一脸懵。
   → 用它真正实现了的部件自己拼 BERT 分词器：
   `wordpiece.NewWordPieceFromFile(vocab, "[UNK]", 100)`（默认就是 HF 的 `##` / 100）
   + `normalizer.NewBertNormalizer(true, true, true, true)`（cleanText / lowercase / handleChineseChars / stripAccents）
   + `pretokenizer.NewBertPreTokenizer()` + `decoder.NewWordPieceDecoder("##", true)`。
3. **依赖版本会被缓存带偏**：`go mod tidy` 选了本机缓存里的 `purego v0.9.0`，
   而绑定用的是新版 API，编译报 `undefined: purego.Dlopen`。
   要显式 `go get github.com/ebitengine/purego@latest`。

**版本配对**：绑定的 README 写明按 ONNX Runtime **1.24.1** 头文件生成、并给了对应表。
我实测**用 1.30.0 的 DLL 也能跑通**（ORT 的 API 结构是追加式的），
但正式落地**要钉住配对版本**，别靠「大概兼容」。

## 四、环境前提（一并实测过）

- 本机**没有 `gcc`** → 基于 cgo 的方案（`yalue/onnxruntime_go`、`hugot`、`go-duckdb`）直接排除。
- `GOPROXY=direct` **能**从 GitHub 拉 Go 模块（`purego` / `sugarme/tokenizer` / `onnxruntime_purego` 都下来了）。
  AGENTS.md 里「包管理器源不可达」对 **GitHub 直连**这条不成立。
- npm 走 npmmirror **能**装包（`@huggingface/transformers` 装成功了）——
  所以**能用 transformers.js 当参照实现对拍**，这比「看着像对」靠谱得多。

## 五、复现步骤

验证代码在仓库外的临时目录（**没进仓库**，因为当时还没有对应的 spec）：

```
%TEMP%\embed-spike\        Go 程序 + model.onnx + onnxruntime.dll + vocab.txt + ref2.json
%TEMP%\tfjs-probe\ref.mjs  transformers.js 参照（生成 ref.json）
```

```powershell
# 参照（JS 侧，逐条、不 padding——padding 会造出假差异，见下）
cd $env:TEMP\tfjs-probe; node ref2.mjs
# Go 侧
cd $env:TEMP\embed-spike\go
$env:SPIKE_DIR = "$env:TEMP\embed-spike"; $env:GOPROXY="direct"; $env:CGO_ENABLED="0"
go run .
```

⚠️ **对拍时参照必须逐条跑**：我第一版参照把三句批处理 + padding 一起跑，
结果 cos 只有 0.9933，看着像"Go 侧算错了"；改逐条后是 **1.000000 / maxAbsDiff 0**。
**差异全在参照那一侧。**

## 六、bge-m3 对拍与「B/C 两套配置」的召回实验（2026-09-17 补）

起因：`derived.spec.md` 要按 LightRAG 的块大小（2000 token）选嵌入模型，需要**量化**而不是拍脑袋。

**性能（本机 CPU，transformers.js `@huggingface/transformers` 4.3.0 + onnxruntime-node，
与 Go 绑定同源——见 §二 的逐位一致结论）**：

| | bge-small-zh-v1.5（q8） | bge-m3（q8） |
|---|---|---|
| 体积 | 22.9 MB | **542 MB**（HF API 实测；fp16 1081 MB、q4 1190 MB） |
| 维度 | 512 | 1024 |
| 加载 | 3.9 s | **30.2 s** |
| RSS 增量 | +80 MB | **+910 MB** |
| 查询（≈15 字） | 1.2 ms | 14.2 ms |
| 512 token 块 | 6.0 ms | 59.1 ms |
| 2000 token 块 | 12.7 ms | **481.9 ms** |

**召回实验**（200 篇文档、302 条查询；查询取文档原句的首/中/尾各一句；
池子 = 各自切法的块；ground truth = 含该句的块，80.1% 的查询能精确命中该定义）：

| 配置 | 块数 | R@1 | R@5 | MRR | 命中块 | 嵌入耗时 |
|---|---|---|---|---|---|---|
| B：512 块 + bge-small-zh | 10,243 | 65.9% | 77.8% | 0.698 | **201 字符** | **1.3 min** |
| C：2000 块 + bge-m3 | 5,849 | 69.2% | 78.1% | 0.727 | 345 字符 | 24.0 min |

配对（同查询）：R@1 只有 B 对 33 / 只有 C 对 43 → **McNemar p≈0.30 不显著**；
R@5 27 / 28 → **打平**。分长度看：head/mid C 明显好（+14pp/+8pp），**tail B 明显好（+12pp）**。

**结论**：C 的优势**没有证据**；C 命中块更大（1.7×）且把长文档尾部内容稀释了。
所以**抽取用 2000 块（喂 LLM）、嵌入用 512 子块（便宜、命中细）**——
`derived.spec.md` §九.1 记了这个决定与数据。

**复现**（脚本在 `%TEMP%` 里，没进仓库；要再跑照这个来）：
`%TEMP%\tfjs-probe\bench.mjs`（性能对拍，`node bench.mjs <模型名>`）、
`%TEMP%\tfjs-probe\recall.mjs --docs 200`（召回对比；它 import 仓库里的
`scripts/check/chunk-sizing.mjs` 复用同一套切块逻辑）。

⚠️ **这份实验不能证明的事**：它**没测抽取质量**（512 上下文会不会把实体/关系抽碎）——
那要真调 LLM 跑一批，花额度。别拿召回数字去替抽取的结论。

## 七、什么时候不该用这份记录

- **先问要不要向量**：双链 + 标签 + SQLite 全文检索够用，就别背这 39MB。
- 体积敏感的分发（小安装包）不适合随包内置 39MB；改「首次运行下载」要另写校验与失败提示。
- 只要 Windows 能跑：`onnxruntime.dll` 是 Windows 专用的，跨平台要重新解决加载。
- 别把它当成「语义正确」的保证：它只做召回，**核验仍然必须是人**。
