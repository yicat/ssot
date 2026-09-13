package metamodel

import (
	"strings"
	"testing"
)

type fakeUnits map[string]bool

func (f fakeUnits) Known(n string) bool { return f[n] }

func ptr[T any](v T) *T { return &v }

func validEntity() *Entity {
	return &Entity{
		Name:        "shikigami",
		MetaVersion: Version,
		Fields: []Field{
			{Key: "id", Type: TypeNumber, Required: true, Identity: true, Unit: "point", Precision: ptr(0)},
			{Key: "name", Type: TypeText, Required: true},
			{Key: "rarity", Type: TypeEnum, Required: true, Values: []string{"R", "SR", "SSR", "SP"}},
			{Key: "atk", Type: TypeNumber, Required: true, Unit: "point"},
		},
	}
}

func TestValidEntityPasses(t *testing.T) {
	ps := validEntity().Validate(fakeUnits{"point": true})
	if len(ps) != 0 {
		t.Fatalf("合法 schema 不应报错，却得到：%v", ps)
	}
}

func TestUnknownTypeIsRejectedWithPosition(t *testing.T) {
	e := validEntity()
	e.Fields[1].Type = "stiring" // typo
	ps := e.Validate(fakeUnits{"point": true})
	if len(ps) == 0 {
		t.Fatal("未知类型名必须被拒绝")
	}
	found := false
	for _, p := range ps {
		if strings.Contains(p.Path, "fields[1]") && strings.Contains(p.Reason, "未知类型") {
			found = true
		}
	}
	if !found {
		t.Errorf("错误必须指出位置与原因，实际：%v", ps)
	}
}

func TestNumberRequiresUnit(t *testing.T) {
	e := validEntity()
	e.Fields[3].Unit = ""
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "必须声明单位") {
		t.Errorf("数值字段缺单位必须被拒绝，实际：%v", ps)
	}
}

func TestUnknownUnitIsRejected(t *testing.T) {
	e := validEntity()
	e.Fields[3].Unit = "wat"
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "未知单位") {
		t.Errorf("未注册的单位必须被拒绝，实际：%v", ps)
	}
}

func TestEnumRequiresValues(t *testing.T) {
	e := validEntity()
	e.Fields[2].Values = nil
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "取值列表") {
		t.Errorf("枚举缺取值列表必须被拒绝，实际：%v", ps)
	}
}

func TestRefRequiresTarget(t *testing.T) {
	e := validEntity()
	e.Fields = append(e.Fields, Field{Key: "owner", Type: TypeRef})
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "目标实体") {
		t.Errorf("引用缺目标必须被拒绝，实际：%v", ps)
	}
}

func TestListRequiresItems(t *testing.T) {
	e := validEntity()
	e.Fields = append(e.Fields, Field{Key: "tags", Type: TypeList})
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "元素类型") {
		t.Errorf("列表缺元素类型必须被拒绝，实际：%v", ps)
	}
}

func TestObjectRequiresFields(t *testing.T) {
	e := validEntity()
	e.Fields = append(e.Fields, Field{Key: "stats", Type: TypeObject})
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "子字段") {
		t.Errorf("对象缺子字段必须被拒绝，实际：%v", ps)
	}
}

func TestIdentityMustBeRequired(t *testing.T) {
	e := validEntity()
	e.Fields[0].Required = false
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "必须同时声明为必填") {
		t.Errorf("身份字段非必填必须被拒绝，实际：%v", ps)
	}
}

func TestIdentityCannotBeComposite(t *testing.T) {
	e := validEntity()
	e.Fields[0].Type = TypeList
	e.Fields[0].Items = &Field{Key: "x", Type: TypeText}
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "不能是对象或列表") {
		t.Errorf("身份字段为列表必须被拒绝，实际：%v", ps)
	}
}

func TestMissingIdentityIsRejected(t *testing.T) {
	e := validEntity()
	e.Fields[0].Identity = false
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "身份字段") {
		t.Errorf("缺身份字段必须被拒绝，实际：%v", ps)
	}
}

func TestDuplicateKeyIsRejected(t *testing.T) {
	e := validEntity()
	e.Fields[1].Key = "id"
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "重复") {
		t.Errorf("字段名重复必须被拒绝，实际：%v", ps)
	}
}

func TestMetamodelVersionTooHighIsRejected(t *testing.T) {
	e := validEntity()
	e.MetaVersion = Version + 1
	ps := e.Validate(fakeUnits{"point": true})
	if !hasReason(ps, "高于引擎支持") {
		t.Errorf("过高的元模型版本必须被拒绝，实际：%v", ps)
	}
}

func TestNestedObjectFieldsAreValidated(t *testing.T) {
	e := validEntity()
	e.Fields = append(e.Fields, Field{
		Key:  "stats",
		Type: TypeObject,
		Fields: []Field{
			{Key: "atk", Type: TypeNumber, Unit: "point"},
			{Key: "bad", Type: TypeNumber}, // 缺单位
		},
	})
	ps := e.Validate(fakeUnits{"point": true})
	found := false
	for _, p := range ps {
		if strings.Contains(p.Path, "stats") && strings.Contains(p.Reason, "必须声明单位") {
			found = true
		}
	}
	if !found {
		t.Errorf("嵌套字段的错误必须被检出并带路径，实际：%v", ps)
	}
}

func hasReason(ps []Problem, substr string) bool {
	for _, p := range ps {
		if strings.Contains(p.Reason, substr) {
			return true
		}
	}
	return false
}
