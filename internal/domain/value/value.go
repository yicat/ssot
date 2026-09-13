// Package value 定义字段取值的状态。
//
// 四态必须可区分（见 docs/specs/metamodel.spec.md 第三节）：
//
//	present 有确定的取值
//	unknown 该有值，但我们不知道它是什么   ← 「已知未知」的载体
//	null    明确无值
//	absent  字段未出现
//
// required 约束要求「不缺失」，因此 present / unknown / null 都满足它；
// 但只有 present 计入「已填充」。
//
// 没有 unknown 这一态，新增必填字段时既有数据就只能靠编造值来满足约束——
// 那正是数据源失准的起点。
package value

import "fmt"

// Kind 是取值的状态。
type Kind string

const (
	Present Kind = "present" // 有值
	Unknown Kind = "unknown" // 该有值，但我们不知道它是什么
	Null    Kind = "null"    // 明确无值
	Absent  Kind = "absent"  // 字段未出现
)

// Value 是一个字段的取值。
type Value struct {
	Kind Kind
	Data any    // 仅 Present 时有意义
	Unit string // 仅数值有意义；空字符串表示无量纲
}

// Of 构造一个有值、无量纲的取值。
func Of(data any) Value { return Value{Kind: Present, Data: data} }

// OfUnit 构造一个有值的数值取值，带量纲。
func OfUnit(data any, unit string) Value { return Value{Kind: Present, Data: data, Unit: unit} }

// UnknownValue 构造「已知未知」。
func UnknownValue() Value { return Value{Kind: Unknown} }

// NullValue 构造明确的空值。
func NullValue() Value { return Value{Kind: Null} }

// AbsentValue 构造缺失。
func AbsentValue() Value { return Value{Kind: Absent} }

// IsPresent 报告是否为有值状态。
func (v Value) IsPresent() bool { return v.Kind == Present }

// SatisfiesRequired 报告该取值是否满足 required 约束。
// 缺失是唯一不满足的情形。
func (v Value) SatisfiesRequired() bool { return v.Kind != Absent }

// CountsAsFilled 报告该取值是否计入「已填充」。
// 只有 present 计入；unknown / null / absent 都不计入。
func (v Value) CountsAsFilled() bool { return v.Kind == Present }

// Number 返回数值。非数值类型返回错误。
func (v Value) Number() (float64, error) {
	if v.Kind != Present {
		return 0, fmt.Errorf("取值不是有值状态（当前为 %s）", v.Kind)
	}
	switch n := v.Data.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case string:
		return 0, fmt.Errorf("取值是文本，不能作为数值使用：%q", n)
	default:
		return 0, fmt.Errorf("取值 %T 不能作为数值使用", v.Data)
	}
}

// String 返回可读表示。
func (v Value) String() string {
	switch v.Kind {
	case Present:
		if v.Unit != "" {
			return fmt.Sprintf("%v %s", v.Data, v.Unit)
		}
		return fmt.Sprintf("%v", v.Data)
	case Unknown:
		return "<未知>"
	case Null:
		return "<空>"
	case Absent:
		return "<缺失>"
	default:
		return string(v.Kind)
	}
}
