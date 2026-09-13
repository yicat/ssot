package store

import (
	"testing"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
)

// 影响面的数据来源：一条断言被多少条派生断言引用。
func TestReferenceCounts(t *testing.T) {
	st := openTemp(t)

	base := sample("262", "cri", value.OfUnit(float64(0.5), "fraction"))
	base2 := sample("262", "crid", value.OfUnit(float64(1.2), "fraction"))
	derived := sample("262", "crit_factor", value.OfUnit(float64(1.6), "fraction"))
	derived.ID = "derived-1"
	derived.Confidence = assertion.L3
	derived.Derived = &assertion.Derivation{
		Formula: "crit_factor", FormulaRev: "1",
		Inputs: []string{base.ID, base2.ID},
	}
	other := sample("200", "atk", value.OfUnit(float64(1), "point"))
	other.ID = "other-1"
	other.Derived = &assertion.Derivation{
		Formula: "x", Inputs: []string{base.ID}, // base 被两条派生引用
	}

	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{
		base, base2, derived, other,
	}}); err != nil {
		t.Fatal(err)
	}

	refs, err := st.ReferenceCounts()
	if err != nil {
		t.Fatal(err)
	}
	if refs[base.ID] != 2 {
		t.Errorf("base 应被引用 2 次，实际 %d", refs[base.ID])
	}
	if refs[base2.ID] != 1 {
		t.Errorf("base2 应被引用 1 次，实际 %d", refs[base2.ID])
	}
	if _, ok := refs[derived.ID]; ok {
		t.Error("派生断言自身不是别人的输入，不应出现在计数里")
	}
}

// 冲突分组：同一身份（主体+谓词+限定条件）而有不同取值。
func TestConflictsGroupsSameKeyDifferentValue(t *testing.T) {
	st := openTemp(t)

	a := sample("262", "atk", value.OfUnit(float64(3082), "point"))
	b := sample("262", "atk", value.OfUnit(float64(3100), "point"))
	// 限定条件不同 —— 不是冲突
	c := sample("262", "atk", value.OfUnit(float64(9999), "point"))
	c.Qualifiers = assertion.Qualifiers{Condition: "觉醒后"}
	// 无关的一条
	d := sample("262", "hp", value.OfUnit(float64(1), "point"))

	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{a, b, c, d}}); err != nil {
		t.Fatal(err)
	}

	groups, err := st.Conflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("应恰好有 1 组冲突，实际 %d 组", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("该组应有 2 条说法，实际 %d", len(groups[0]))
	}
	for _, x := range groups[0] {
		if x.Qualifiers.Condition != "" {
			t.Error("限定条件不同的断言不应被算作同一组冲突")
		}
	}
}

func TestConflictsEmptyWhenNone(t *testing.T) {
	st := openTemp(t)
	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{
		sample("262", "atk", value.OfUnit(float64(1), "point")),
		sample("262", "hp", value.OfUnit(float64(2), "point")),
	}}); err != nil {
		t.Fatal(err)
	}
	groups, err := st.Conflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("无冲突时应返回空，实际 %d 组", len(groups))
	}
}

func TestSelectFilters(t *testing.T) {
	st := openTemp(t)
	a := sample("262", "atk", value.OfUnit(float64(1), "point"))
	b := sample("262", "hp", value.OfUnit(float64(2), "point"))
	b.Confidence = assertion.L2
	c := sample("200", "atk", value.OfUnit(float64(3), "point"))
	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{a, b, c}}); err != nil {
		t.Fatal(err)
	}

	got, err := st.Select(Filter{Entity: "shikigami", Predicate: "atk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("实体+谓词筛选应得 2 条，实际 %d", len(got))
	}

	got, _ = st.Select(Filter{Confidence: "L2"})
	if len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("按分级筛选应得 b，实际 %d 条", len(got))
	}

	got, _ = st.Select(Filter{Subject: "262"})
	if len(got) != 2 {
		t.Errorf("按主体筛选应得 2 条，实际 %d", len(got))
	}

	got, _ = st.Select(Filter{Limit: 1})
	if len(got) != 1 {
		t.Errorf("limit 应生效，实际 %d 条", len(got))
	}
}

// 无条件的筛选必须是**显式可见**的——它一次会选中全部，不能让人以为是精确筛选。
func TestFilterDescribeWarnsWhenUnconditional(t *testing.T) {
	var f Filter
	if got := f.Describe(); got == "" {
		t.Error("无条件筛选必须有可见提示")
	}
	f = Filter{Entity: "skill", Predicate: "ratio"}
	d := f.Describe()
	if d != "实体=skill 且 谓词=ratio" {
		t.Errorf("筛选描述应可读，实际 %q", d)
	}
}
