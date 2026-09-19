// 纠正块：文档里「这条我说了算」的写法。
//
// 语法定在 docs/specs/document.spec.md「纠正块的写法」，语义定在 derived.spec.md §九.2：
//
//	> [!correction] 茨木童子：鬼手是右手
//	> 类型：式神
//	> 说明：旧版设定里写成左手，改掉了。
//
// 为什么这条规则在 domain：它是**纯规则**——从正文里认出纠正块、算出它管哪一条、给出行号。
// 换成别的存储或索引，这件事照样要做（「换掉外部系统后还需要吗」那条落位判断）。
//
// 本包只允许 Go 标准库（分层铁律）。
package vault

import "strings"

// CorrectionKeyword 是纠正块的 callout 关键字：`> [!correction] …`。
//
// 为什么用英文关键字（2026-09-20 定）：前端 callout 解析器认的是 `[!(\w+)]`，
// 而 JS 的 `\w` **不含汉字**——`> [!纠正]` 根本不会被渲染成 callout（实测会退回普通引用块）。
// 关键字与已有的 note / tip / warning 同族，标题与正文仍然写中文。
const CorrectionKeyword = "correction"

// Correction 是一条纠正：文档里写明「<名字> 这条，更正后的说法是 <…>」。
//
// 它会被读成派生层里 `authority: corrected` 的一条来源行（重建时重读文件，
// 所以纠正活得过重建——ADR 0009 那条硬约束）。
type Correction struct {
	// Name / Type 是要纠正的那条实体的归并键。**Type 必填**：
	// 派生层按 (name, type) 归并，类型靠猜的话，猜错会让纠正变成另一个实体。
	Name string
	Type string
	// Description 是更正后的说法（覆盖模型抽出来的那份）。
	Description string
	// Note 是块里其余的行（给人看的，机器不管）。多行用 \n 连。
	Note string
	// FromLine / ToLine 是纠正块在**文件**里的行号区间（1 起，闭区间），
	// 与 vaultfs 的 BodyOffset、chunk 的行号是同一套。
	FromLine int
	ToLine   int
}

// IgnoredCorrection 是认出来了、但没写对因此**没生效**的纠正块。
//
// 「不静默」：重建时逐条报出来（连同原因），别让人改了没反应还以为是系统坏了。
type IgnoredCorrection struct {
	// Line 是纠正块第一行在文件里的行号。
	Line int
	// Reason 说明缺什么或哪句不对。
	Reason string
}

// ParseCorrections 从正文里认出所有纠正块。
//
// bodyOffset 是正文第一行在**文件**里的行号（vaultfs 的 Doc.BodyOffset）；
// bodyOffset<=0 时按 1 处理。写对的和没写对的分开返回，顺序都是正文顺序。
func ParseCorrections(body string, bodyOffset int) ([]Correction, []IgnoredCorrection) {
	if bodyOffset <= 0 {
		bodyOffset = 1
	}
	lines := strings.Split(normalizeNewlines(body), "\n")

	var out []Correction
	var bad []IgnoredCorrection
	for i := 0; i < len(lines); i++ {
		head, ok := correctionHead(lines[i])
		if !ok {
			continue
		}
		// 块的范围：连着往下走，直到某一行不再是引用行（`>` 开头）。
		content := []string{head}
		j := i + 1
		for j < len(lines) {
			inner, isQuote := quoteContent(lines[j])
			if !isQuote {
				break
			}
			content = append(content, inner)
			j++
		}
		from := bodyOffset + i
		to := bodyOffset + j - 1
		i = j - 1

		cor, reason := parseCorrectionBlock(content, from, to)
		if reason != "" {
			bad = append(bad, IgnoredCorrection{Line: from, Reason: reason})
			continue
		}
		out = append(out, cor)
	}
	return out, bad
}

