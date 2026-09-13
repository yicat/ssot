// Package expr 实现场景声明领域规则所用的小型表达式语言。
//
// 见 docs/specs/computation.spec.md。设计目标是三件事：
//
//  1. 可静态检查——字段是否存在、类型是否匹配、**单位是否相容**，
//     都必须在撰写期而非运行期报错。
//  2. 量纲感知——`percent + point` 必须被拦下；`50%` 与 `0.5` 必须等价。
//  3. 确定性——相同输入相同输出（本 MVP 不含随机量）。
//
// 之所以自研而非引入库：离线环境无任何表达式库可用，且网络恢复后引入
// 第三方语言也意味着校验逻辑变成不可静态检查的字符串。
package expr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Kind 是求值结果的类型。
type Kind int

const (
	KNumber Kind = iota
	KText
	KBool
	KUnknown // 未知值参与运算的结果
)

func (k Kind) String() string {
	switch k {
	case KNumber:
		return "数值"
	case KText:
		return "文本"
	case KBool:
		return "布尔"
	case KUnknown:
		return "未知"
	}
	return "?"
}

// Val 是求值结果。数值带单位。
type Val struct {
	Kind Kind
	Num  float64
	Unit string
	Text string
	Bool bool
}

// Number 构造数值。
func Number(n float64, unit string) Val { return Val{Kind: KNumber, Num: n, Unit: unit} }

// Str 构造文本。
func Str(s string) Val { return Val{Kind: KText, Text: s} }

// Bl 构造布尔。
func Bl(b bool) Val { return Val{Kind: KBool, Bool: b} }

// UnknownVal 构造未知值。
func UnknownVal() Val { return Val{Kind: KUnknown} }

func (v Val) String() string {
	switch v.Kind {
	case KNumber:
		if v.Unit != "" {
			return fmt.Sprintf("%g %s", v.Num, v.Unit)
		}
		return fmt.Sprintf("%g", v.Num)
	case KText:
		return v.Text
	case KBool:
		return strconv.FormatBool(v.Bool)
	}
	return "<未知>"
}

// FieldSpec 描述一个可用字段的静态信息。
type FieldSpec struct {
	Type string // "number" | "text" | "bool"
	Unit string // 仅 number 有意义
}

// Env 提供静态检查所需的字段信息。
type Env interface {
	Field(path string) (FieldSpec, bool)
}

// MapEnv 是 Env 的简单实现。
type MapEnv map[string]FieldSpec

func (m MapEnv) Field(path string) (FieldSpec, bool) {
	s, ok := m[path]
	return s, ok
}

// Error 是带位置的表达式错误。
type Error struct {
	Pos    int
	Msg    string
	Source string
}

func (e *Error) Error() string {
	return fmt.Sprintf("第 %d 列：%s", e.Pos+1, e.Msg)
}

// Caret 返回带指示符的源码行，便于定位。
func (e *Error) Caret() string {
	if e.Source == "" {
		return ""
	}
	return e.Source + "\n" + strings.Repeat(" ", e.Pos) + "^"
}

// ── 词法 ────────────────────────────────────────────────────────────────────

type tokKind int

const (
	tEOF tokKind = iota
	tNumber
	tString
	tIdent
	tOp
	tLParen
	tRParen
	tComma
	tDot
)

type token struct {
	kind tokKind
	text string
	num  float64
	unit string // 数值字面量可以自带单位，例如 `50%`
	pos  int
}

func (t token) String() string {
	switch t.kind {
	case tEOF:
		return "表达式结尾"
	case tNumber:
		return fmt.Sprintf("数值 %g", t.num)
	case tString:
		return fmt.Sprintf("文本 %q", t.text)
	case tIdent:
		return fmt.Sprintf("名称 %q", t.text)
	default:
		return fmt.Sprintf("%q", t.text)
	}
}

var keywords = map[string]bool{
	"if": true, "then": true, "else": true,
	"and": true, "or": true, "not": true,
	"true": true, "false": true,
}

func lex(src string) ([]token, error) {
	var out []token
	rs := []rune(src)
	i := 0
	for i < len(rs) {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(':
			out = append(out, token{kind: tLParen, text: "(", pos: i})
			i++
		case c == ')':
			out = append(out, token{kind: tRParen, text: ")", pos: i})
			i++
		case c == ',':
			out = append(out, token{kind: tComma, text: ",", pos: i})
			i++
		case c == '.':
			out = append(out, token{kind: tDot, text: ".", pos: i})
			i++
		case c == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			if j >= len(rs) {
				return nil, &Error{Pos: i, Msg: "文本未闭合", Source: src}
			}
			out = append(out, token{kind: tString, text: string(rs[i+1 : j]), pos: i})
			i = j + 1
		case unicode.IsDigit(c):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			n, err := strconv.ParseFloat(string(rs[i:j]), 64)
			if err != nil {
				return nil, &Error{Pos: i, Msg: "非法数值：" + string(rs[i:j]), Source: src}
			}
			tk := token{kind: tNumber, num: n, pos: i, text: string(rs[i:j])}
			// `%` 紧跟在数值之后视为百分比单位；否则 `%` 是取模运算符。
			// 二者靠「是否紧邻」区分：`50%` 是百分比，`a % b` 是取模。
			if j < len(rs) && rs[j] == '%' {
				tk.unit = "percent"
				j++
			}
			out = append(out, tk)
			i = j
		case isIdentStart(c):
			j := i
			for j < len(rs) && isIdentPart(rs[j]) {
				j++
			}
			text := string(rs[i:j])
			out = append(out, token{kind: tIdent, text: text, pos: i})
			i = j
		default:
			// 多字符运算符优先
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "==", "!=", "<=", ">=":
				out = append(out, token{kind: tOp, text: two, pos: i})
				i += 2
				continue
			}
			switch c {
			case '+', '-', '*', '/', '%', '^', '<', '>':
				out = append(out, token{kind: tOp, text: string(c), pos: i})
				i++
			default:
				return nil, &Error{Pos: i, Msg: fmt.Sprintf("无法识别的字符 %q", string(c)), Source: src}
			}
		}
	}
	out = append(out, token{kind: tEOF, pos: len(rs)})
	return out, nil
}

func isIdentStart(c rune) bool {
	return unicode.IsLetter(c) || c == '_'
}

func isIdentPart(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_'
}
