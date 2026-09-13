package decision

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

var at = time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

func cand(pct float64, anchor, ctx string) Candidate {
	return Candidate{
		Value:   value.OfUnit(pct, "percent"),
		Anchor:  anchor,
		Context: ctx,
		Parsing: assertion.ParsingText,
	}
}

func item(t *testing.T, cands ...Candidate) Item {
	t.Helper()
	it, err := New("skill", "262_03", "ratio", "描述中出现 2 个伤害倍率（33%, 88%）",
		"造成攻击33%伤害，若目标…则造成攻击88%伤害",
		"Data:Character/262.json", "revid:8112", "huijiwiki", at, cands)
	if err != nil {
		t.Fatalf("构造事项失败：%v", err)
	}
	return it
}

func human(id string) verification.Actor { return verification.Actor{Kind: verification.Human, ID: id} }

func res(choice int, v value.Value) Resolution {
	return Resolution{
		Choice: choice, ChosenValue: v, By: human("ngnl5"),
		Reason: "按主伤害那一句取", Method: verification.Editorial, At: at,
	}
}

// 验收：抽出 0 个或 1 个取值时不产出待判定事项。
func TestNewRejectsNonAmbiguity(t *testing.T) {
	if _, err := New("skill", "s", "ratio", "r", "c", "a.json", "rev", "wiki", at, nil); err == nil {
		t.Error("零候选应当被拒绝：没有候选不是歧义")
	} else if !strings.Contains(err.Error(), "单候选") && !strings.Contains(err.Error(), "0 个候选") {
		t.Errorf("零候选的报错没说清原因：%v", err)
	}
	if _, err := New("skill", "s", "ratio", "r", "c", "a.json", "rev", "wiki", at,
		[]Candidate{cand(33, "x", "…33%…")}); err == nil {
		t.Error("单候选应当被拒绝：单候选不是歧义，应直接走准入")
	}
}

// 验收：候选值重复时去重；去重后不足 2 个则不产生事项。
func TestNewDedupesCandidates(t *testing.T) {
	it := item(t, cand(88, "a", "…88%…"), cand(88, "b", "…88%…"), cand(33, "c", "…33%…"))
	if len(it.Candidates) != 2 {
		t.Fatalf("去重后应为 2 个候选，实际 %d", len(it.Candidates))
	}
	if it.Candidates[0].Anchor != "a" {
		t.Errorf("去重应保留首次出现的候选，实际锚点 %s", it.Candidates[0].Anchor)
	}
	if _, err := New("skill", "s", "ratio", "r", "c", "a.json", "rev", "wiki", at,
		[]Candidate{cand(88, "a", "…88%…"), cand(88, "b", "…88%…")}); err == nil {
		t.Error("去重后只剩一个候选时应当被拒绝")
	}
}

// 验收：每个候选带上下文片段，人不打开原文也能判断。
func TestCandidateRequiresAnchorAndContext(t *testing.T) {
	bad := cand(33, "", "…33%…")
	if err := bad.Validate(); err == nil {
		t.Error("缺少锚点的候选应当被拒绝")
	}
	bad2 := cand(33, "a", "")
	if err := bad2.Validate(); err == nil {
		t.Error("缺少上下文的候选应当被拒绝——只给一个数字人无法判断")
	}
}

// 验收：agent 无法裁决。
func TestAgentCannotResolve(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	r := res(1, value.OfUnit(88, "percent"))
	r.By = verification.Actor{Kind: verification.Agent, ID: "ingest-pipeline"}
	if _, err := it.Resolve(r); err == nil {
		t.Fatal("agent 裁决必须被拒绝：agent 可以提出候选，但不能自己决定什么算数")
	}
}

// 验收：裁决人缺失时被拒绝。
func TestResolveRequiresHumanIdentity(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	r := res(1, value.OfUnit(88, "percent"))
	r.By = verification.Actor{}
	if _, err := it.Resolve(r); err == nil {
		t.Error("未标识裁决人时必须拒绝")
	}
}

// 验收：裁决理由必填。
func TestResolveRequiresReason(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	r := res(1, value.OfUnit(88, "percent"))
	r.Reason = ""
	if _, err := it.Resolve(r); err == nil {
		t.Error("没有理由的裁决必须被拒绝")
	}
}

