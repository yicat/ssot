package admit

import (
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/application/ingest"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/metamodel"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/value"
)

func ptr[T any](v T) *T { return &v }

func testSchema(t *testing.T) *schema.Set {
	t.Helper()
	e := &metamodel.Entity{
		Name:        "shikigami",
		MetaVersion: metamodel.Version,
		Fields: []metamodel.Field{
			{Key: "id", Type: metamodel.TypeNumber, Required: true, Identity: true, Unit: "point", Precision: ptr(0)},
			{Key: "name", Type: metamodel.TypeText, Required: true},
			{Key: "atk", Type: metamodel.TypeNumber, Required: true, Unit: "point"},
		},
	}
	set, ps := schema.NewSet(e)
	if len(ps) > 0 {
		t.Fatalf("构造 schema 失败：%v", ps)
	}
	return set
}

func cand(subject, predicate string, v value.Value, p ingest.Parsing) ingest.Candidate {
	return ingest.Candidate{
		Entity: "shikigami", Subject: subject, Predicate: predicate, Value: v,
		Artifact: "a.json", Anchor: "x", Revision: "1", Parsing: p,
	}
}

func opts() Options {
	return Options{
		Entity:     "shikigami",
		Source:     assertion.Source{Name: "test"},
		CapturedAt: time.Now(),
		Units:      unit.Default(),
	}
}

func TestAdmitAcceptsValidCandidates(t *testing.T) {
	cands := []ingest.Candidate{
		cand("262", "id", value.OfUnit(float64(262), "point"), ingest.ParsingDirect),
		cand("262", "name", value.Of("姑获鸟"), ingest.ParsingDirect),
		cand("262", "atk", value.OfUnit(float64(3082), "point"), ingest.ParsingDirect),
	}
	cs, rep, err := Run(cands, testSchema(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Accepted != 3 || rep.Rejected != 0 {
		t.Errorf("应接受 3 条，实际 接受 %d 拒绝 %d", rep.Accepted, rep.Rejected)
	}
	for _, a := range cs.Insert {
		if a.Confidence != assertion.L1 {
			t.Errorf("结构化直取应为 L1，实际 %s", a.Confidence)
		}
		if a.Status != assertion.StatusPending {
			t.Errorf("初始状态必须是待核验，实际 %s", a.Status)
		}
	}
}

// 回退解析**不得定为 L1** —— 这是分级规则的核心。
func TestFallbackParsingIsNotL1(t *testing.T) {
	cands := []ingest.Candidate{
		cand("262", "id", value.OfUnit(float64(262), "point"), ingest.ParsingFallback),
		cand("262", "name", value.Of("姑获鸟"), ingest.ParsingText),
		cand("262", "atk", value.OfUnit(float64(1), "point"), ingest.ParsingDirect),
	}
	cs, _, err := Run(cands, testSchema(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]assertion.Confidence{}
	for _, a := range cs.Insert {
		got[a.Predicate] = a.Confidence
	}
	if got["id"] == assertion.L1 {
		t.Error("回退解析不得定为 L1")
	}
	if got["name"] != assertion.L2 {
		t.Errorf("文本解析应为 L2，实际 %s", got["name"])
	}
}

func TestRequiredMissingRejectsRecord(t *testing.T) {
	cands := []ingest.Candidate{
		cand("262", "id", value.OfUnit(float64(262), "point"), ingest.ParsingDirect),
		cand("262", "name", value.Of("姑获鸟"), ingest.ParsingDirect),
		// 缺 atk
	}
	cs, rep, err := Run(cands, testSchema(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Rejected != 1 {
		t.Errorf("缺必填字段应拒绝该记录，实际 %d", rep.Rejected)
	}
	if len(cs.Insert) != 0 {
		t.Errorf("被拒记录不应产生任何断言，实际 %d 条", len(cs.Insert))
	}
	if len(rep.Problems) == 0 || !strings.Contains(rep.Problems[0].Reason, "required") {
		t.Errorf("拒绝原因应指出 required，实际 %v", rep.Problems)
	}
}

// 与既有断言身份相同而取值不同 -> 冲突，必须并存并标记。
func TestConflictDetectedAgainstExisting(t *testing.T) {
	cands := []ingest.Candidate{
		cand("262", "id", value.OfUnit(float64(262), "point"), ingest.ParsingDirect),
		cand("262", "name", value.Of("姑获鸟"), ingest.ParsingDirect),
		cand("262", "atk", value.OfUnit(float64(3100), "point"), ingest.ParsingDirect),
	}
	// 既有库中同一身份已有不同取值
	existingAssertion := cand("262", "atk", value.OfUnit(float64(3082), "point"), ingest.ParsingDirect)
	key := assertion.Assertion{Entity: "shikigami", Subject: "262", Predicate: "atk"}.Key()
	existing := map[string][]string{key: {assertion.ValueJSON(existingAssertion.Value)}}

	cs, rep, err := Run(cands, testSchema(t), existing, opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conflicts != 1 {
		t.Errorf("应检出 1 条冲突，实际 %d", rep.Conflicts)
	}
	if len(cs.Conflict) != 1 {
		t.Errorf("变更集应带 1 条冲突标记，实际 %d", len(cs.Conflict))
	}
	if rep.Accepted != 3 {
		t.Errorf("冲突断言仍应被接受（并存），实际接受 %d", rep.Accepted)
	}
}

func TestUnchangedValueIsNotConflict(t *testing.T) {
	cands := []ingest.Candidate{
		cand("262", "id", value.OfUnit(float64(262), "point"), ingest.ParsingDirect),
		cand("262", "name", value.Of("姑获鸟"), ingest.ParsingDirect),
		cand("262", "atk", value.OfUnit(float64(3082), "point"), ingest.ParsingDirect),
	}
	key := assertion.Assertion{Entity: "shikigami", Subject: "262", Predicate: "atk"}.Key()
	existing := map[string][]string{key: {assertion.ValueJSON(value.OfUnit(float64(3082), "point"))}}

	_, rep, err := Run(cands, testSchema(t), existing, opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conflicts != 0 {
		t.Errorf("取值未变不应判为冲突，实际 %d", rep.Conflicts)
	}
}

func TestUndeclaredPredicateIsReportedNotBlocking(t *testing.T) {
	cands := []ingest.Candidate{
		cand("262", "id", value.OfUnit(float64(262), "point"), ingest.ParsingDirect),
		cand("262", "name", value.Of("姑获鸟"), ingest.ParsingDirect),
		cand("262", "atk", value.OfUnit(float64(1), "point"), ingest.ParsingDirect),
		cand("262", "voice", value.Of("行成飞亚"), ingest.ParsingDirect), // schema 未声明
	}
	cs, rep, err := Run(cands, testSchema(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Undeclared) != 1 || rep.Undeclared[0] != "voice" {
		t.Errorf("应报告未声明谓词 voice，实际 %v", rep.Undeclared)
	}
	for _, a := range cs.Insert {
		if a.Predicate == "voice" {
			t.Error("未声明谓词不应被写入")
		}
	}
}

func TestUnknownEntityFails(t *testing.T) {
	o := opts()
	o.Entity = "nope"
	if _, _, err := Run(nil, testSchema(t), nil, o); err == nil {
		t.Error("未知实体应报错")
	}
}
