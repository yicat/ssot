// Package api 是接口层：把用例暴露给前端与外部调用方。
//
// 它只做参数转换与调用转发，**不写业务规则**（见 AGENTS.md 分层铁律）。
// 所有判断都在 domain 与 application 里——这样 CLI、GUI 与将来的 HTTP 接口
// 看到的是同一份规则，不会各自实现一套。
package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Item 是队列里的一条断言（面向界面的扁平结构）。
type Item struct {
	ID         string `json:"id"`
	Entity     string `json:"entity"`
	Subject    string `json:"subject"`
	Predicate  string `json:"predicate"`
	Value      string `json:"value"`
	Unit       string `json:"unit"`
	Confidence string `json:"confidence"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	Artifact   string `json:"artifact"`
	Anchor     string `json:"anchor"`
	Revision   string `json:"revision"`
}

// HistoryItem 是一条核验记录（面向界面）。
type HistoryItem struct {
	Decision   string `json:"decision"`
	Method     string `json:"method"`
	ProposedBy string `json:"proposedBy"`
	ApprovedBy string `json:"approvedBy"`
	Reason     string `json:"reason"`
	Evidence   string `json:"evidence"`
	At         string `json:"at"`
}

// Stats 是库的整体状态。
type Stats struct {
	Total        int            `json:"total"`
	ByStatus     map[string]int `json:"byStatus"`
	ByEntity     map[string]int `json:"byEntity"`
	ByConfidence map[string]int `json:"byConfidence"`
	Verifications int           `json:"verifications"`
}

// ReviewService 是核验工作台的后端。
type ReviewService struct {
	projectDir string

	mu sync.Mutex
	p  *compose.Project
}

// NewReviewService 构造服务。项目在首次使用时惰性加载。
func NewReviewService(projectDir string) *ReviewService {
	return &ReviewService{projectDir: projectDir}
}

func (s *ReviewService) project() (*compose.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p != nil {
		return s.p, nil
	}
	p, err := compose.Load(s.projectDir, true)
	if err != nil {
		return nil, err
	}
	s.p = p
	return p, nil
}

// ProjectDir 返回项目目录，供界面显示。
func (s *ReviewService) ProjectDir() string { return s.projectDir }

// close 释放底层资源。
//
// 刻意不导出：它是宿主生命周期方法，不该出现在给前端的绑定量里。
func (s *ReviewService) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return nil
	}
	err := s.p.Close()
	s.p = nil
	return err
}

func toItem(a assertion.Assertion) Item {
	return Item{
		ID: a.ID, Entity: a.Entity, Subject: a.Subject, Predicate: a.Predicate,
		Value: a.Value.String(), Unit: a.Value.Unit,
		Confidence: string(a.Confidence), Status: string(a.Status),
		Source: a.Source.Name, Artifact: a.Provenance.Artifact,
		Anchor: a.Provenance.Anchor, Revision: a.Provenance.Revision,
	}
}

// Pending 返回待核验队列。
func (s *ReviewService) Pending(entity, status string, limit int) ([]Item, error) {
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = string(assertion.StatusPending)
	}
	as, err := p.Store.PendingByEntity(entity, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(as))
	for _, a := range as {
		out = append(out, toItem(a))
	}
	return out, nil
}

// Stats 返回库的整体状态。
func (s *ReviewService) Stats() (Stats, error) {
	p, err := s.project()
	if err != nil {
		return Stats{}, err
	}
	all, err := p.Store.All()
	if err != nil {
		return Stats{}, err
	}
	st := Stats{
		ByStatus:     map[string]int{},
		ByEntity:     map[string]int{},
		ByConfidence: map[string]int{},
	}
	for _, a := range all {
		st.Total++
		st.ByStatus[string(a.Status)]++
		st.ByEntity[a.Entity]++
		st.ByConfidence[string(a.Confidence)]++
	}
	n, err := p.Store.VerificationCount()
	if err != nil {
		return st, err
	}
	st.Verifications = n
	return st, nil
}

// Approve 批准一条断言。
//
// by 必须是人——agent 可以提出与取证，但不能自己决定什么算数。
// 这条规则在领域层的 Validate 中强制，此处只做转发。
func (s *ReviewService) Approve(id, by, method, reason, evidence string) error {
	return s.decide(id, verification.Approved, by, method, reason, evidence)
}

// Reject 驳回一条断言。
func (s *ReviewService) Reject(id, by, reason string) error {
	return s.decide(id, verification.Rejected, by, "editorial", reason, "")
}

func (s *ReviewService) decide(id string, dec verification.Decision, by, method, reason, evidence string) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if by == "" {
		return fmt.Errorf("必须指明批准者（必须是人）——无追责的核验等于没有核验")
	}
	if reason == "" {
		return fmt.Errorf("必须说明理由——只有状态没有理由的不是核验")
	}
	if method == "" {
		method = "editorial"
	}
	return p.Store.Verify(verification.Record{
		AssertionID: id,
		Decision:    dec,
		Method:      verification.Method(method),
		ProposedBy:  verification.Actor{Kind: verification.Agent, ID: "ingest-pipeline"},
		ApprovedBy:  &verification.Actor{Kind: verification.Human, ID: by},
		Reason:      reason,
		Evidence:    evidence,
		At:          time.Now().UTC(),
	})
}

// History 返回某断言的核验历史。
func (s *ReviewService) History(id string) ([]HistoryItem, error) {
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	recs, err := p.Store.Verifications(id)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryItem, 0, len(recs))
	for _, r := range recs {
		by := ""
		if r.ApprovedBy != nil {
			by = r.ApprovedBy.String()
		}
		out = append(out, HistoryItem{
			Decision: string(r.Decision), Method: r.Method.Label(),
			ProposedBy: r.ProposedBy.String(), ApprovedBy: by,
			Reason: r.Reason, Evidence: r.Evidence,
			At: r.At.Format(time.RFC3339),
		})
	}
	return out, nil
}
