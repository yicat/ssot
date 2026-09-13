// Package unit 提供量纲与单位换算。
//
// 目的（见 docs/specs/computation.spec.md 第二节）：在求值前挡掉一整类错误——
// 把百分数当小数、把不同量纲相加。
//
// 实测依据：同一式神的暴击在来源 A 记为 50、来源 B 记为 0.5。
// 二者单位不同（percent 与 fraction）但同属 ratio 维度，
// **归一后是一致的**；不做归一就会把「一致」误判成「冲突」。
//
// 已知限制：同维度但语义不同的单位无法区分。
// 例如攻击与防御同属 point 维度，`atk + def` 不会被拦下——
// 那需要领域语义，超出量纲能力。
package unit

import (
	"fmt"
	"sort"
)

// Dimension 是可互相换算的单位所属的类别。
type Dimension string

const (
	Ratio    Dimension = "ratio"    // 无量纲比值
	Point    Dimension = "point"    // 点数
	Time     Dimension = "time"     // 时间
	Currency Dimension = "currency" // 货币
)

// Label 返回维度的中文名。
func (d Dimension) Label() string {
	switch d {
	case Ratio:
		return "比值"
	case Point:
		return "点数"
	case Time:
		return "时间"
	case Currency:
		return "货币"
	}
	return string(d)
}

// Unit 是一个具体计量单位。
type Unit struct {
	Name      string
	Dimension Dimension
	Scale     float64 // 相对该维度基准单位的倍率
	// Label 是该单位的中文名。**空串表示未知**——那时界面退回显示维度名，
	// 而不是编一个听起来合理的名字。
	Label string
}

// Table 是单位与换算关系的集合。换算关系是数据，可扩展。
type Table struct {
	units map[string]Unit
}

// NewTable 构造一个空表。
func NewTable() *Table { return &Table{units: map[string]Unit{}} }

// Register 注册一个单位。
func (t *Table) Register(u Unit) {
	if u.Scale == 0 {
		u.Scale = 1
	}
	t.units[u.Name] = u
}

// Lookup 查找单位。
func (t *Table) Lookup(name string) (Unit, bool) {
	if name == "" {
		return Unit{Name: "", Dimension: Ratio, Scale: 1}, true
	}
	u, ok := t.units[name]
	return u, ok
}

