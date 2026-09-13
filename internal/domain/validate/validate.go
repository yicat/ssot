// Package validate 实现六类校验。
//
// 见 docs/specs/core.spec.md「校验」一节。六类各自独立判定：
// 必填、类型、范围、枚举、唯一、引用完整性。
//
// **关键设计：对结构错误严，对数据取值宽但不静默。**
//
//	schema 用未知类型名      -> 拒绝加载（mm 包，防我们自己写错）
//	数据取值超出枚举集合     -> 接受并标记「未建模取值」，不进 Violations
//
// 丢弃等于信息损失；静默通过等于放任错误。二者都不取。
package validate

import (
	"fmt"
	"math"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/metamodel"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/value"
)

// Record 是一条待校验的记录。
type Record struct {
	Entity string
	Values map[string]value.Value
}

// Violation 是一次拒绝。
type Violation struct {
	Record string
	Field  string
	Rule   string // required / type / range / enum-unit / unique / ref / precision
	Detail string
}

func (v Violation) String() string {
	f := v.Field
	if f == "" {
		f = "-"
	}
	return fmt.Sprintf("[%s] %s.%s: %s", v.Rule, v.Record, f, v.Detail)
}

// Marker 是一条「接受但标记」。它不阻止写入，但必须随数据一起流转。
type Marker struct {
	Record string
	Field  string
	Kind   string // unmodeled-enum-value
	Detail string
}

// Report 是一条记录的校验结果。
type Report struct {
	RecordID   string
	Violations []Violation
	Markers    []Marker
}

// OK 报告是否通过（无 Violation）。
func (r Report) OK() bool { return len(r.Violations) == 0 }

// Checkers 提供需要跨记录查询的校验能力（集合级约束）。
type Checkers struct {
	// UniqueExists 报告 (entity, field, value) 是否已被其他记录占用。
	UniqueExists func(entity, field string, v any) (bool, error)
	// RefExists 报告目标实体中是否存在该身份值。
	RefExists func(entity string, id any) (bool, error)
	// Units 用于数值单位的一致性检查；为 nil 时跳过单位校验。
	Units *unit.Table
}

// Record 对一条记录执行六类校验。
//
// 注意：值为 unknown / null / absent 时**不做类型与范围检查**——
// 我们不知道那个值，无从判断。required 只要求「不缺失」。
func Check(entity *metamodel.Entity, rec Record, ck Checkers) Report {
	rep := Report{RecordID: recID(entity, rec)}
	for i := range entity.Fields {
		f := &entity.Fields[i]
		v, present := rec.Values[f.Key]
		if !present {
			v = value.AbsentValue()
		}
		checkField(entity, rec, f, f.Key, v, ck, &rep)
	}
	return rep
}

