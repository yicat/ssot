package vault

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Link 是正文里的一条双链：`[[目标]]`、`[[目标#标题]]`、`[[目标#^块]]`、`[[目标|显示文本]]`。
//
// 块级锚点（`#^块`）是这套东西的**主张级溯源**：整理层的每条主张能指回原始层的具体段落。
// 对照：WeKnora 只到 chunk、LightRAG 只到 chunk、MaxKB 只到段落——它们做不到这一级。
type Link struct {
	// Raw 是原文（含方括号），便于回指与显示。
	Raw string
	// Target 是目标文档，未规范化（可能是 `docs/式神/茨木童子`，也可能只是 `茨木童子`）。
	Target string
	// Heading 是 `#标题` 锚点（可空）。
	Heading string
	// Block 是 `#^块` 锚点，不含 `^`（可空）。
	Block string
	// Alias 是 `|显示文本`（可空），只影响显示，不影响解析。
	Alias string
	// Embed 表示这是 `![[...]]` 嵌入而非普通链接。
	// ⚠️ 嵌入的语义（要不要展开内容）**还没定**，现在只记录、不展开。
	Embed bool
	// Offset 是这条链接在正文里的字节偏移。
	Offset int
}

// ParseLinks 扫出正文里的全部双链。
//
// 只认成对的 `[[ ]]`；没闭合的按普通文本处理——**不猜**，
// 把不完整的东西当成链接会让人以为引用已经建立。
func ParseLinks(body string) []Link {
	var out []Link
	for i := 0; i < len(body); {
		open := strings.Index(body[i:], "[[")
		if open < 0 {
			break
		}
		start := i + open
		end := strings.Index(body[start+2:], "]]")
		if end < 0 {
			break
		}
		raw := body[start : start+2+end+2]
		if l, err := parseLinkInner(raw, body[start+2:start+2+end]); err == nil {
			l.Offset = start
			// `![[...]]` 的 `!` 紧跟在前一位。
			if start > 0 && body[start-1] == '!' {
				l.Embed = true
			}
			out = append(out, l)
		}
		i = start + 2 + end + 2
	}
	return out
}

// ParseLink 解析一条双链。方括号可以省略（命令行里手打整串方括号很烦），
// 前面带 `!` 视为嵌入。
func ParseLink(text string) (Link, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return Link{}, fmt.Errorf("空的双链")
	}
	embed := false
	if strings.HasPrefix(s, "!") {
		embed = true
		s = strings.TrimSpace(s[1:])
	}
	raw := s
	if strings.HasPrefix(s, "[[") && strings.HasSuffix(s, "]]") {
		inner := s[2 : len(s)-2]
		l, err := parseLinkInner(raw, inner)
		if err != nil {
			return Link{}, err
		}
		l.Embed = embed
		return l, nil
	}
	l, err := parseLinkInner(raw, s)
	if err != nil {
		return Link{}, err
	}
	l.Embed = embed
	return l, nil
}

// parseLinkInner 拆 `目标#锚点|别名`。别名先拆，再拆锚点——顺序错了
// （先拆 #）会把 `[[a|b#c]]` 这类写法解析歪。
func parseLinkInner(raw, inner string) (Link, error) {
	l := Link{Raw: raw}
	inner = strings.TrimSpace(inner)
	if idx := strings.LastIndex(inner, "|"); idx >= 0 {
		l.Alias = strings.TrimSpace(inner[idx+1:])
		inner = inner[:idx]
	}
	if idx := strings.Index(inner, "#"); idx >= 0 {
		anchor := strings.TrimSpace(inner[idx+1:])
		inner = inner[:idx]
		if strings.HasPrefix(anchor, "^") {
			l.Block = strings.TrimSpace(anchor[1:])
		} else {
			l.Heading = anchor
		}
	}
	l.Target = strings.TrimSpace(inner)
	if l.Target == "" && l.Block == "" && l.Heading == "" {
		return Link{}, fmt.Errorf("双链里没有目标：%q", raw)
	}
	return l, nil
}

