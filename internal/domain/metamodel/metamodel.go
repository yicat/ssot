// Package metamodel 定义通用引擎的类型与约束原语。
//
// 见 docs/specs/metamodel.spec.md。核心约束：**原语要少而正交**。
// 引擎不认识任何游戏概念——「鬼火」「技能倍率」全部由 schema 用这些原语表达。
//
// 本包只管 schema 自身的合法性（结构），不管数据的合法性（那是 validate 的事）。
package metamodel

import (
	"fmt"
	"strings"
)

// Version 是原语集合的版本。元模型变更时必须递增。
const Version = 1

// TypeName 是类型原语名。取自封闭集合。
type TypeName string

const (
	TypeText   TypeName = "text"
	TypeNumber TypeName = "number"
	TypeBool   TypeName = "bool"
	TypeEnum   TypeName = "enum"
	TypeRef    TypeName = "ref"
	TypeObject TypeName = "object"
	TypeList   TypeName = "list"
)

// Types 返回全部合法的类型原语名。
func Types() []TypeName {
	return []TypeName{TypeText, TypeNumber, TypeBool, TypeEnum, TypeRef, TypeObject, TypeList}
}

// Valid 报告类型名是否在封闭集合内。
func (t TypeName) Valid() bool {
	for _, x := range Types() {
		if t == x {
			return true
		}
	}
	return false
}

// UnitRegistry 报告单位是否已注册。用于校验数值字段的量纲声明。
type UnitRegistry interface {
	Known(name string) bool
}

// Field 是实体中的一个字段声明。
type Field struct {
	Key         string
	Description string
	Type        TypeName

	// 字段级约束
	Required  bool
	Identity  bool
	Unique    bool
	Min       *float64
	Max       *float64
	Precision *int

	// 按类型必需的附加声明
	Unit   string   // number 必填
	Values []string // enum
	Target string   // ref
	Items  *Field   // list
	Fields []Field  // object
}

// Entity 是一个实体类型的 schema。
type Entity struct {
	Name        string
	Description string
	MetaVersion int
	SchemaRev   string
	Fields      []Field
}

// Problem 是一条 schema 结构问题，带位置。
type Problem struct {
	Path   string
	Reason string
}

func (p Problem) String() string { return fmt.Sprintf("%s: %s", p.Path, p.Reason) }

// IdentityField 返回身份字段。无身份字段时返回 nil。
func (e *Entity) Identity() *Field {
	for i := range e.Fields {
		if e.Fields[i].Identity {
			return &e.Fields[i]
		}
	}
	return nil
}

// FieldByKey 按键查找字段（只查顶层）。
func (e *Entity) FieldByKey(key string) (*Field, bool) {
	for i := range e.Fields {
		if e.Fields[i].Key == key {
			return &e.Fields[i], true
		}
	}
	return nil, false
}

// Keys 返回顶层字段名。
func (e *Entity) Keys() []string {
	out := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		out = append(out, f.Key)
	}
	return out
}

// Validate 校验 schema 自身的合法性。
//
// 严格模式：未知类型名、缺必需声明一律失败并指出位置——
// schema 是我们自己写的，typo 必须被抓到。
// （与之相反，数据取值超出枚举应当接受并标记，那是 validate 的事。）
func (e *Entity) Validate(ur UnitRegistry) []Problem {
	var ps []Problem

	if strings.TrimSpace(e.Name) == "" {
		ps = append(ps, Problem{Path: "name", Reason: "实体名不能为空"})
	}
	if e.MetaVersion == 0 {
		ps = append(ps, Problem{Path: "metamodelVersion", Reason: "必须声明所要求的元模型版本"})
	} else if e.MetaVersion > Version {
		ps = append(ps, Problem{
			Path:   "metamodelVersion",
			Reason: fmt.Sprintf("声明的版本 %d 高于引擎支持的 %d，拒绝加载", e.MetaVersion, Version),
		})
	}
	if len(e.Fields) == 0 {
		ps = append(ps, Problem{Path: "fields", Reason: "实体必须至少有一个字段"})
	}

	seen := map[string]bool{}
	for i := range e.Fields {
		f := &e.Fields[i]
		path := fmt.Sprintf("fields[%d]", i)
		ps = append(ps, validateField(f, path, ur, seen)...)
	}

	if id := e.Identity(); id == nil {
		ps = append(ps, Problem{Path: "fields", Reason: "实体必须声明一个身份字段（identity: true）"})
	}

	return ps
}