func checkField(entity *metamodel.Entity, rec Record, f *metamodel.Field, path string, v value.Value, ck Checkers, rep *Report) {
	addV := func(rule, detail string) {
		rep.Violations = append(rep.Violations, Violation{Record: recID(entity, rec), Field: path, Rule: rule, Detail: detail})
	}

	// 1. 必填
	if f.Required && !v.SatisfiesRequired() {
		addV("required", "必填字段缺失")
	}
	if !v.IsPresent() {
		// 未知/空/缺失：无从判断类型与范围，直接返回
		return
	}

	// 2. 类型 + 4. 枚举 + 5. 唯一 + 6. 引用
	switch f.Type {
	case metamodel.TypeText:
		s, ok := v.Data.(string)
		if !ok {
			addV("type", fmt.Sprintf("期望文本，实际 %T", v.Data))
			return
		}
		_ = s

	case metamodel.TypeBool:
		if _, ok := v.Data.(bool); !ok {
			addV("type", fmt.Sprintf("期望布尔，实际 %T", v.Data))
			return
		}

	case metamodel.TypeNumber:
		n, err := v.Number()
		if err != nil {
			addV("type", err.Error())
			return
		}
		// 单位一致性
		if f.Unit != "" && ck.Units != nil {
			if v.Unit == "" {
				addV("enum-unit", fmt.Sprintf("数值缺少单位声明，期望 %q", f.Unit))
			} else if !ck.Units.Compatible(v.Unit, f.Unit) {
				addV("enum-unit", fmt.Sprintf("单位 %q 与声明的 %q 量纲不同", v.Unit, f.Unit))
			}
		}
		// 精度
		if f.Precision != nil && *f.Precision == 0 && n != math.Trunc(n) {
			addV("precision", fmt.Sprintf("期望整数，实际 %v", n))
		}
		// 3. 范围
		if f.Min != nil && n < *f.Min {
			addV("range", fmt.Sprintf("%v 小于下界 %v", n, *f.Min))
		}
		if f.Max != nil && n > *f.Max {
			addV("range", fmt.Sprintf("%v 大于上界 %v", n, *f.Max))
		}

	case metamodel.TypeEnum:
		s, ok := v.Data.(string)
		if !ok {
			addV("type", fmt.Sprintf("期望文本枚举，实际 %T", v.Data))
			return
		}
		if !contains(f.Values, s) {
			// 接受，但标记——外部数据出现新取值是常态，丢弃等于信息损失
			rep.Markers = append(rep.Markers, Marker{
				Record: recID(entity, rec), Field: path, Kind: "unmodeled-enum-value",
				Detail: fmt.Sprintf("取值 %q 不在声明集合 [%s] 内", s, strings.Join(f.Values, " ")),
			})
		}

	case metamodel.TypeRef:
		if ck.RefExists != nil {
			if exists, err := ck.RefExists(f.Target, v.Data); err != nil {
				addV("ref", fmt.Sprintf("引用检查失败：%v", err))
			} else if !exists {
				addV("ref", fmt.Sprintf("引用目标 %s=%v 不存在", f.Target, v.Data))
			}
		}

	case metamodel.TypeList:
		items, ok := v.Data.([]any)
		if !ok {
			addV("type", fmt.Sprintf("期望列表，实际 %T", v.Data))
			return
		}
		if f.Items != nil {
			for i, it := range items {
				sub := Record{Entity: rec.Entity, Values: map[string]value.Value{f.Items.Key: value.Of(it)}}
				inner := Report{}
				checkField(entity, sub, f.Items, fmt.Sprintf("%s[%d]", path, i), value.Of(it), ck, &inner)
				rep.Violations = append(rep.Violations, inner.Violations...)
				rep.Markers = append(rep.Markers, inner.Markers...)
			}
		}

	case metamodel.TypeObject:
		m, ok := v.Data.(map[string]any)
		if !ok {
			addV("type", fmt.Sprintf("期望对象，实际 %T", v.Data))
			return
		}
		for i := range f.Fields {
			sub := &f.Fields[i]
			sv, ok := m[sub.Key]
			if !ok {
				checkField(entity, rec, sub, path+"."+sub.Key, value.AbsentValue(), ck, rep)
				continue
			}
			checkField(entity, rec, sub, path+"."+sub.Key, value.Of(sv), ck, rep)
		}
	}

	// 5. 唯一——适用于任意类型，因此放在类型分支之外
	if f.Unique && ck.UniqueExists != nil {
		if exists, err := ck.UniqueExists(entity.Name, f.Key, v.Data); err != nil {
			addV("unique", fmt.Sprintf("唯一性检查失败：%v", err))
		} else if exists {
			addV("unique", fmt.Sprintf("取值 %v 已存在", v.Data))
		}
	}
}

func recID(e *metamodel.Entity, rec Record) string {
	if id := e.Identity(); id != nil {
		if v, ok := rec.Values[id.Key]; ok && v.IsPresent() {
			return fmt.Sprintf("%s:%v", e.Name, v.Data)
		}
	}
	return e.Name + ":?"
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
