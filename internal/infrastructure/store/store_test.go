package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("打开库失败：%v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func sample(subject, predicate string, v value.Value) assertion.Assertion {
	return assertion.Assertion{
		ID:         "id-" + subject + "-" + predicate + "-" + v.String(),
		Entity:     "shikigami",
		Subject:    subject,
		Predicate:  predicate,
		Value:      v,
		Source:     assertion.Source{Name: "test"},
		Provenance: assertion.Provenance{Artifact: "a.json", Anchor: "x", Revision: "1", CapturedAt: time.Now()},
		Confidence: assertion.L1,
		Status:     assertion.StatusPending,
	}
}

func TestApplyIsAtomicAndIdempotent(t *testing.T) {
	st := openTemp(t)
	cs := assertion.ChangeSet{Insert: []assertion.Assertion{
		sample("262", "atk", value.OfUnit(float64(3082), "point")),
		sample("262", "name", value.Of("姑获鸟")),
	}}
	r1, err := st.Apply(cs)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Inserted != 2 {
		t.Fatalf("首次应用应新增 2，实际 %d", r1.Inserted)
	}
	// 重复应用必须幂等
	r2, err := st.Apply(cs)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Inserted != 0 || r2.Duplicated != 2 {
		t.Errorf("重复应用应为 0 新增 / 2 重复，实际 %d/%d", r2.Inserted, r2.Duplicated)
	}
	n, _ := st.Count()
	if n != 2 {
		t.Errorf("断言总数应保持 2，实际 %d", n)
	}
}

// 冲突断言必须**并存并标记**，不得覆盖 —— 系统不替用户选。
func TestConflictingAssertionsCoexist(t *testing.T) {
	st := openTemp(t)

	a := sample("262", "atk", value.OfUnit(float64(3082), "point"))
	b := sample("262", "atk", value.OfUnit(float64(3100), "point"))

	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{a}}); err != nil {
		t.Fatal(err)
	}
	res, err := st.Apply(assertion.ChangeSet{
		Insert:   []assertion.Assertion{b},
		Conflict: []string{b.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflicted != 1 {
		t.Errorf("应标记 1 条冲突，实际 %d", res.Conflicted)
	}

	as, err := st.BySubject("shikigami", "262")
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != 2 {
		t.Fatalf("两个取值的断言都应保留，实际 %d 条", len(as))
	}
	statuses := map[assertion.Status]int{}
	for _, x := range as {
		statuses[x.Status]++
	}
	if statuses[assertion.StatusDisputed] != 1 {
		t.Errorf("应恰好有 1 条标记为 disputed，实际 %v", statuses)
	}
	if statuses[assertion.StatusPending] != 1 {
		t.Errorf("原断言应保持 pending，实际 %v", statuses)
	}
}

func TestApplyRejectsInvalidAssertion(t *testing.T) {
	st := openTemp(t)
	bad := sample("262", "atk", value.OfUnit(float64(1), "point"))
	bad.Source = assertion.Source{} // 缺来源
	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{bad}}); err == nil {
		t.Error("缺来源的断言必须被拒绝")
	}
	n, _ := st.Count()
	if n != 0 {
		t.Errorf("拒绝后不应有残留，实际 %d", n)
	}
}

func TestUniqueAndRefCheckers(t *testing.T) {
	st := openTemp(t)
	a := sample("262", "name", value.Of("姑获鸟"))
	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{a}}); err != nil {
		t.Fatal(err)
	}
	exists, err := st.UniqueExists("shikigami", "name", "姑获鸟")
	if err != nil || !exists {
		t.Errorf("唯一性检查应报告已存在（err=%v）", err)
	}
	exists, _ = st.UniqueExists("shikigami", "name", "雪女")
	if exists {
		t.Error("不存在的取值不应报告已存在")
	}

	if _, err := st.Apply(assertion.ChangeSet{Insert: []assertion.Assertion{sample("1", "id", value.OfUnit(float64(1), "point"))}}); err != nil {
		t.Fatal(err)
	}
	ok, _ := st.RefExists("shikigami", float64(1))
	if !ok {
		t.Error("引用检查应找到已存在的身份值")
	}
	ok, _ = st.RefExists("shikigami", float64(999))
	if ok {
		t.Error("不存在的身份值不应被找到")
	}
}
