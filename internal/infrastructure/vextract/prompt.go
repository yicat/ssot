// 提示词：照 LightRAG 的结构借（docs/specs/derived.spec.md §二），加我们自己的要求
// （每条必须带 doc + 文件行号、类型用我们的词表、description 压短）。
//
// ⚠️ 这份提示词**没定稿**（spec §六.4）：它是起点，改它要拿跑批的产出与坏例说话。
package vextract

import (
	"fmt"
	"strings"
)

// BuildPrompt 把一批块拼成一次调用（**多块合一次**——spec §三.1 的硬约束）。
func BuildPrompt(chunks []Chunk) string {
	var sb strings.Builder
	sb.WriteString(`你是信息抽取器。下面给你若干**文档块**，每块带 doc（相对路径）与**文件行号区间**。
请抽出块里出现的**实体**与**实体之间的关系**，只输出一个 JSON 对象，不要写别的解释。

要求：
1. 实体字段：name / type / description / doc / line
   - type 只用这些词：`)
	sb.WriteString(strings.Join(EntityTypes, "、"))
	sb.WriteString(`
   - 拿不准就用 Other
2. 关系字段：source / target / keywords / description / doc / line
   - source 与 target 必须是上面抽出的实体名（原样写，别改写）
   - keywords 用「技能/伤害」「CV/配音」这类**高层词**，不要照抄句子
3. description 控制在 40 字以内，说清「是什么/做什么」，不要空话
4. doc 照抄块头给的值；line 写该内容在**文件**里的行号，必须落在该块的行号区间内
5. **只抽「正文里讲了的东西」**。下面这些**不算实体**（实测最容易出错的几类）：
   - 文件名、表名、路径（例如「增益减益.csv」「tables/式神属性.csv」）
   - 命令与用法示例里的词（例如「ssot vault」「--patch」「mcp__ssot__file_read」）
   - 章节序号、页码、年份数字本身（「1.2」「2026」）
   - 空的占位（「无」「待定」「其他」）
6. type 要**尽量用具体的那一类**（式神/技能/机制/数值/物品/地点/组织/人物）；
   只有真的归不进去才用 Other——**Other 太多等于没分类**
7. 块里没有可抽的就返回空数组；宁缺毋滥，不要脑补块外的信息

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
func BuildGleaningPrompt(chunks []Chunk, prev string) string {
	var sb strings.Builder
	sb.WriteString(BuildPrompt(chunks))
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
