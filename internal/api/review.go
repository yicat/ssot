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

	"github.com/ngnl5/ssot/internal/application/review"
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

// QueueItem 是带优先级的队列项。
//
// 除了断言本身，它还带上「为什么排在前面」——排在前面的理由必须可见，
// 否则人只能盲信排序。
type QueueItem struct {
	Item    Item   `json:"item"`
	Tier    string `json:"tier"`
	Reason  string `json:"reason"`
	Score   int    `json:"score"`
	Sampled bool   `json:"sampled"`
}

// ConflictGroup 是一组互相冲突的断言。
type ConflictGroup struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Claims    []Item `json:"claims"`
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

// FilterInput 是批量筛选条件（面向界面）。
type FilterInput struct {
	Entity     string `json:"entity"`
	Status     string `json:"status"`
	Predicate  string `json:"predicate"`
	Artifact   string `json:"artifact"`
	Revision   string `json:"revision"`
	Confidence string `json:"confidence"`
	Subject    string `json:"subject"`
	Limit      int    `json:"limit"`
}

func (f FilterInput) toDomain() assertion.Filter {
	return assertion.Filter{
		Entity: f.Entity, Status: f.Status, Predicate: f.Predicate,
		Artifact: f.Artifact, Revision: f.Revision,
		Confidence: f.Confidence, Subject: f.Subject, Limit: f.Limit,
	}
}

// BatchPreview 是批量操作的预览结果。
type BatchPreview struct {
	Matched int    `json:"matched"`
	Where   string `json:"where"`
	Sample  []Item `json:"sample"`
}

// BatchResult 是批量操作的结果。
type BatchResult struct {
	Matched int    `json:"matched"`
	Applied int    `json:"applied"`
	Failed  int    `json:"failed"`
	FirstErr string `json:"firstErr"`
}

// Stats 是库的整体状态。
type Stats struct {
	Total         int            `json:"total"`
	ByStatus      map[string]int `json:"byStatus"`
	ByEntity      map[string]int `json:"byEntity"`
	ByConfidence  map[string]int `json:"byConfidence"`
	Verifications int            `json:"verifications"`
	Conflicts     int            `json:"conflicts"`
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

// Pending 返回待核验队列（不排序，按主体谓词）。
func (s *ReviewService) Pending(entity, status string, limit int) ([]Item, error) {
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = string(assertion.StatusPending)
	}
	as, err := p.Store.Select(assertion.Filter{Entity: entity, Status: status, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(as))
	for _, a := range as {
		out = append(out, toItem(a))
	}
	return out, nil
}

// Queue 返回**按优先级排序**的核验队列，并强制包含抽检项。
func (s *ReviewService) Queue(entity, status string, limit int, sampleRatio float64) ([]QueueItem, error) {
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = string(assertion.StatusPending)
	}
	items, err := review.Queue(p.Store,
		assertion.Filter{Entity: entity, Status: status}, limit, sampleRatio, 1)
	if err != nil {
		return nil, err
	}
	out := make([]QueueItem, 0, len(items))
	for _, it := range items {
		out = append(out, QueueItem{
			Item:    toItem(it.Assertion),
			Tier:    string(it.Priority.Tier),
			Reason:  it.Priority.Reason,
			Score:   it.Priority.Score,
			Sampled: it.Priority.Sampled,
		})
	}
	return out, nil
}

// Conflicts 返回冲突分组。**系统不裁决**，只把同一件事的说法摆在一起。
func (s *ReviewService) Conflicts() ([]ConflictGroup, error) {
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	groups, err := review.Conflicts(p.Store)
	if err != nil {
		return nil, err
	}
	out := make([]ConflictGroup, 0, len(groups))
	for _, g := range groups {
		claims := make([]Item, 0, len(g.Claims))
		for _, a := range g.Claims {
			claims = append(claims, toItem(a))
		}
		out = append(out, ConflictGroup{Subject: g.Subject, Predicate: g.Predicate, Claims: claims})
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
	cg, err := p.Store.Conflicts()
	if err != nil {
		return st, err
	}
	st.Conflicts = len(cg)
	return st, nil
}

// BatchPreview 展示批量操作会选中什么——**执行前必须能看见影响面**。
func (s *ReviewService) BatchPreview(f FilterInput) (BatchPreview, error) {
	p, err := s.project()
	if err != nil {
		return BatchPreview{}, err
	}
	df := f.toDomain()
	as, err := review.Preview(p.Store, df)
	if err != nil {
		return BatchPreview{}, err
	}
	if df.Status == "" {
		df.Status = string(assertion.StatusPending)
	}
	sample := make([]Item, 0, 5)
	for i, a := range as {
		if i >= 5 {
			break
		}
		sample = append(sample, toItem(a))
	}
	return BatchPreview{Matched: len(as), Where: df.Describe(), Sample: sample}, nil
}

// BatchApprove 批量批准。每条断言各自留下核验记录，审计轨迹不合并。
func (s *ReviewService) BatchApprove(f FilterInput, by, method, reason, evidence string) (BatchResult, error) {
	return s.batch(f, verification.Approved, by, method, reason, evidence)
}

// BatchReject 批量驳回。
func (s *ReviewService) BatchReject(f FilterInput, by, reason string) (BatchResult, error) {
	return s.batch(f, verification.Rejected, by, "editorial", reason, "")
}

func (s *ReviewService) batch(f FilterInput, dec verification.Decision, by, method, reason, evidence string) (BatchResult, error) {
	p, err := s.project()
	if err != nil {
		return BatchResult{}, err
	}
	if method == "" {
		method = "editorial"
	}
	res, err := review.Batch(p.Store, p.Store, review.BatchInput{
		Filter:   f.toDomain(),
		Decision: dec,
		Method:   verification.Method(method),
		By:       by,
		Reason:   reason,
		Evidence: evidence,
		Proposed: "ingest-pipeline",
	}, time.Now().UTC())
	out := BatchResult{Matched: res.Matched, Applied: res.Applied, Failed: res.Failed}
	if res.FirstErr != nil {
		out.FirstErr = res.FirstErr.Error()
	}
	return out, err
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