// Resolution 是一条双链被解析到的东西。
type Resolution struct {
	// Path 是命中的文档（相对 vault 根的路径）。
	Path string
	// Heading / Block 是原样带过来的锚点。
	Heading string
	Block   string
	// HeadingLine 是标题锚点命中的**文件行号**（1 起；0 表示没找到）。
	HeadingLine int
	// BlockLine / BlockText 是块锚点命中的文件行号与那一行原文（行号 0 表示没找到）。
	// 用文件行号而不是正文内行号：人拿到它要能直接在编辑器里跳过去。
	BlockLine int
	BlockText string
	// Candidates 只在目标有歧义时出现：同名文档有多篇，谁都不选——**系统不裁决**。
	Candidates []string
}

// Resolve 把一条双链解析到具体文档（含锚点命中情况）。
//
// 目标可以是全路径，也可以只是文件名（像 Obsidian 那样 `[[茨木童子]]`）。
// 同名多篇时**不猜**：返回候选列表让调用方去问人（系统不裁决）。
func Resolve(docs []Doc, link Link) (Resolution, error) {
	link = normalizeLink(link)
	res := Resolution{Heading: link.Heading, Block: link.Block}
	target := NormalizeTarget(link.Target)
	if target == "" {
		// 只写了锚点（`[[#^块]]`）：指本文档自身。
		return res, fmt.Errorf("双链没有目标文档（只有锚点 %q）", link.Raw)
	}

	for _, d := range docs {
		if d.Path == target {
			return fill(d, res), nil
		}
	}
	for _, d := range docs {
		if d.Path == target+".md" {
			return fill(d, res), nil
		}
	}
	// 退一步：只按文件名匹配（不含扩展名）。
	want := strings.TrimSuffix(path.Base(target), ".md")
	var hits []Doc
	for _, d := range docs {
		if strings.TrimSuffix(path.Base(d.Path), ".md") == want {
			hits = append(hits, d)
		}
	}
	switch len(hits) {
	case 0:
		return res, fmt.Errorf("找不到双链目标 %q（写过 %q 都试了）", link.Target, target)
	case 1:
		return fill(hits[0], res), nil
	default:
		for _, d := range hits {
			res.Candidates = append(res.Candidates, d.Path)
		}
		sort.Strings(res.Candidates)
		// 报得能让人做决定：把候选列出来，而不是只说「有歧义」。
		return res, fmt.Errorf("双链目标 %q 有 %d 个同名文档，需要你指明是哪一个：%s",
			link.Target, len(hits), strings.Join(res.Candidates, "、"))
	}
}

func fill(d Doc, res Resolution) Resolution {
	res.Path = d.Path
	// 行号换算成**文件行号**：FindBlock/FindHeading 报的是正文内行号，
	// 加上 front matter 的偏移才是人能直接跳过去的行号。
	toFileLine := func(bodyLine int) int {
		if bodyLine <= 0 {
			return 0
		}
		offset := d.BodyOffset
		if offset <= 0 {
			offset = 1
		}
		return offset + bodyLine - 1
	}
	if res.Block != "" {
		if line, text, ok := FindBlock(d.Body, res.Block); ok {
			res.BlockLine, res.BlockText = toFileLine(line), text
		}
	}
	if res.Heading != "" {
		if line, ok := FindHeading(d.Body, res.Heading); ok {
			res.HeadingLine = toFileLine(line)
		}
	}
	return res
}

// normalizeLink 容忍「锚点还留在 Target 里」这种用法。
//
// 锚点本该由 ParseLink 拆开，但调用方（尤其是 agent 生成的调用）很容易把
// `raw/x#^段` 整串塞进 Target。宽进严出：这里补一次拆分，而不是让解析悄悄落空。
func normalizeLink(link Link) Link {
	if (link.Block != "" || link.Heading != "") || !strings.Contains(link.Target, "#") {
		return link
	}
	if parsed, err := ParseLink(link.Target); err == nil {
		parsed.Raw, parsed.Embed = link.Raw, link.Embed
		return parsed
	}
	return link
}

