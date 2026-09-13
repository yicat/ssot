package validate

import (
	"testing"

	"github.com/ngnl5/ssot/internal/domain/metamodel"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/value"
)

func ptr[T any](v T) *T { return &v }

func entity() *metamodel.Entity {
	return &metamodel.Entity{
		Name:        "shikigami",
		MetaVersion: metamodel.Version,
		Fields: []metamodel.Field{
			{Key: "id", Type: metamodel.TypeNumber, Required: true, Identity: true, Unit: "point", Precision: ptr(0)},
			{Key: "name", Type: metamodel.TypeText, Required: true},
			{Key: "rarity", Type: metamodel.TypeEnum, Required: true, Values: []string{"R", "SR", "SSR"}},
			{Key: "atk", Type: metamodel.TypeNumber, Required: true, Unit: "point", Min: ptr(0.0), Max: ptr(10000.0)},
			{Key: "owner", Type: metamodel.TypeRef, Target: "player"},
			{Key: "unique_code", Type: metamodel.TypeText, Unique: true},
		},
	}
}

func vals(kv map[string]value.Value) Record {
	return Record{Entity: "shikigami", Values: kv}
}

func check(t *testing.T, kv map[string]value.Value, ck Checkers) Report {
	t.Helper()
	return Check(entity(), vals(kv), ck)
}

func hasViolation(r Report, rule string) bool {
	for _, v := range r.Violations {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

func base() map[string]value.Value {
	return map[string]value.Value{
		"id":     value.OfUnit(float64(262), "point"),
		"name":   value.Of("姑获鸟"),
		"rarity": value.Of("SR"),
		"atk":    value.OfUnit(float64(3082), "point"),
	}
}

func TestValidRecordPasses(t *testing.T) {
	r := check(t, base(), Checkers{Units: unit.Default()})
	if !r.OK() {
		t.Fatalf("合法记录不应报错：%v", r.Violations)
	}
	if len(r.Markers) != 0 {
		t.Errorf("合法记录不应有标记：%v", r.Markers)
	}
}

func TestRequiredMissingIsViolation(t *testing.T) {
	m := base()
	delete(m, "atk")
	r := check(t, m, Checkers{Units: unit.Default()})
	if !hasViolation(r, "required") {
		t.Errorf("必填缺失应报 required，实际 %v", r.Violations)
	}
}

// 未知值满足 required —— 这正是「已知未知」存在的意义：
// 没有它，新增必填字段时既有数据只能靠编造值来满足约束。
func TestUnknownSatisfiesRequiredAndSkipsTypeRange(t *testing.T) {
	m := base()
	m["atk"] = value.UnknownValue()
	r := check(t, m, Checkers{Units: unit.Default()})
	if hasViolation(r, "required") {
		t.Error("未知值应满足 required")
	}
	if hasViolation(r, "type") || hasViolation(r, "range") {
		t.Errorf("未知值无从判断类型与范围，不应报错：%v", r.Violations)
	}
}

func TestNullSatisfiesRequired(t *testing.T) {
	m := base()
	m["owner"] = value.NullValue()
	r := check(t, m, Checkers{Units: unit.Default()})
	if hasViolation(r, "required") {
		t.Error("空值应满足 required")
	}
}

func TestTypeMismatch(t *testing.T) {
	m := base()
	m["name"] = value.Of(123)
	r := check(t, m, Checkers{Units: unit.Default()})
	if !hasViolation(r, "type") {
		t.Errorf("类型不符应报 type，实际 %v", r.Violations)
	}
}

func TestRangeViolation(t *testing.T) {
	m := base()
	m["atk"] = value.OfUnit(float64(99999), "point")
	r := check(t, m, Checkers{Units: unit.Default()})
	if !hasViolation(r, "range") {
		t.Errorf("越界应报 range，实际 %v", r.Violations)
	}
}

func TestPrecisionViolation(t *testing.T) {
	m := base()
	m["id"] = value.OfUnit(float64(1.5), "point")
	r := check(t, m, Checkers{Units: unit.Default()})
	if !hasViolation(r, "precision") {
		t.Errorf("整数精度违规应报 precision，实际 %v", r.Violations)
	}
}

// 最关键的语义分界：超出枚举**接受并标记**，不进 Violations。
func TestUnmodeledEnumValueIsAcceptedAndMarked(t *testing.T) {
	m := base()
	m["rarity"] = value.Of("UR") // 未声明的新品阶
	r := check(t, m, Checkers{Units: unit.Default()})
	if !r.OK() {
		t.Errorf("未建模枚举取值不应被拒绝：%v", r.Violations)
	}
	if len(r.Markers) != 1 || r.Markers[0].Kind != "unmodeled-enum-value" {
		t.Errorf("应产生一条未建模取值标记，实际 %v", r.Markers)
	}
}

func TestUnitMismatchIsViolation(t *testing.T) {
	m := base()
	m["atk"] = value.OfUnit(float64(3082), "day") // 量纲不对
	r := check(t, m, Checkers{Units: unit.Default()})
	if !hasViolation(r, "enum-unit") {
		t.Errorf("单位量纲不符应报错，实际 %v", r.Violations)
	}
}

// percent 与 point 不可相加的意义在这里体现：单位必须可换算。
func TestUnitPercentIsIncompatibleWithPoint(t *testing.T) {
	m := base()
	m["atk"] = value.OfUnit(float64(50), "percent")
	r := check(t, m, Checkers{Units: unit.Default()})
	if !hasViolation(r, "enum-unit") {
		t.Errorf("percent 与 point 量纲不同应报错，实际 %v", r.Violations)
	}
}

func TestUniqueViolation(t *testing.T) {
	m := base()
	m["unique_code"] = value.Of("dup")
	ck := Checkers{Units: unit.Default(), UniqueExists: func(entity, field string, v any) (bool, error) {
		return v == "dup", nil
	}}
	r := check(t, m, ck)
	if !hasViolation(r, "unique") {
		t.Errorf("唯一冲突应报 unique，实际 %v", r.Violations)
	}
}

func TestRefViolation(t *testing.T) {
	m := base()
	m["owner"] = value.Of(float64(999))
	ck := Checkers{Units: unit.Default(), RefExists: func(entity string, id any) (bool, error) {
		return false, nil
	}}
	r := check(t, m, ck)
	if !hasViolation(r, "ref") {
		t.Errorf("引用目标不存在应报 ref，实际 %v", r.Violations)
	}
}

func TestNestedObjectValidation(t *testing.T) {
	e := &metamodel.Entity{
		Name:        "x",
		MetaVersion: metamodel.Version,
		Fields: []metamodel.Field{
			{Key: "id", Type: metamodel.TypeNumber, Required: true, Identity: true, Unit: "point"},
			{Key: "stats", Type: metamodel.TypeObject, Fields: []metamodel.Field{
				{Key: "atk", Type: metamodel.TypeNumber, Required: true, Unit: "point"},
			}},
		},
	}
	rec := Record{Entity: "x", Values: map[string]value.Value{
		"id":    value.OfUnit(float64(1), "point"),
		"stats": value.Of(map[string]any{}), // 缺 atk
	}}
	r := Check(e, rec, Checkers{Units: unit.Default()})
	if !hasViolation(r, "required") {
		t.Errorf("嵌套必填缺失应被检出，实际 %v", r.Violations)
	}
	if len(r.Violations) > 0 && r.Violations[0].Field != "stats.atk" {
		t.Errorf("违规应带嵌套路径，实际 %q", r.Violations[0].Field)
	}
}