// 验收：裁决索引越界被拒绝。
func TestResolveRejectsOutOfRange(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	for _, choice := range []int{-2, 2, 99} {
		r := res(choice, value.OfUnit(88, "percent"))
		if _, err := it.Resolve(r); err == nil {
			t.Errorf("候选序号 %d 越界，应当被拒绝", choice)
		}
	}
}

// 验收：裁决取值与所选候选不一致时被拒绝（不得凭序号与取值两套口径）。
func TestResolveRejectsValueMismatch(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	r := res(1, value.OfUnit(33, "percent"))
	if _, err := it.Resolve(r); err == nil {
		t.Error("选了第 2 个候选却报第 1 个候选的取值，必须被拒绝")
	}
}

// 验收：选中候选 → 已裁决，且未选中的候选仍然保留。
func TestResolveKeepsAllCandidates(t *testing.T) {
	it := item(t, cand(33, "a", "第一段"), cand(88, "b", "第二段"))
	got, err := it.Resolve(res(1, value.OfUnit(88, "percent")))
	if err != nil {
		t.Fatalf("裁决失败：%v", err)
	}
	if got.Status != StatusDecided {
		t.Errorf("状态应为已裁决，实际 %s", got.Status)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("裁决后候选不得被裁掉：期望 2 个，实际 %d", len(got.Candidates))
	}
	if got.Candidates[0].Context != "第一段" {
		t.Error("未选中的候选连同上下文都必须保留——「当时还有哪些说法」是可追溯性的一部分")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("裁决后的事项应当满足不变量：%v", err)
	}
}

// 验收：选「都不对」时不产生断言，谓词记为缺失。
func TestResolveNoneMakesNoAssertion(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	r := res(None, value.Value{})
	r.Reason = "两处都是条件分支里的数值，主伤害那一段没写倍率"
	got, err := it.Resolve(r)
	if err != nil {
		t.Fatalf("判「都不对」应当被接受：%v", err)
	}
	if !got.Resolution.IsNone() {
		t.Error("应记为「都不对」")
	}
	if got.Status != StatusDecided {
		t.Errorf("「都不对」是结论，状态应为已裁决，实际 %s", got.Status)
	}
	if got.HasChosen() {
		t.Error("判「都不对」时不得认为自己选中了候选")
	}
	if err := got.BindAssertion("a123"); err == nil {
		t.Error("判「都不对」时绑定断言应当报错——它不该产出断言")
	}
}

// 验收：已裁决事项再次裁决被拒绝。
func TestResolveTwiceRejected(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	got, err := it.Resolve(res(1, value.OfUnit(88, "percent")))
	if err != nil {
		t.Fatalf("首次裁决失败：%v", err)
	}
	if _, err := got.Resolve(res(0, value.OfUnit(33, "percent"))); !errors.Is(err, ErrAlreadyDecided) {
		t.Errorf("二次裁决应当返回 ErrAlreadyDecided，实际 %v", err)
	}
}

// 验收：暂缓不产生断言，事项仍在队列中。
func TestDeferStaysInQueue(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	got, err := it.Defer(Deferral{By: human("ngnl5"), Reason: "等下个版本再看", At: at})
	if err != nil {
		t.Fatalf("暂缓失败：%v", err)
	}
	if got.Status != StatusDeferred {
		t.Errorf("状态应为已暂缓，实际 %s", got.Status)
	}
	if !got.Status.NeedsAttention() {
		t.Error("暂缓是「未处理」，必须仍出现在需要人看的队列里")
	}
	if got.Resolution != nil {
		t.Error("暂缓不得产生裁决记录")
	}
	if _, err := got.Resolve(res(1, value.OfUnit(88, "percent"))); err != nil {
		t.Errorf("已暂缓事项仍应可裁决：%v", err)
	}
}

// 验收：暂缓必须有理由与人。
func TestDeferRequiresHumanAndReason(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	if _, err := it.Defer(Deferral{By: human("h"), At: at}); err == nil {
		t.Error("暂缓缺少理由时必须拒绝")
	}
	if _, err := it.Defer(Deferral{
		By: verification.Actor{Kind: verification.Agent, ID: "bot"}, Reason: "r", At: at,
	}); err == nil {
		t.Error("agent 不得暂缓（那等于替人决定先不处理）")
	}
}