func validateField(f *Field, path string, ur UnitRegistry, seen map[string]bool) []Problem {
	var ps []Problem
	fp := path
	if f.Key != "" {
		fp = path + "(" + f.Key + ")"
	}

	if strings.TrimSpace(f.Key) == "" {
		ps = append(ps, Problem{Path: path, Reason: "字段名不能为空"})
	} else if seen[f.Key] {
		ps = append(ps, Problem{Path: fp, Reason: "字段名重复"})
	} else {
		seen[f.Key] = true
	}

	if !f.Type.Valid() {
		ps = append(ps, Problem{
			Path:   fp,
			Reason: fmt.Sprintf("未知类型 %q（合法取值：%s）", f.Type, joinTypes()),
		})
		return ps
	}

	// 各类型必需的附加声明
	switch f.Type {
	case TypeNumber:
		if f.Unit == "" {
			ps = append(ps, Problem{Path: fp, Reason: "数值字段必须声明单位（unit）"})
		} else if ur != nil && !ur.Known(f.Unit) {
			ps = append(ps, Problem{Path: fp, Reason: fmt.Sprintf("未知单位 %q", f.Unit)})
		}
	case TypeEnum:
		if len(f.Values) == 0 {
			ps = append(ps, Problem{Path: fp, Reason: "枚举字段必须声明取值列表（values）"})
		}
	case TypeRef:
		if f.Target == "" {
			ps = append(ps, Problem{Path: fp, Reason: "引用字段必须声明目标实体（target）"})
		}
	case TypeList:
		if f.Items == nil {
			ps = append(ps, Problem{Path: fp, Reason: "列表字段必须声明元素类型（items）"})
		} else {
			itemSeen := map[string]bool{}
			ps = append(ps, validateField(f.Items, fp+".items", ur, itemSeen)...)
		}
	case TypeObject:
		if len(f.Fields) == 0 {
			ps = append(ps, Problem{Path: fp, Reason: "对象字段必须声明子字段（fields）"})
		}
		subSeen := map[string]bool{}
		for i := range f.Fields {
			ps = append(ps, validateField(&f.Fields[i], fmt.Sprintf("%s.fields[%d]", fp, i), ur, subSeen)...)
		}
	}

	// 身份约束
	if f.Identity {
		switch f.Type {
		case TypeObject, TypeList:
			ps = append(ps, Problem{Path: fp, Reason: "身份字段不能是对象或列表"})
		}
		if !f.Required {
			ps = append(ps, Problem{Path: fp, Reason: "身份字段必须同时声明为必填（required）"})
		}
	}

	return ps
}

func joinTypes() string {
	out := make([]string, 0, len(Types()))
	for _, t := range Types() {
		out = append(out, string(t))
	}
	return strings.Join(out, ", ")
}

// Label 返回类型的中文名。
//
// 界面上只写 `number` / `ref` 等于没写——定义是给人看的，
// 中文名与原始标识一起显示，才既看得懂又对得上数据。
func (t TypeName) Label() string {
	switch t {
	case TypeText:
		return "文本"
	case TypeNumber:
		return "数值"
	case TypeBool:
		return "布尔"
	case TypeEnum:
		return "枚举"
	case TypeRef:
		return "引用"
	case TypeObject:
		return "对象"
	case TypeList:
		return "列表"
	}
	return string(t)
}
