# 0014 后端工具集再收紧：连只读的 `glob`/`grep` 也不给

状态：已定（**推翻 0006 里「保留 `tool-fs-search`」那一条**；0006 的「写收口」原则不变）
关联：`docs/specs/dsh.spec.md`「后端工具集必须收紧」、`docs/adr/0003-gate-in-capability-layer.md`、
`docs/adr/0006-toolset-read-open-write-gated.md`、`internal/infrastructure/appconfig`

## 背景

0006 定了「读放开、写收口」，并把 `tool-fs-search`（`glob` / `grep`）划进「读」保留下来，
理由是它**纯只读**——走打包的 ripgrep，一个字节都写不了。

实际用起来发现：这个划分只回答了「**能不能写**」，没回答「**读到哪、读什么**」。
一个只读工具照样能把我们要守的两样东西一起绕过去，于是留下一个洞。

## 决策

`tool-fs-search` 与 `tool-fs`、`tool-str-replace-editor` 一起**禁用**。
agent 的**读**全部由能力层提供：`file_read`（限 vault 内）、`vault_list`（有哪些文档/表）、
`vault_search`（检索）、`doc_read`、`link_*`、`table_*`。

## 为什么

**1）它没有 vault 边界。** 插件 `@deepseek-ai/dsh-tool-fs-search` 唯一的路径规整函数是
`toWorkdirRelative`，实测源码行为就是：

```js
function toWorkdirRelative(path, workdir) {
	if (!isAbsolute(path)) return path;
	const rel = relative(workdir, path);
	if (rel.length === 0) return ".";
	if (rel === ".." || rel.startsWith(`..${sep}`)) return path;   // ← 在 workdir 外：原样放行
	return rel;
}
```

也就是说「workdir 内转相对路径好看一点」是它唯一的约束；**workdir 外的绝对路径照样搜**。
它的 `Config` 里全是输出与结果上限（`rawOutputMaxBytes`、`searchMetaMaxBytes`、结果条数这类），
**没有任何 root / workdir 限制项**——想收也收不了。

**2）它会绕开我们的检索。** 「哪篇文档说过 X」正是 `vault_search`（分块 + 向量 + 图）要回答的问题。
留着 `grep`，agent 必然改用关键词匹配：

- **收录范围（`derived.spec.md` §4）白定**：范围之外的文件照样被 grep 出来，
  被当成事实写进整理稿——而范围、留痕、可核验正是派生层的全部价值；
- 派生层的产物（块、向量、实体、关系）**一个都不用**，我们做的 RAG 变成摆设。

用户的判断很直接：**有 grep，还要我们的 RAG 干什么。**

## 代价与补法

- 代价：agent 失去「按文件名找文件」。补法是能力层给受约束的只读检索——
  有哪些文档用 `vault_list`，「哪篇说过 X」用 `vault_search`。
- 代价：agent **看不到 vault 外的东西**。这不是损失，**就是要的**。
- 与 0006 的自我纠正不冲突：那次纠正的是「一刀切把读也砍了 → agent 变瞎」；
  这次砍的不是读，是**一个不受约束、且会替掉我们检索的读**。

## 否掉的选项

- **按 0006 保留 `glob`/`grep`**：见上两条理由，等于自己拆掉范围与检索。
- **给该插件加 root 限制**：它没有这个配置项（Config 只有各种 size/count 上限），做不到。
- **只在 prompt/skill 里要求「别用 grep」**：软约束，模型换个说法就绕过去了；
  0003 的门是靠**工具给不给**保证的，不是靠嘱咐。

## 后果

- 后端工具定义体积只会更小（0006 记的那笔「7,863 → 5,220 token」是砍掉 pwsh/fs 之后的账）。
- `docs/specs/agent.spec.md` §「读放开、写收口」那句「外加 DSH 那侧纯只读的 `glob`/`grep`」同步删掉。
- 验证方式与踩过的坑写在 `dsh.spec.md`「能验到哪一步」（按 `id` 逐个配对看 `disabled: true`，
  别截一半输出就下结论）。