// 验收：重复接入同一批数据不改变已裁决事项的状态。
func TestRefreshIdempotentWhenRevisionUnchanged(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	dec, err := it.Resolve(res(1, value.OfUnit(88, "percent")))
	if err != nil {
		t.Fatalf("裁决失败：%v", err)
	}
	again := dec.Refresh(dec.Candidates, dec.Revision, dec.Reason, dec.Context)
	if again.Status != StatusDecided {
		t.Errorf("修订未变化时状态不得改变，实际 %s", again.Status)
	}
	if again.Resolution == nil || again.Resolution.AssertionID != dec.Resolution.AssertionID {
		t.Error("重复接入不得清掉裁决记录")
	}
}

// 验收：修订变化且原选项仍在 → 维持已裁决。
func TestRefreshKeepsDecidedWhenChosenStillPresent(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	dec, _ := it.Resolve(res(1, value.OfUnit(88, "percent")))
	next := dec.Refresh([]Candidate{
		cand(88, "a'", "新修订里仍是 88%"), cand(33, "b'", "…"), cand(99, "c'", "…"),
	}, "revid:9000", "描述中出现 3 个伤害倍率", "新上下文")
	if next.Status != StatusDecided {
		t.Errorf("原选项仍在候选中时应维持已裁决，实际 %s", next.Status)
	}
	if next.Revision != "revid:9000" {
		t.Error("修订标识应当更新")
	}
}

// 验收：修订变化且原选项消失 → 标记需复核，不得静默保留旧结论。
func TestRefreshMarksStaleWhenChosenGone(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	dec, _ := it.Resolve(res(1, value.OfUnit(88, "percent")))
	next := dec.Refresh([]Candidate{
		cand(33, "a'", "…"), cand(120, "c'", "…"),
	}, "revid:9000", "描述中出现 2 个伤害倍率", "新上下文")
	if next.Status != StatusStale {
		t.Fatalf("原选项消失时应标记需复核，实际 %s", next.Status)
	}
	if next.Resolution == nil {
		t.Error("需复核状态仍应保留旧裁决，否则无从知道当初选了什么")
	}
	if err := next.Validate(); err != nil {
		t.Errorf("需复核事项的不变量应自洽：%v", err)
	}
}

// 验收：判「都不对」后原文变化 → 重新打开。
func TestRefreshReopensNoneOnRevisionChange(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	r := res(None, value.Value{})
	dec, _ := it.Resolve(r)
	next := dec.Refresh([]Candidate{cand(33, "a'", "…"), cand(88, "b'", "…")},
		"revid:9000", "描述中出现 2 个伤害倍率", "新上下文")
	if next.Status != StatusOpen {
		t.Errorf("原文变了，「都不对」这个结论未必还成立，应重新打开，实际 %s", next.Status)
	}
	if next.Resolution != nil {
		t.Error("重新打开时不得留着旧裁决，否则状态与数据不一致")
	}
}

// 验收：暂缓事项在修订变化后回到待判定。
func TestRefreshReopensDeferred(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	def, _ := it.Defer(Deferral{By: human("h"), Reason: "先放着", At: at})
	next := def.Refresh([]Candidate{cand(33, "a'", "…"), cand(88, "b'", "…")},
		"revid:9000", "描述中出现 2 个伤害倍率", "新上下文")
	if next.Status != StatusOpen {
		t.Errorf("修订变化后暂缓应失效并重新打开，实际 %s", next.Status)
	}
	if next.Deferral != nil {
		t.Error("重新打开时不得留着暂缓记录")
	}
}

// 验收：需复核事项可再次裁决，旧结论保留为历史。
func TestResolveStaleKeepsHistory(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	dec, _ := it.Resolve(res(1, value.OfUnit(88, "percent")))
	stale := dec.Refresh([]Candidate{cand(33, "a'", "…"), cand(120, "c'", "…")},
		"revid:9000", "描述中出现 2 个伤害倍率", "新上下文")

	r2 := res(1, value.OfUnit(120, "percent"))
	r2.Reason = "新修订里主伤害改成 120%"
	next, err := stale.ResolveStale(r2)
	if err != nil {
		t.Fatalf("需复核事项应当可再次裁决：%v", err)
	}
	if next.Status != StatusDecided {
		t.Errorf("改判后状态应为已裁决，实际 %s", next.Status)
	}
	if len(next.History) != 1 {
		t.Fatalf("旧结论应进历史，实际历史 %d 条", len(next.History))
	}
	if next.History[0].ChosenValue.String() != "88 percent" {
		t.Errorf("历史里应保留旧结论 88 percent，实际 %s", next.History[0].ChosenValue.String())
	}
	if next.HasChosen() == false {
		t.Error("改判后应选中新候选")
	}
}