// NormalizeTarget 把目标统一成 `/` 分隔、去掉 `./` 与前导 `/` 的相对路径。
func NormalizeTarget(target string) string {
	t := strings.TrimSpace(strings.ReplaceAll(target, "\\", "/"))
	t = strings.TrimPrefix(t, "./")
	return strings.TrimPrefix(t, "/")
}

// FindBlock 找块锚点：某一行结尾处的 `^块id`。返回行号（1 起）与那一行原文。
//
// 行尾才算——正文里出现 `^abc` 是普通字符，不能当成锚点。
func FindBlock(body, block string) (int, string, bool) {
	if block == "" {
		return 0, "", false
	}
	marker := "^" + block
	for i, line := range strings.Split(body, "\n") {
		if strings.HasSuffix(strings.TrimRight(line, " \t\r"), marker) {
			return i + 1, strings.TrimRight(line, " \t\r"), true
		}
	}
	return 0, "", false
}

// FindHeading 找标题锚点：`# 标题` 或 `## 标题`（层级不限）。返回行号（1 起）。
func FindHeading(body, heading string) (int, bool) {
	heading = strings.TrimSpace(heading)
	for i, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "#") {
			continue
		}
		if strings.TrimSpace(strings.TrimLeft(t, "#")) == heading {
			return i + 1, true
		}
	}
	return 0, false
}

// Backlink 是一条反向链接：哪篇文档的哪条链接指向了目标。
type Backlink struct {
	// From 是指向目标的文档路径。
	From string
	// Link 是那条链接本身（含 Raw、锚点，便于显示「他引的是哪一段」）。
	Link Link
}

// Backlinks 算出指向 target 的全部反链，按来源路径排序。
//
// 反链是**算出来的**，不写进文件（文件里只存正链）——见 vault.spec.md §3。
// 链到不存在文档的链接不在这里报错：那是另一个问题（断链），由调用方单独查。
func Backlinks(docs []Doc, target string) []Backlink {
	target = NormalizeTarget(target)
	var out []Backlink
	for _, d := range docs {
		if d.Path == target {
			continue // 自链不算反链
		}
		for _, l := range d.Links {
			res, err := Resolve(docs, l)
			if err != nil || res.Path != target {
				continue
			}
			out = append(out, Backlink{From: d.Path, Link: l})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].Link.Offset < out[j].Link.Offset
	})
	return out
}

// LinkIssueKind 是链接问题的种类。
//
// 把「断了」和「指不清」分开：前者是目标真不存在，后者是同名多篇、需要人指明。
// 混成一类会让人以为内容缺失，实际是命名冲突——报错要报得能让人做决定。
type LinkIssueKind string

const (
	// IssueBroken：目标不存在（断链）。
	IssueBroken LinkIssueKind = "broken"
	// IssueAmbiguous：同名多篇，指不清（不是断链）。
	IssueAmbiguous LinkIssueKind = "ambiguous"
)

// LinkIssue 是一条有问题的链接。
type LinkIssue struct {
	Link   Link
	Kind   LinkIssueKind
	Reason string
}

// LinkIssues 找某篇文档链出去的问题链接（断链与歧义分开报）。
//
// 这两件事都算质量问题，得能列出来——只报反链会让人以为文档很干净。
func LinkIssues(docs []Doc, from string) []LinkIssue {
	var out []LinkIssue
	for _, d := range docs {
		if d.Path != NormalizeTarget(from) {
			continue
		}
		for _, l := range d.Links {
			res, err := Resolve(docs, l)
			if err == nil {
				continue
			}
			kind := IssueBroken
			if len(res.Candidates) > 0 {
				kind = IssueAmbiguous
			}
			out = append(out, LinkIssue{Link: l, Kind: kind, Reason: err.Error()})
		}
	}
	return out
}
