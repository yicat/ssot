// 提示词：结构照 LightRAG 借（docs/specs/derived.spec.md §二），**内容全部来自项目声明**。
//
// ⚠️ 代码里不许写死实体类型词表或反例（`.ssot/derived-scope.yml` 提供）：
// 换一个项目就是另一套术语，写死在代码里必然错。
package vextract

import (
	"fmt"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// BuildPrompt 把一批块拼成一次调用（**多块合一次**——spec §三.1 的硬约束）。
func BuildPrompt(chunks []Chunk, cfg vault.ExtractConfig) string {
	var sb strings.Builder
	sb.WriteString(`你是信息抽取器。下面给你若干**文档块**，每块带 doc（相对路径）与**文件行号区间**。
请抽出块里出现的**实体**与**实体之间的关系**，只输出一个 JSON 对象，不要写别的解释。

要求：
1. 实体字段：name / type / description / doc / line
2. 关系字段：source / target / keywords / description / doc / line
   - source 与 target 必须是上面抽出的实体名（原样写，别改写）
   - keywords 用**高层词**（概括类别），不要照抄句子
3. description 控制在 40 字以内，说清「是什么/做什么」，不要空话
4. doc 照抄块头给的值；line 写该内容在**文件**里的行号，必须落在该块的行号区间内
5. **只抽「正文里讲了的东西」**——名字像下面这些形状的都不是实体：
`)
	if pats := cfg.IgnoreNamePatterns; len(pats) > 0 {
		for _, p := range pats {
			fmt.Fprintf(&sb, "   - 名字匹配 `%s` 的\n", p)
		}
	} else {
		sb.WriteString("   - （项目还没声明忽略形状）按常识判断：文件名、路径、占位符、命令与工具名不是实体\n")
	}
	if words := cfg.EmptyWords; len(words) > 0 {
		fmt.Fprintf(&sb, "   - 这些空占位词：%s\n", strings.Join(words, "、"))
	}
	sb.WriteString("6. type 的取值：")
	if types := cfg.EntityTypes; len(types) > 0 {
		fmt.Fprintf(&sb, "只用这些词：%s；拿不准就用 %s\n", strings.Join(types, "、"), vault.OtherType)
	} else {
		fmt.Fprintf(&sb, "项目还没声明类型词表，一律用 %s\n", vault.OtherType)
	}
	sb.WriteString("7. 多文字面量（数字、符号、单位）要保持原样，不要改写\n")
	if ex := cfg.Examples; len(ex) > 0 {
		sb.WriteString("8. 这些是**反例**（照声明给的，别抽它们）：\n")
		for _, e := range ex {
			fmt.Fprintf(&sb, "   - %s\n", strings.TrimSpace(e))
		}
	}
	sb.WriteString("9. 块里没有可抽的就返回空数组；宁缺毋滥，不要脑补块外的信息\n")

	sb.WriteString(`
输出格式（只输出这个 JSON）：
{"entities":[{"name":"…","type":"…","description":"…","doc":"…","line":0}],
 "relations":[{"source":"…","target":"…","keywords":"…","description":"…","doc":"…","line":0}]}

`)
	fmt.Fprintf(&sb, "块（共 %d 块）：\n", len(chunks))
	for i, c := range chunks {
		fmt.Fprintf(&sb, "\n--- 块 %d ---\ndoc: %s\n行号: %s\n%s\n", i+1, c.Doc, lineRange(c), c.Text)
	}
	return sb.String()
}

// BuildGleaningPrompt 是补抽轮（LightRAG 的 `entity_continue_extraction`）：
// 把上一轮的回包一起带上，让它只补**漏掉的**，避免重复。
func BuildGleaningPrompt(chunks []Chunk, prev string, cfg vault.ExtractConfig) string {
	var sb strings.Builder
	sb.WriteString(BuildPrompt(chunks, cfg))
	sb.WriteString("\n上一轮你已经抽出了这些（JSON）：\n")
	sb.WriteString(head(prev, 6000))
	sb.WriteString(`

上面可能还有**遗漏**的实体与关系。请**只输出新增的**（同样格式；没有新增就输出空数组），
不要重复上面已经给过的条目，也不要解释。
`)
	return sb.String()
}

// lineRange 是块的行号区间说明（单行时不写成 `12-12`，免得模型困惑）。
func lineRange(c Chunk) string {
	if c.FromLine == c.ToLine {
		return fmt.Sprintf("%d", c.FromLine)
	}
	return fmt.Sprintf("%d-%d", c.FromLine, c.ToLine)
}
