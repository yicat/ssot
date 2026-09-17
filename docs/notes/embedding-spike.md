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

**补偿实验（同一批 302 条查询，干净池子 8,667 块）**——测「B 的 head 劣势能不能用结构性手段补」：

| 策略 | R@1 | R@5 | head R@1 | mid R@1 | tail R@1 | 正确文档进前 3 |
|---|---|---|---|---|---|---|
| chunk-only（B 基线） | **61.6%** | **79.8%** | 59.8% | 61.6% | **63.4%** | — |
| chunk + 文档向量 α=0.5 | 59.9% | 78.5% | 57.8% | 61.6% | 60.4% | — |
| chunk + 文档向量 α=1.0 | 55.3% | 72.5% | 54.9% | 58.6% | 52.5% | — |
| 两段式（文档向量选前 3 篇再排块） | **19.5%** | 23.5% | 15.7% | 23.2% | 19.8% | **24.8%** |

**负结果**：文档向量（标题+小标题+第一段，截 450 字）当路由器只有 24.8% 命中率，
融合是负收益、两段式直接崩。所以「整篇向量」那条从 spec 撤掉了。

**标题/标签字面加权**（不花 LLM，另一条更便宜的补偿）：

| 查询组 | 只用向量 β=0 | β=0.05 | β=0.1 | β=0.2 |
|---|---|---|---|---|
| 正文句查询（n=302）R@1 / R@5 | 61.6% / 79.8% | 61.6% / 79.8% | 61.6% / 79.8% | 61.6% / 79.8% |
| **标题式查询（n=102，"X 是什么？"）R@1** | **21.6%** | 24.5% | **26.5%** | 26.5% |
| 同上 R@5 | 54.9% | 58.8% | 60.8% | **62.7%** |

⚠️ **最要紧的一条**：纯向量在**实体名式短查询**上 R@1 只有 21.6%（真人最常问的形状），
正文句查询那 59.8% 掩盖了它 → **FTS + 向量的混合检索是必需项**。
字面加权对这类查询单调有效（β 取 0.1–0.2），对正文句查询无害 → 可常开。

剩下能补 B 的只有**图那条路**（查询关键词抽取 + 实体关系图），**未实测**。

**结论**：C 的优势**没有证据**；C 命中块更大（1.7×）且把长文档尾部内容稀释了。
所以**抽取用 2000 块（喂 LLM）、嵌入用 512 子块（便宜、命中细）**——
`derived.spec.md` §九.1 记了这个决定与数据。

**复现**（脚本在 `%TEMP%` 里，没进仓库；要再跑照这个来）：
`%TEMP%\tfjs-probe\bench.mjs`（性能对拍，`node bench.mjs <模型名>`）、
`%TEMP%\tfjs-probe\recall.mjs --docs 200`（召回对比；它 import 仓库里的
`scripts/check/chunk-sizing.mjs` 复用同一套切块逻辑）。

⚠️ **这份实验不能证明的事**：它**没测抽取质量**（512 上下文会不会把实体/关系抽碎）——
那要真调 LLM 跑一批，花额度。别拿召回数字去替抽取的结论。

### 六.4 切块口径：spike 脚本 vs 真正跑的实现（2026-09-18 对拍）

上面所有块级数字都出自 `scripts/check/chunk-sizing.mjs`（JS）。**真正跑的是 Go**
（`internal/domain/vault/chunk.go` → `vaultindex`）。同一份 vault 上两者**不等数**：

| `projects/demo`（410 篇 md） | JS 脚本 | Go 实现 | 差 |
|---|---|---|---|
| 512 口径块数 | 13,178 | **12,512** | −5.0% |
| 2000 口径块数 | 7,501 | **7,147** | −4.7% |

对拍方式：两侧逐篇 dump 两套块数（Go 侧临时测试，用完删掉），按路径比对——
410 篇里 210 篇不一致，**差值最大的一批集中在 `raw/剧情/*.md`**（对话体长文档，
单篇 −15 块），其余 200 篇完全一致。

