package assertion

import (
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/value"
)

func base() Assertion {
	return Assertion{
		ID:         "a1",
		Entity:     "shikigami",
		Subject:    "262",
		Predicate:  "atk",
		Value:      value.OfUnit(float64(3082), "point"),
		Source:     Source{Name: "huijiwiki", Tier: "semi-official"},
		Provenance: Provenance{Artifact: "Data:Attribute.json", Anchor: "attributes[0]", Revision: "9939", CapturedAt: time.Now()},
		Confidence: L1,
		Status:     StatusPending,
	}
}

func TestValidAssertionPasses(t *testing.T) {
	if err := base().Validate(); err != nil {
		t.Fatalf("完整断言不应报错：%v", err)
	}
}

func TestMissingSourceAndProvenanceRejected(t *testing.T) {
	a := base()
	a.Source = Source{}
	if err := a.Validate(); err == nil {
		t.Error("缺来源必须被拒绝")
	}

	b := base()
	b.Provenance = Provenance{}
	if err := b.Validate(); err == nil {
		t.Error("缺溯源必须被拒绝")
	}
}

func TestL3RequiresDerivation(t *testing.T) {
	a := base()
	a.Confidence = L3
	if err := a.Validate(); err == nil {
		t.Error("L3 断言没有推导链必须被拒绝")
	}
	a.Derived = &Derivation{Formula: "panel", Inputs: []string{"a1"}}
	if err := a.Validate(); err != nil {
		t.Errorf("带推导链的 L3 应通过：%v", err)
	}
}

// 核心：限定条件不同即为不同断言，互不覆盖。
func TestDifferentQualifiersAreDifferentAssertions(t *testing.T) {
	a := base()
	b := base()
	b.ID = "a2"
	b.Qualifiers = Qualifiers{Condition: "自身总攻击为友方非召唤物中最高时"}

	if a.SameKey(b) {
		t.Error("限定条件不同的断言不应被视为同一件事")
	}
	if a.Conflicting(b) {
		t.Error("限定条件不同不构成冲突")
	}
}

// 身份相同而取值不同 —— 冲突，必须并存并标记，不得择一。
func TestSameKeyDifferentValueConflicts(t *testing.T) {
	a := base()
	b := base()
	b.ID = "a2"
	b.Value = value.OfUnit(float64(3100), "point")

	if !a.SameKey(b) {
		t.Fatal("主体谓词限定条件全同应视为同一件事")
	}
	if !a.Conflicting(b) {
		t.Error("同一件事取值不同应判为冲突")
	}
}

func TestSameValueDoesNotConflict(t *testing.T) {
	a := base()
	b := base()
	b.ID = "a2"
	if a.Conflicting(b) {
		t.Error("取值相同不构成冲突")
	}
}

func TestConfidenceValidity(t *testing.T) {
	for _, c := range []Confidence{L1, L2, L3, L4} {
		if !c.Valid() {
			t.Errorf("%s 应为合法分级", c)
		}
	}
	if Confidence("L9").Valid() {
		t.Error("L9 不应是合法分级")
	}
}

func TestMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Assertion)
	}{
		{"缺主体", func(a *Assertion) { a.Subject = "" }},
		{"缺谓词", func(a *Assertion) { a.Predicate = "" }},
		{"缺实体", func(a *Assertion) { a.Entity = "" }},
		{"缺 ID", func(a *Assertion) { a.ID = "" }},
		{"分级非法", func(a *Assertion) { a.Confidence = "X" }},
	}
	for _, c := range cases {
		a := base()
		c.mutate(&a)
		if err := a.Validate(); err == nil {
			t.Errorf("%s 必须被拒绝", c.name)
		}
	}
}