// BlankCorrections 把纠正块**抹成空行**，行数一个不变。
//
// 用途：抽取块（喂模型的那 2000 口径）不该包含纠正块——它是给派生层的更正指令，
// 不是原文事实，不抹就会被再抽一遍、抽出来的还是错的那条（derived.spec.md §九.2）。
// **保留行数**是关键：块的行号是文件行号，删行会让后面所有块的行号错位。
func BlankCorrections(body string) string {
	lines := strings.Split(normalizeNewlines(body), "\n")
	for i := 0; i < len(lines); i++ {
		if _, ok := correctionHead(lines[i]); !ok {
			continue
		}
		for i < len(lines) {
			if _, isQuote := quoteContent(lines[i]); !isQuote {
				break
			}
			lines[i] = ""
			i++
		}
		i--
	}
	return strings.Join(lines, "\n")
}

// HasCorrections 报这篇正文里有没有纠正块（管它对不对，认出来就算）。
func HasCorrections(body string) bool {
	for _, line := range strings.Split(normalizeNewlines(body), "\n") {
		if _, ok := correctionHead(line); ok {
			return true
		}
	}
	return false
}

// parseCorrectionBlock 解析一个纠正块的内容行（第一行是标题行，其余是块内正文）。
// 返回的 reason 非空表示这条没生效。
func parseCorrectionBlock(content []string, from, to int) (Correction, string) {
	name, desc := splitAtColon(content[0])
	if name == "" || desc == "" {
		return Correction{}, "标题行要写成「> [!correction] 名字：更正后的说法」（名字与说法都不能空）"
	}
	cor := Correction{Name: name, Description: desc, FromLine: from, ToLine: to}
	var notes []string
	for _, line := range content[1:] {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if v, ok := fieldValue(t, "类型"); ok {
			cor.Type = v
			continue
		}
		// 其余行是给人看的，原样留着。
		notes = append(notes, t)
	}
	if cor.Type == "" {
		return Correction{}, "缺 `类型：…` 那一行（派生层按「名字 + 类型」归并，类型不能猜）"
	}
	cor.Note = strings.Join(notes, "\n")
	return cor, ""
}

// correctionHead 判断这一行是不是纠正块的标题行，是就返回 `[!correction]` 之后的内容。
func correctionHead(line string) (string, bool) {
	inner, ok := quoteContent(line)
	if !ok {
		return "", false
	}
	t := strings.TrimSpace(inner)
	prefix := "[!" + CorrectionKeyword + "]"
	// 关键字大小写不敏感（前端也是这么处理的），但只在 ASCII 前缀上比，别把整行折了。
	if len(t) < len(prefix) || !strings.EqualFold(t[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(t[len(prefix):]), true
}

// quoteContent 取引用行 `> xxx` 的内容；不是引用行时 ok=false。
//
// 只认 `>` 开头的行：纠正块**到第一个不是 `>` 的行就结束**（写法见 document.spec.md），
// 不搞「懒延续」那套——那种规则谁也说不清块到哪结束。
func quoteContent(line string) (string, bool) {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, ">") {
		return "", false
	}
	t = t[1:]
	t = strings.TrimPrefix(t, " ") // 只吃一个空格：`>   缩进` 里的缩进是内容
	return strings.TrimSuffix(t, "\r"), true
}

// splitAtColon 按**第一个**冒号切（全角半角都认）。
func splitAtColon(s string) (string, string) {
	for i, r := range s {
		if r == '：' || r == ':' {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+runeLen(r):])
		}
	}
	return strings.TrimSpace(s), ""
}

// fieldValue 认 `类型：X` 这种字段行；key 不含冒号。
func fieldValue(line, key string) (string, bool) {
	for i, r := range line {
		if r != '：' && r != ':' {
			continue
		}
		if strings.TrimSpace(line[:i]) != key {
			return "", false
		}
		return strings.TrimSpace(line[i+runeLen(r):]), true
	}
	return "", false
}

func runeLen(r rune) int {
	if r < 0x80 {
		return 1
	}
	return len(string(r))
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
