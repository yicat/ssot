// 文档的接口（见 docs/specs/document.spec.md）。
//
// 文档与断言并列，**不参与计算，只作为依据**。它的价值在于任何一条断言
// 被质疑时能回到原文。
package api

import (
	"fmt"
	"time"

	appdoc "github.com/ngnl5/ssot/internal/application/document"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/document"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// DocView 是一份文档（面向界面）。
type DocView struct {
	// RevID 标识这一份具体修订。
	RevID string `json:"revId"`
	// DocID 是文档身份，**跨修订稳定**。
	DocID string `json:"docId"`

	Source string `json:"source"`
	Title  string `json:"title"`
	// Kind 是类型：依据 / 规则 / 说明 / 变更 / 未建模。
	Kind     string `json:"kind"`
	KindText string `json:"kindText"`

	Revision string `json:"revision"`
	// Hash 是内容摘要，**由内容算出**——来源可以忘记更新修订号，摘要不会。
	Hash       string `json:"hash"`
	CapturedAt string `json:"capturedAt"`
	// RegisteredAt 是登记时间，与采集时间不是一回事。
	RegisteredAt string `json:"registeredAt"`

	// Body 是原文；镜像文档为空串（归档里有）。
	Body string `json:"body"`
	// ArtifactPath 是镜像文档的归档路径；自撰文档为空串。
	ArtifactPath string `json:"artifactPath"`

	AppliesVersions  []string `json:"appliesVersions"`
	AppliesScenarios []string `json:"appliesScenarios"`

	Status     string `json:"status"`
	StatusText string `json:"statusText"`
	// VerifiedBy 为 null 表示未核验。
	VerifiedBy *ActorView `json:"verifiedBy"`
	Method     string     `json:"method"`
	Reason     string     `json:"reason"`

	// Current 报告它是不是该文档当前生效的修订。
	Current bool `json:"current"`
	// UsageCount 是被多少条断言引用。
	UsageCount int `json:"usageCount"`
	// SupersededBy 是取代它的 revId；空串表示没有。
	SupersededBy string `json:"supersededBy"`
}

// DocInput 是一次登记。
type DocInput struct {
	Source   string `json:"source"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Revision string `json:"revision"`
	Body     string `json:"body"`
	// ArtifactPath 是镜像文档的归档路径。
	ArtifactPath string `json:"artifactPath"`
	// ContentHash 是内容摘要。Body 非空时可留空（自动算）。
	ContentHash      string   `json:"contentHash"`
	CapturedAt       string   `json:"capturedAt"`
	AppliesVersions  []string `json:"appliesVersions"`
	AppliesScenarios []string `json:"appliesScenarios"`
}

// DocRegisterResult 是一次登记的结果。
type DocRegisterResult struct {
	Doc     DocView `json:"doc"`
	Changed bool    `json:"changed"`
	// Reverted 报告修订标识回退到旧值——这通常意味着来源出过问题。
	Reverted bool `json:"reverted"`
	// Requeued 是本次因修订变化回到待核验的断言数。**必须在界面上说出来。**
	Requeued int `json:"requeued"`
}

// UnregisteredView 是一个「依据未登记」的原件。
type UnregisteredView struct {
	Artifact   string `json:"artifact"`
	Assertions int    `json:"assertions"`
}

// DocumentService 暴露文档。
type DocumentService struct {
	session *compose.Session
}

// NewDocumentService 构造服务。
func NewDocumentService(s *compose.Session) *DocumentService {
	return &DocumentService{session: s}
}

func (s *DocumentService) port() (*compose.Project, error) { return s.session.Project() }

// List 返回每个文档的当前修订。场景名为空表示全部。
func (s *DocumentService) List(scenario string) ([]DocView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	docs, err := p.Store.Documents(scenario)
	if err != nil {
		return nil, err
	}
	usage, err := p.Store.ArtifactUsageCounts()
	if err != nil {
		return nil, err
	}
	out := make([]DocView, 0, len(docs))
	for _, d := range docs {
		v := toDocView(d)
		v.UsageCount = usage[d.Title]
		v.Current = true
		out = append(out, v)
	}
	return out, nil
}

// History 返回某个文档的全部修订，登记时间倒序。
func (s *DocumentService) History(revID string) ([]DocView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	d, ok, err := p.Store.DocumentByRev(revID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("文档修订 %s 不存在", revID)
	}
	history, err := p.Store.DocumentHistory(d.ID)
	if err != nil {
		return nil, err
	}
	current := document.Current(history)
	usage, err := p.Store.ArtifactUsageCounts()
	if err != nil {
		return nil, err
	}
	out := make([]DocView, 0, len(history))
	for _, h := range document.Order(history) {
		v := toDocView(h)
		v.Current = current != nil && h.RevID == current.RevID
		v.UsageCount = usage[h.Title]
		out = append(out, v)
	}
	return out, nil
}

// Register 登记一份文档（可能是新修订）。
//
// 返回的 `requeued` 是一次同步让多少条断言回到待核验——
// **不告知人的批量回退就是最坏的那种静默**。
func (s *DocumentService) Register(in DocInput) (DocRegisterResult, error) {
	p, err := s.port()
	if err != nil {
		return DocRegisterResult{}, err
	}
	input, err := toDocInput(in)
	if err != nil {
		return DocRegisterResult{}, err
	}
	res, err := appdoc.Register(p.Store, input, time.Now().UTC())
	if err != nil {
		return DocRegisterResult{}, err
	}
	return DocRegisterResult{
		Doc: toDocView(res.Doc), Changed: res.Changed,
		Reverted: res.Reverted, Requeued: res.Requeued,
	}, nil
}

// Verify 为这份原文背书。核验人必须是人；方法只能是编审。
func (s *DocumentService) Verify(revID, by, method, reason string) (DocView, error) {
	p, err := s.port()
	if err != nil {
		return DocView{}, err
	}
	if by == "" {
		return DocView{}, fmt.Errorf("必须指明核验人（必须是人）——无追责的核验等于没有核验")
	}
	if method == "" {
		method = string(verification.Editorial)
	}
	d, err := appdoc.Verify(p.Store, revID,
		verification.Actor{Kind: verification.Human, ID: by},
		verification.Method(method), reason)
	if err != nil {
		return DocView{}, err
	}
	return toDocView(d), nil
}

// Reject 驳回一份文档。条目保留。
func (s *DocumentService) Reject(revID, by, reason string) (DocView, error) {
	p, err := s.port()
	if err != nil {
		return DocView{}, err
	}
	if by == "" {
		return DocView{}, fmt.Errorf("必须指明驳回人（必须是人）")
	}
	d, err := appdoc.Reject(p.Store, revID,
		verification.Actor{Kind: verification.Human, ID: by}, reason)
	if err != nil {
		return DocView{}, err
	}
	return toDocView(d), nil
}

// Usage 返回引用某份文档的断言。
func (s *DocumentService) Usage(revID string) ([]Item, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	as, err := appdoc.Usage(p.Store, revID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(as))
	for _, a := range as {
		out = append(out, toItem(a))
	}
	return out, nil
}

// Unregistered 返回还没有对应文档的原件。
//
// 「溯源的终点是一个字符串」正是文档层要消灭的状态，
// 因此剩余多少必须能查出来，而不是靠人感觉。
func (s *DocumentService) Unregistered() ([]UnregisteredView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	refs, err := p.Store.UnregisteredArtifacts()
	if err != nil {
		return nil, err
	}
	out := make([]UnregisteredView, 0, len(refs))
	for _, r := range refs {
		out = append(out, UnregisteredView{Artifact: r.Artifact, Assertions: r.Assertions})
	}
	return out, nil
}

// ── 转换 ────────────────────────────────────────────────────────────────────

func toDocView(d document.Doc) DocView {
	v := DocView{
		RevID: d.RevID, DocID: d.ID, Source: d.Source, Title: d.Title,
		Kind: string(d.Kind), KindText: d.Kind.Label(),
		Revision: d.Revision, Hash: d.Hash,
		CapturedAt:   d.CapturedAt.Format(time.RFC3339),
		RegisteredAt: d.RegisteredAt.Format(time.RFC3339),
		Body:         d.Body, ArtifactPath: d.ArtifactPath,
		AppliesVersions: []string{}, AppliesScenarios: []string{},
		Status: string(d.Status), StatusText: d.Status.Label(),
		Method: string(d.Method), Reason: d.Reason,
		SupersededBy: d.SupersededBy,
	}
	if d.Applies.Versions != nil {
		v.AppliesVersions = d.Applies.Versions
	}
	if d.Applies.Scenarios != nil {
		v.AppliesScenarios = d.Applies.Scenarios
	}
	if d.VerifiedBy != nil {
		a := toActorView(*d.VerifiedBy)
		v.VerifiedBy = &a
	}
	return v
}

func toDocInput(in DocInput) (document.Input, error) {
	if !document.Kind(in.Kind).Valid() {
		return document.Input{}, fmt.Errorf(
			"文档类型非法：%q（应为 evidence / rule / explanation / change / unmodeled）", in.Kind)
	}
	var captured time.Time
	if in.CapturedAt != "" {
		t, err := time.Parse(time.RFC3339, in.CapturedAt)
		if err != nil {
			return document.Input{}, fmt.Errorf("采集时间格式不对：%w", err)
		}
		captured = t
	}
	if captured.IsZero() {
		return document.Input{}, fmt.Errorf("必须给出采集时间——它是「我们什么时候抓到的」")
	}
	return document.Input{
		Source: in.Source, Title: in.Title, Kind: document.Kind(in.Kind),
		Revision: in.Revision, Body: in.Body, ArtifactPath: in.ArtifactPath,
		ContentHash: in.ContentHash, CapturedAt: captured,
		Applies: document.AppliesTo{
			Versions: in.AppliesVersions, Scenarios: in.AppliesScenarios,
		},
	}, nil
}
