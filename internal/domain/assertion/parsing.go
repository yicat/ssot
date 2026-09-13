package assertion

// Parsing 是候选的解析方式。它决定准入层能给的**最高**分级。
//
// 这条规则——**回退或推测解析不得定为 L1**——是关于「分级怎么定的」的
// 纯业务规则，因此归领域层；接入层只负责如实标注自己是怎么读出来的。
type Parsing string

const (
	// ParsingDirect 结构化直取：字段就在结构化数据里，可与原文逐字比对 → L1
	ParsingDirect Parsing = "direct"
	// ParsingText 从自由文本解析得出 → L2
	ParsingText Parsing = "text"
	// ParsingFallback 回退或推测解析 → **不得定为 L1**
	ParsingFallback Parsing = "fallback"
)

// Valid 报告解析方式是否合法。
func (p Parsing) Valid() bool {
	switch p {
	case ParsingDirect, ParsingText, ParsingFallback:
		return true
	}
	return false
}

// MaxConfidence 返回该解析方式允许的最高分级。
//
// 这是「回退解析不得定为 L1」这条规则的唯一实现处。
func (p Parsing) MaxConfidence() Confidence {
	switch p {
	case ParsingDirect:
		return L1
	case ParsingText:
		return L2
	default:
		return L4
	}
}

// Label 返回中文名，供界面显示。
func (p Parsing) Label() string {
	switch p {
	case ParsingDirect:
		return "结构化直取"
	case ParsingText:
		return "文本抽取"
	case ParsingFallback:
		return "回退解析"
	}
	return string(p)
}