**原因**（定位到单篇看出来的，`raw/剧情/终焉降临.md`，512 口径 235 vs 220）：
JS 先把超长段落**在段落内部**预打包成 ~512 的片段，再拿片段去合并 —— 标题（17 token）
+ 片段（504）> 512，于是标题单独成块、片段独占一块；
Go 把句子直接按贪心装进当前块 —— 17 + 479 = 496 塞得下，于是合成一块。
往后两边块的尺寸逐一对齐（493 / 511 / 488 …），只差这个错位。**两边都不丢内容**，
JS 只是把块在段落边界处装得更松。

**影响**：少 5% 不改变「512 与 2000 召回无显著差异」的结论（差的是打包顺序，
不是块大小也不是模型）；但**引用块级数字时必须写清是哪套口径**。
另外 JS 脚本把 front matter 当正文读（`终焉降临` 的 front matter 单独成了 124 token
的段落并并进首块），Go 会剥掉 —— 总数不受影响，但 spike 里每篇首个块混了 YAML，实现里没有。

**顺带实测**（Go，`bin/ssot-cli.exe vault index -root projects/demo`）：
全量重建（410 篇文档 + 13 张表 + 两套块）**127 秒**，产出 12,512 + 7,147 行块。

### 六.5 纯 Go 跑 ONNX Runtime：可行性实测（2026-09-18，P1 的地基）

P1 整条路压在一个假设上：**本机没有 gcc，所以不能 cgo 链接 ORT**，只能走 purego 加载 DLL。
已实测（探针 `scripts/check/onnx-probe/main.go`）：

```
go run ./scripts/check/onnx-probe <onnxruntime.dll> <model_quantized.onnx>
  ORT 版本 1.30.0，加载耗时 57ms
  会话建好，耗时 56ms
  推理完成，耗时 1ms，输出 1 个
    输出 last_hidden_state：形状 [1 8 512]，共 4096 个数
```

结论：**这条路通**。依赖 `github.com/getcharzp/onnxruntime_purego v1.24.0`
（+ `ebitengine/purego v0.9.0`、`up-zero/gotool`）已在本机模块缓存里，`GOPROXY=off` 也能装上——
离线可复现。

本机现有文件（都是 spike 时 transformers.js 下的，在临时目录，**没进仓库**）：

| 文件 | 体积 | 路径 |
|---|---|---|
| `onnxruntime.dll`（x64，napi-v6） | **27.4 MB** | `%TEMP%\tfjs-probe\node_modules\onnxruntime-node\bin\napi-v6\win32\x64\` |
| `model_quantized.onnx`（bge-small-zh-v1.5 int8） | 22.9 MB | `…\@huggingface\transformers\.cache\Xenova\bge-small-zh-v1.5\onnx\` |
| `tokenizer.json`（同模型） | 0.4 MB | `…\bge-small-zh-v1.5\` |

⚠️ **更正一处旧数字**：本文 §一/§六 里 ORT 写作「15.7 MB」，那是下载包的体积；
本机真正加载的这个 DLL 是 **27.4 MB**。算分发体积时按 27.4 MB 算。

**还差的两块**（P1 的活，见 `docs/OPEN.md` #15/#16）：
1. **分词器要自己写**——`tokenizer.json` 是 BERT WordPiece（中文按字切），Go 侧没有现成依赖，
   得照文件里的 normalizer/pre-tokenizer/WordPiece 实现，再跟 transformers.js 逐条对拍。
2. **模型与 DLL 怎么分发**没定（现在只在本机临时目录里）。

⚠️ 这个绑定**没导出「取输入名」的方法**（`getInputName` 是小写），
所以 `input_ids` / `attention_mask` / `token_type_ids` 是写死的——BGE 系列都是 BERT 结构，
名字固定，但换模型时要先确认。

## 七、什么时候不该用这份记录

- **先问要不要向量**：双链 + 标签 + SQLite 全文检索够用，就别背这 39MB。
- 体积敏感的分发（小安装包）不适合随包内置 39MB；改「首次运行下载」要另写校验与失败提示。
- 只要 Windows 能跑：`onnxruntime.dll` 是 Windows 专用的，跨平台要重新解决加载。
- 别把它当成「语义正确」的保证：它只做召回，**核验仍然必须是人**。