// Names 返回全部已注册单位名（排序）。
func (t *Table) Names() []string {
	out := make([]string, 0, len(t.units))
	for n := range t.units {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Known 报告单位是否已注册。空字符串视为合法（无量纲）。
func (t *Table) Known(name string) bool {
	if name == "" {
		return true
	}
	_, ok := t.units[name]
	return ok
}

// All 返回全部单位，按名称排序。
//
// 词表用它列出「这个项目认识哪些单位」——界面因此可以把 percent 显示成
// 「百分比」，而不是让看的人自己去猜。
func (t *Table) All() []Unit {
	out := make([]Unit, 0, len(t.units))
	for _, u := range t.units {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Compatible 报告两个单位是否同维度、可互相换算。
func (t *Table) Compatible(a, b string) bool {
	ua, okA := t.Lookup(a)
	ub, okB := t.Lookup(b)
	if !okA || !okB {
		return false
	}
	return ua.Dimension == ub.Dimension
}

// Convert 把数值从 from 单位换算到 to 单位。
func (t *Table) Convert(v float64, from, to string) (float64, error) {
	uf, okF := t.Lookup(from)
	ut, okT := t.Lookup(to)
	if !okF {
		return 0, fmt.Errorf("未知单位 %q", from)
	}
	if !okT {
		return 0, fmt.Errorf("未知单位 %q", to)
	}
	if uf.Dimension != ut.Dimension {
		return 0, fmt.Errorf("单位 %q(%s) 与 %q(%s) 量纲不同，无法换算",
			from, uf.Dimension, to, ut.Dimension)
	}
	if ut.Scale == 0 {
		return 0, fmt.Errorf("单位 %q 的换算系数为 0", to)
	}
	return v * uf.Scale / ut.Scale, nil
}

// AddUnits 返回加减运算的结果单位。
//
// 规则：同维度可换算；无量纲字面量采用另一侧的单位。
func (t *Table) AddUnits(a, b string) (string, error) {
	switch {
	case a == "" && b == "":
		return "", nil
	case a == "":
		return b, nil
	case b == "":
		return a, nil
	}
	if !t.Compatible(a, b) {
		ua, _ := t.Lookup(a)
		ub, _ := t.Lookup(b)
		return "", fmt.Errorf("量纲不同，不能相加减：%q(%s) 与 %q(%s)", a, ua.Dimension, b, ub.Dimension)
	}
	// 取左操作数的单位
	return a, nil
}

// MulUnits 返回乘法运算的结果单位。
//
// 规则：ratio 视为无量纲缩放；不支持复合单位，两个非 ratio 单位相乘即报错。
func (t *Table) MulUnits(a, b string) (string, error) {
	switch {
	case a == "" && b == "":
		return "", nil
	case a == "":
		return b, nil
	case b == "":
		return a, nil
	}
	ua, _ := t.Lookup(a)
	ub, _ := t.Lookup(b)
	ar, br := ua.Dimension == Ratio, ub.Dimension == Ratio
	switch {
	case ar && br:
		return "", nil // 两个比值相乘仍是无量纲
	case ar:
		return b, nil
	case br:
		return a, nil
	default:
		return "", fmt.Errorf("不支持复合单位：%q(%s) 与 %q(%s) 相乘", a, ua.Dimension, b, ub.Dimension)
	}
}

// DivUnits 返回除法运算的结果单位。
func (t *Table) DivUnits(a, b string) (string, error) {
	switch {
	case a == "" && b == "":
		return "", nil
	case b == "":
		return a, nil
	case a == "":
		// 无量纲 / 有量纲 -> 不支持（会产生 1/point 这类复合单位）
		ub, _ := t.Lookup(b)
		if ub.Dimension == Ratio {
			return "", nil
		}
		return "", fmt.Errorf("不支持复合单位：无量纲除以 %q(%s)", b, ub.Dimension)
	}
	if a == b {
		return "", nil // 同单位相除得到无量纲比值
	}
	ua, _ := t.Lookup(a)
	ub, _ := t.Lookup(b)
	if ub.Dimension == Ratio {
		return a, nil
	}
	if ua.Dimension == Ratio {
		return "", fmt.Errorf("不支持复合单位：%q(%s) 除以 %q(%s)", a, ua.Dimension, b, ub.Dimension)
	}
	return "", fmt.Errorf("不支持复合单位：%q(%s) 除以 %q(%s)", a, ua.Dimension, b, ub.Dimension)
}

// Default 返回引擎内置的基础单位表。
func Default() *Table {
	t := NewTable()
	// ratio 维度：fraction 为基准
	t.Register(Unit{Name: "fraction", Dimension: Ratio, Scale: 1, Label: "小数"})
	t.Register(Unit{Name: "percent", Dimension: Ratio, Scale: 0.01, Label: "百分比"})
	// point 维度
	t.Register(Unit{Name: "point", Dimension: Point, Scale: 1, Label: "点数"})
	// time 维度：day 为基准
	t.Register(Unit{Name: "day", Dimension: Time, Scale: 1, Label: "天"})
	t.Register(Unit{Name: "week", Dimension: Time, Scale: 7, Label: "周"})
	t.Register(Unit{Name: "month", Dimension: Time, Scale: 30, Label: "月"})
	t.Register(Unit{Name: "year", Dimension: Time, Scale: 365, Label: "年"})
	// currency 维度
	// gem 的取名权留给项目：引擎只知道它是货币维度的基准单位。
	// 编一个「宝石」听起来合理，但另一个项目可能拿它当积分。
	t.Register(Unit{Name: "gem", Dimension: Currency, Scale: 1})
	return t
}