// 事项标识跨修订稳定，否则每次同步都会生成新事项，历史就断链了。
func TestItemIDStableAcrossRevision(t *testing.T) {
	a := ItemID("skill", "262_03", "ratio", "Data:Character/262.json")
	b := ItemID("skill", "262_03", "ratio", "Data:Character/262.json")
	if a != b {
		t.Error("同一 (实体, 主体, 谓词, 原件) 必须得到同一 ID")
	}
	if a == ItemID("skill", "262_03", "ratio_max", "Data:Character/262.json") {
		t.Error("不同谓词必须是不同事项")
	}
}

// 验收：裁决方法受限——在原文候选间取舍只有编审与实测两种正当依据。
func TestResolveRejectsInapplicableMethod(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	for _, m := range []verification.Method{verification.Recompute, verification.CrossSource} {
		r := res(1, value.OfUnit(88, "percent"))
		r.Method = m
		if _, err := it.Resolve(r); err == nil {
			t.Errorf("方法「%s」不适用于在原文候选间取舍，应当被拒绝", m.Label())
		}
	}
}

// 验收：修订未变化时状态不变，但候选内容以本次抽取为准。
//
// 原文没变而候选变了，只可能是抽取器改了判断——此时状态不用动，
// 但界面必须看到新的候选，否则人是在拿旧信息做决定。
func TestRefreshAdoptsNewCandidatesWithSameRevision(t *testing.T) {
	it := item(t, cand(33, "a", "旧上下文"), cand(88, "b", "旧上下文"))
	next := it.Refresh([]Candidate{
		cand(33, "a", "新上下文"), cand(88, "b", "新上下文"),
	}, it.Revision, "", "")
	if next.Status != StatusOpen {
		t.Errorf("修订未变化时状态不得改变，实际 %s", next.Status)
	}
	if next.Candidates[0].Context != "新上下文" {
		t.Errorf("候选内容应取本次抽取结果，实际 %q", next.Candidates[0].Context)
	}
	if err := next.Validate(); err != nil {
		t.Errorf("不变量应自洽：%v", err)
	}
}

// 抽取器改了判断、原选项不见了：即使原文修订没变，结论也必须复核。
func TestRefreshStaleWhenExtractorDropsChosenValue(t *testing.T) {
	it := item(t, cand(33, "a", "x"), cand(88, "b", "y"))
	dec, _ := it.Resolve(res(1, value.OfUnit(88, "percent")))
	next := dec.Refresh([]Candidate{cand(33, "a", "x"), cand(120, "c", "z")},
		dec.Revision, "", "")
	if next.Status != StatusStale {
		t.Errorf("原选项被抽取器去掉后必须复核，实际 %s", next.Status)
	}
	if err := next.Validate(); err != nil {
		t.Errorf("不变量应自洽：%v", err)
	}
}

// 验收：待判定队列按影响面排序，且排序确定可复现。
func TestOrderIsDeterministic(t *testing.T) {
	mk := func(subject string, n int) Item {
		cands := make([]Candidate, 0, n)
		for i := 0; i < n; i++ {
			cands = append(cands, cand(float64(10*(i+1)), "a", "c"))
		}
		it, err := New("skill", subject, "ratio", "r", "c", "a.json", "rev", "wiki", at, cands)
		if err != nil {
			t.Fatal(err)
		}
		return it
	}
	// b 影响面最大；a 与 c 影响面相同，a 候选更少应排前。
	items := []Item{mk("a", 2), mk("b", 2), mk("c", 3)}
	impact := map[string]int{"skill|a": 5, "skill|b": 50, "skill|c": 5}

	first := Order(items, impact, 0)
	second := Order(items, impact, 0)
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatal("两次排序结果必须相同——否则人无法判断上次看到哪了")
		}
	}
	if first[0].Subject != "b" {
		t.Errorf("影响面最大者应排最前，实际 %s", first[0].Subject)
	}
	if first[1].Subject != "a" || first[2].Subject != "c" {
		t.Errorf("影响面相同时候选少者优先，实际顺序 %s, %s", first[1].Subject, first[2].Subject)
	}
	if got := Order(items, impact, 2); len(got) != 2 {
		t.Errorf("limit 应生效，实际 %d 条", len(got))
	}
	if r := ImpactReason(first[0], impact); !strings.Contains(r, "50") {
		t.Errorf("排序理由必须可见，实际 %q", r)
	}
}
