// 待判定：面向界面的接口（见 docs/specs/decision.spec.md）。
//
// 本文件只做参数转换与调用转发，不写业务规则——规则在 domain/decision 与
// application/disambig 里，CLI 与 GUI 因此看到的是同一套行为。
package api

import (
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/application/admit"
	"github.com/ngnl5/ssot/internal/application/disambig"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// DecisionCandidate 是一个候选取值（面向界面）。
//
// 上下文必填：只给一个数字，人无法判断。
type DecisionCandidate struct {
	Index   int    `json:"index"`
	Value   string `json:"value"`
	Unit    string `json:"unit"`
	Anchor  string `json:"anchor"`
	Context string `json:"context"`
	Note    string `json:"note"`
}

// DecisionResolution 是一次裁决（面向界面）。
type DecisionResolution struct {
	Choice int `json:"choice"`
	// ChosenValue 在「都不对」时为空串。
	ChosenValue string `json:"chosenValue"`
	By          string `json:"by"`
	Reason      string `json:"reason"`
	Method      string `json:"method"`
	Evidence    string `json:"evidence"`
	At          string `json:"at"`
	// AssertionID 在「都不对」时为空串。
	AssertionID string `json:"assertionId"`
}

// DecisionItem 是一个待判定事项（面向界面）。
type DecisionItem struct {
	ID         string `json:"id"`
	Entity     string `json:"entity"`
	Subject    string `json:"subject"`
	Predicate  string `json:"predicate"`
	Reason     string `json:"reason"`
	Context    string `json:"context"`
	Artifact   string `json:"artifact"`
	Revision   string `json:"revision"`
	Source     string `json:"source"`
	Status     string `json:"status"`
	StatusText string `json:"statusText"`
	// Impact 是被引用次数，ImpactReason 是「为什么排在前面」。
	Impact       int                 `json:"impact"`
	ImpactReason string              `json:"impactReason"`
	Candidates   []DecisionCandidate `json:"candidates"`
	Resolution   *DecisionResolution `json:"resolution"`
	DeferredBy   string              `json:"deferredBy"`
	DeferReason  string              `json:"deferReason"`
	// Editable 报告该事项此刻是否还能裁决。它由状态算出，
	// 免得界面自己判断状态——那会让「能不能点」变成第二套规则。
	Editable bool `json:"editable"`
}

// DecisionResult 是一次裁决的结果。
type DecisionResult struct {
	Applied     bool   `json:"applied"`
	Status      string `json:"status"`
	AssertionID string `json:"assertionId"`
	Message     string `json:"message"`
}

// DecisionStats 是待判定的整体状态。
type DecisionStats struct {
	Open     int `json:"open"`
	Deferred int `json:"deferred"`
	Decided  int `json:"decided"`
	Missing  int `json:"missing"`
	Stale    int `json:"stale"`
}

func toDecisionItem(e disambig.Entry) DecisionItem {
	it := e.Item
	out := DecisionItem{
		ID: it.ID, Entity: it.Entity, Subject: it.Subject, Predicate: it.Predicate,
		Reason: it.Reason, Context: it.Context, Artifact: it.Artifact,
		Revision: it.Revision, Source: it.Source,
		Status: string(it.Status), StatusText: it.Status.Label(),
		Impact: e.Impact, ImpactReason: e.Reason,
		Editable: it.Status.NeedsAttention(),
	}
	for i, c := range it.Candidates {
		out.Candidates = append(out.Candidates, DecisionCandidate{
			Index: i, Value: fmt.Sprintf("%v", c.Value.Data), Unit: c.Value.Unit,
			Anchor: c.Anchor, Context: c.Context, Note: c.Note,
		})
	}
	if it.Resolution != nil {
		out.Resolution = &DecisionResolution{
			Choice:   it.Resolution.Choice,
			By:       it.Resolution.By.String(),
			Reason:   it.Resolution.Reason,
			Method:   it.Resolution.Method.Label(),
			Evidence: it.Resolution.Evidence,
			At:       it.Resolution.At.Format(time.RFC3339),
		}
		if !it.Resolution.IsNone() {
			out.Resolution.ChosenValue = fmt.Sprintf("%v %s",
				it.Resolution.ChosenValue.Data, it.Resolution.ChosenValue.Unit)
		}
		out.Resolution.AssertionID = it.Resolution.AssertionID
	}
	if it.Deferral != nil {
		out.DeferredBy = it.Deferral.By.String()
		out.DeferReason = it.Deferral.Reason
	}
	return out
}

// Decisions 返回待判定队列。
//
// status 为空表示「还需要人看的」：待判定 + 已暂缓 + 需复核。
// 暂缓不是结论，因此**仍在队列里**，只是换个标记。
func (s *ReviewService) Decisions(status string, limit int) ([]DecisionItem, error) {
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	entries, err := disambig.Queue(p.Store, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]DecisionItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, toDecisionItem(e))
	}
	return out, nil
}

// DecisionStats 返回各状态的计数。
func (s *ReviewService) DecisionStats() (DecisionStats, error) {
	p, err := s.project()
	if err != nil {
		return DecisionStats{}, err
	}
	st, err := disambig.Summary(p.Store)
	if err != nil {
		return DecisionStats{}, err
	}
	return DecisionStats{
		Open: st.Open, Deferred: st.Deferred, Decided: st.Decided,
		Missing: st.Missing, Stale: st.Stale,
	}, nil
}

// ResolveDecision 裁决一条待判定事项。
//
// choice >= 0 选中该序号候选；choice == -1 表示「都不对」。
// by 必须是人——agent 可以提出候选与取证，但不能自己决定什么算数。
func (s *ReviewService) ResolveDecision(id string, choice int, by, method, reason, evidence string) (DecisionResult, error) {
	p, err := s.project()
	if err != nil {
		return DecisionResult{}, err
	}
	if by == "" {
		return DecisionResult{}, fmt.Errorf("必须指明裁决人（必须是人）——无追责的裁决等于没有裁决")
	}
	if reason == "" {
		return DecisionResult{}, fmt.Errorf("必须说明理由——只有结论没有理由的不是裁决")
	}
	if method == "" {
		method = string(verification.Editorial)
	}
	res, err := disambig.Resolve(p.Store, p.Schema, admit.Options{
		Source: assertion.Source{Name: "huijiwiki", Tier: "semi-official"},
		Units:  p.Units, UniqueExists: p.Store.UniqueExists, RefExists: p.Store.RefExists,
	}, disambig.ResolveInput{
		ID:       id,
		Choice:   choice,
		By:       verification.Actor{Kind: verification.Human, ID: by},
		Method:   verification.Method(method),
		Reason:   reason,
		Evidence: evidence,
		Proposed: verification.Actor{Kind: verification.Agent, ID: "ingest-pipeline"},
	}, time.Now().UTC())
	if err != nil {
		return DecisionResult{}, err
	}
	return DecisionResult{
		Applied:     true,
		Status:      string(res.Item.Status),
		AssertionID: res.AssertionID,
		Message:     res.Message,
	}, nil
}

// DeferDecision 暂缓一条事项：不产生断言，它仍在待判定队列里。
func (s *ReviewService) DeferDecision(id, by, reason string) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if by == "" {
		return fmt.Errorf("必须指明暂缓人（必须是人）")
	}
	if reason == "" {
		return fmt.Errorf("必须说明理由——只说「先放着」不构成一条可追溯的记录")
	}
	_, err = disambig.Defer(p.Store, id,
		verification.Actor{Kind: verification.Human, ID: by}, reason, time.Now().UTC())
	return err
}
