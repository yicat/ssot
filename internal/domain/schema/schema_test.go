package schema

import (
	"testing"

	"github.com/ngnl5/ssot/internal/domain/metamodel"
)

func entity() *metamodel.Entity {
	return &metamodel.Entity{
		Name:        "shikigami",
		MetaVersion: metamodel.Version,
		Fields: []metamodel.Field{
			{Key: "id", Type: metamodel.TypeNumber, Required: true, Identity: true, Unit: "point"},
			{Key: "name", Type: metamodel.TypeText, Required: true},
			{Key: "rarity", Type: metamodel.TypeText},
			{Key: "voice", Type: metamodel.TypeText}, // schema 有，数据从无
		},
	}
}

func rec(keys ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// 实测场景：schema 声明 13 字段，数据实际出现 15 个，两个方向同时漂移。
func TestDetectDriftBothDirections(t *testing.T) {
	records := []map[string]bool{
		rec("id", "name", "rarity", "tags"),
		rec("id", "name", "rarity", "tags", "voice2"),
	}
	d := DetectDrift(entity(), records)

	if len(d.Undeclared) != 2 {
		t.Fatalf("应检出 2 个未声明字段，实际 %v", d.Undeclared)
	}
	if d.Undeclared[0] != "tags" || d.Undeclared[1] != "voice2" {
		t.Errorf("未声明字段应为 [tags voice2]，实际 %v", d.Undeclared)
	}
	if len(d.Unused) != 1 || d.Unused[0] != "voice" {
		t.Errorf("从未出现的声明字段应为 [voice]，实际 %v", d.Unused)
	}
	if !d.HasAny() {
		t.Error("存在漂移时 HasAny 应为 true")
	}
}

func TestFillRateCountsPresenceNotContent(t *testing.T) {
	records := []map[string]bool{
		rec("id", "name"),
		rec("id", "name", "rarity"),
	}
	d := DetectDrift(entity(), records)
	if got := d.FillRate["rarity"]; got != 0.5 {
		t.Errorf("rarity 填充率应为 0.5，实际 %v", got)
	}
	if got := d.FillRate["voice"]; got != 0 {
		t.Errorf("voice 填充率应为 0，实际 %v", got)
	}
	if d.Samples != 2 {
		t.Errorf("样本数应为 2，实际 %d", d.Samples)
	}
}

func TestEmptyRecordsProduceNoDrift(t *testing.T) {
	d := DetectDrift(entity(), nil)
	if d.HasAny() {
		t.Errorf("无记录时不应报漂移，实际 %+v", d)
	}
}

func TestSetRejectsDuplicateEntityNames(t *testing.T) {
	_, ps := NewSet(entity(), entity())
	if len(ps) == 0 {
		t.Error("实体名重复必须被拒绝")
	}
}

func TestSetLookup(t *testing.T) {
	s, ps := NewSet(entity())
	if len(ps) != 0 {
		t.Fatalf("构造失败：%v", ps)
	}
	if _, ok := s.Lookup("shikigami"); !ok {
		t.Error("应能查到已注册实体")
	}
	if _, ok := s.Lookup("nope"); ok {
		t.Error("不应查到未注册实体")
	}
}
