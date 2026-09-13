// 经验的接口（见 docs/specs/experience.spec.md）。
//
// 责任级别**不在这里**，也不在界面上：它由参与者与依据推导。
// 给级别留一个输入框，就一定会有人填错，而级别错会让整层可信度失真。
package api

import (
	"fmt"
	"time"

	appexp "github.com/ngnl5/ssot/internal/application/experience"
	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/experience"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// ActorView 是一个参与者（面向界面）。
type ActorView struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func toActorView(a verification.Actor) ActorView {
	return ActorView{Kind: string(a.Kind), ID: a.ID}
}

// EntryView 是一条经验（面向界面）。
type EntryView struct {
	ID       string `json:"id"`
	Scenario string `json:"scenario"`
	Topic    string `json:"topic"`
	Kind     string `json:"kind"`
	KindText string `json:"kindText"`

	Statement  string   `json:"statement"`
	Rationale  string   `json:"rationale"`
	Chain      []string `json:"chain"`
	SampleSize int      `json:"sampleSize"`
	SampleFrom string   `json:"sampleFrom"`
	Conditions string   `json:"conditions"`
	Preference string   `json:"preference"`

	// Level 与 MaxConfidence 都是**算出来的**，界面只显示不提供修改。
	Level         int    `json:"level"`
	LevelText     string `json:"levelText"`
	MaxConfidence string `json:"maxConfidence"`

	ProposedBy    ActorView   `json:"proposedBy"`
	Collaborators []ActorView `json:"collaborators"`
	// ApprovedBy 为 null 表示尚未批准。
	ApprovedBy    *ActorView `json:"approvedBy"`
	ApproveReason string     `json:"approveReason"`

	SessionID string `json:"sessionId"`
	Anchor    string `json:"anchor"`

	Status     string `json:"status"`
	StatusText string `json:"statusText"`
	Usable     bool   `json:"usable"`
	At         string `json:"at"`
	Supersedes string `json:"supersedes"`

	ConflictsWith []string `json:"conflictsWith"`
}

// TurnView 是会话中的一段发言。
type TurnView struct {
	Seq  int       `json:"seq"`
	Role ActorView `json:"role"`
	Text string    `json:"text"`
	At   string    `json:"at"`
}

// SessionView 是一段会话记录（依据）。
type SessionView struct {
	ID       string     `json:"id"`
	Scenario string     `json:"scenario"`
	Title    string     `json:"title"`
	At       string     `json:"at"`
	Turns    []TurnView `json:"turns"`
}

// ProposalInput 是一次提出。
type ProposalInput struct {
	Scenario   string   `json:"scenario"`
	Topic      string   `json:"topic"`
	Kind       string   `json:"kind"`
	Statement  string   `json:"statement"`
	Rationale  string   `json:"rationale"`
	Chain      []string `json:"chain"`
	SampleSize int      `json:"sampleSize"`
	SampleFrom string   `json:"sampleFrom"`
	Conditions string   `json:"conditions"`
	Preference string   `json:"preference"`
	SessionID  string   `json:"sessionId"`
	Anchor     string   `json:"anchor"`

	ProposedByKind string `json:"proposedByKind"`
	ProposedByID   string `json:"proposedById"`
	// Collaborators 里出现人即判为「协作」。
	Collaborators []ActorView `json:"collaborators"`
}

// ConflictView 是同一场景同一话题的一组不同说法。
type ConflictView struct {
	Scenario string      `json:"scenario"`
	Topic    string      `json:"topic"`
	Entries  []EntryView `json:"entries"`
}

// RefreshResultView 是一次依赖检查的结果。
type RefreshResultView struct {
	Checked   int      `json:"checked"`
	Staled    []string `json:"staled"`
	Recompute []string `json:"recompute"`
}

// ExperienceService 暴露经验。
type ExperienceService struct {
	session *compose.Session
}

// NewExperienceService 构造服务。
func NewExperienceService(s *compose.Session) *ExperienceService {
	return &ExperienceService{session: s}
}

func (s *ExperienceService) port() (*compose.Project, error) { return s.session.Project() }

// List 返回某场景的经验，按级别与时间排序。场景名为空表示全部。
func (s *ExperienceService) List(scenario string) ([]EntryView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	entries, err := p.Store.Entries(scenario)
	if err != nil {
		return nil, err
	}
	ordered := experience.Order(entries)
	out := make([]EntryView, 0, len(ordered))
	for _, e := range ordered {
		out = append(out, toEntryView(e))
	}
	return out, nil
}

// Conflicts 返回冲突分组。**只呈现，不裁决。**
func (s *ExperienceService) Conflicts(scenario string) ([]ConflictView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	entries, err := p.Store.Entries(scenario)
	if err != nil {
		return nil, err
	}
	groups := appexp.Conflicts(entries)
	out := make([]ConflictView, 0, len(groups))
	for _, g := range groups {
		cv := ConflictView{Scenario: g.Scenario, Topic: g.Topic, Entries: []EntryView{}}
		for _, e := range g.Entries {
			cv.Entries = append(cv.Entries, toEntryView(e))
		}
		out = append(out, cv)
	}
	return out, nil
}

// Propose 提出一条候选经验。重复提出同一句话返回已有条目。
func (s *ExperienceService) Propose(in ProposalInput) (EntryView, error) {
	p, err := s.port()
	if err != nil {
		return EntryView{}, err
	}
	proposal, err := toProposal(in)
	if err != nil {
		return EntryView{}, err
	}
	e, err := appexp.Propose(p.Store, proposal, time.Now().UTC())
	if err != nil {
		return EntryView{}, err
	}
	return toEntryView(e), nil
}

// Approve 批准一条候选经验。by 必须是人。
func (s *ExperienceService) Approve(id, by, reason string) (EntryView, error) {
	p, err := s.port()
	if err != nil {
		return EntryView{}, err
	}
	if by == "" {
		return EntryView{}, fmt.Errorf("必须指明批准者（必须是人）——无追责的确认等于没有确认")
	}
	e, err := appexp.Approve(p.Store, id,
		verification.Actor{Kind: verification.Human, ID: by}, reason)
	if err != nil {
		return EntryView{}, err
	}
	return toEntryView(e), nil
}

// Reject 驳回一条经验。条目保留——驳回是审计轨迹。
func (s *ExperienceService) Reject(id, by, reason string) (EntryView, error) {
	p, err := s.port()
	if err != nil {
		return EntryView{}, err
	}
	if by == "" {
		return EntryView{}, fmt.Errorf("必须指明驳回人（必须是人）")
	}
	e, err := appexp.Reject(p.Store, id,
		verification.Actor{Kind: verification.Human, ID: by}, reason)
	if err != nil {
		return EntryView{}, err
	}
	return toEntryView(e), nil
}

// Supersede 用新判断取代旧条目。旧条目保留，状态改为「已被取代」。
func (s *ExperienceService) Supersede(oldID string, in ProposalInput) (EntryView, EntryView, error) {
	p, err := s.port()
	if err != nil {
		return EntryView{}, EntryView{}, err
	}
	proposal, err := toProposal(in)
	if err != nil {
		return EntryView{}, EntryView{}, err
	}
	next, old, err := appexp.Supersede(p.Store, oldID, proposal, time.Now().UTC())
	if err != nil {
		return EntryView{}, EntryView{}, err
	}
	return toEntryView(next), toEntryView(old), nil
}

// Sessions 返回某场景的会话记录（依据）。
func (s *ExperienceService) Sessions(scenario string) ([]SessionView, error) {
	p, err := s.port()
	if err != nil {
		return nil, err
	}
	sessions, err := p.Store.Sessions(scenario)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(sessions))
	for _, x := range sessions {
		out = append(out, toSessionView(x))
	}
	return out, nil
}

// OpenSession 新建一段会话记录。
//
// 刻意叫 Open 而不是 Create：会话是**开出来然后往里追加**的，
// 没有「编辑会话」这个动作。
func (s *ExperienceService) OpenSession(scenario, title string) (SessionView, error) {
	p, err := s.port()
	if err != nil {
		return SessionView{}, err
	}
	if scenario == "" || title == "" {
		return SessionView{}, fmt.Errorf("会话必须归属场景，且要有标题")
	}
	now := time.Now().UTC()
	sess := experience.Session{
		ID:       sessionID(scenario, title, now),
		Scenario: scenario, Title: title, At: now,
	}
	if err := p.Store.UpsertSession(sess); err != nil {
		return SessionView{}, err
	}
	return toSessionView(sess), nil
}

// AppendTurn 追加一段发言。
//
// **没有对应的修改与删除方法**：会话记录是经验的依据，
// 一旦能改，经验所指向的「第 3 段」就可能已经不是当初那句话了。
func (s *ExperienceService) AppendTurn(sessionID, roleKind, roleID, text string) (SessionView, error) {
	p, err := s.port()
	if err != nil {
		return SessionView{}, err
	}
	role := verification.Actor{Kind: verification.ActorKind(roleKind), ID: roleID}
	next, err := p.Store.AppendSessionTurn(sessionID, role, text, time.Now().UTC())
	if err != nil {
		return SessionView{}, err
	}
	return toSessionView(next), nil
}

// Refresh 检查全部经验的依赖，标出失效与待重算。
func (s *ExperienceService) Refresh(scenario string) (RefreshResultView, error) {
	p, err := s.port()
	if err != nil {
		return RefreshResultView{}, err
	}
	formulas, err := formula.LoadDir(p.FormulasDir(), p.Units)
	if err != nil {
		return RefreshResultView{}, fmt.Errorf("加载公式：%w", err)
	}
	res, err := appexp.Refresh(p.Store, formulas, p.Units, scenario)
	if err != nil {
		return RefreshResultView{}, err
	}
	out := RefreshResultView{Checked: res.Checked, Staled: res.Staled, Recompute: res.Recompute}
	if out.Staled == nil {
		out.Staled = []string{}
	}
	if out.Recompute == nil {
		out.Recompute = []string{}
	}
	return out, nil
}

// ── 转换 ────────────────────────────────────────────────────────────────────

func toEntryView(e experience.Entry) EntryView {
	v := EntryView{
		ID: e.ID, Scenario: e.Scenario, Topic: e.Topic,
		Kind: string(e.Kind), KindText: e.Kind.Label(),
		Statement: e.Statement, Rationale: e.Rationale,
		Chain: []string{}, Conditions: e.Conditions, Preference: e.Preference,
		Level: int(e.Level()), LevelText: e.Level().Label(),
		MaxConfidence: string(e.Level().MaxConfidence()),
		ProposedBy:    toActorView(e.ProposedBy),
		Collaborators: []ActorView{},
		ApproveReason: e.ApproveReason,
		SessionID:     e.SessionID, Anchor: e.Anchor,
		Status: string(e.Status), StatusText: e.Status.Label(),
		Usable:        e.Status.Usable(),
		At:            e.At.Format(time.RFC3339),
		Supersedes:    e.Supersedes,
		ConflictsWith: []string{},
	}
	if e.Chain != nil {
		v.Chain = e.Chain
	}
	for _, a := range e.Collaborators {
		v.Collaborators = append(v.Collaborators, toActorView(a))
	}
	if e.ApprovedBy != nil {
		a := toActorView(*e.ApprovedBy)
		v.ApprovedBy = &a
	}
	if e.Sample != nil {
		v.SampleSize = e.Sample.Size
		v.SampleFrom = e.Sample.From
	}
	return v
}

func toSessionView(s experience.Session) SessionView {
	v := SessionView{
		ID: s.ID, Scenario: s.Scenario, Title: s.Title,
		At: s.At.Format(time.RFC3339), Turns: []TurnView{},
	}
	for _, t := range s.Turns {
		v.Turns = append(v.Turns, TurnView{
			Seq: t.Seq, Role: toActorView(t.Role), Text: t.Text, At: t.At.Format(time.RFC3339),
		})
	}
	return v
}

func toProposal(in ProposalInput) (appexp.Proposal, error) {
	if k := verification.ActorKind(in.ProposedByKind); k != verification.Human && k != verification.Agent {
		return appexp.Proposal{}, fmt.Errorf("提出者类型非法：%q（应为 human / agent）", in.ProposedByKind)
	}
	if in.ProposedByID == "" {
		return appexp.Proposal{}, fmt.Errorf("必须标识提出者——无追责的经验等于没有经验")
	}
	var collab []verification.Actor
	for _, c := range in.Collaborators {
		k := verification.ActorKind(c.Kind)
		if (k != verification.Human && k != verification.Agent) || c.ID == "" {
			return appexp.Proposal{}, fmt.Errorf("参与者标识不合法：%+v", c)
		}
		collab = append(collab, verification.Actor{Kind: k, ID: c.ID})
	}
	var sample *experience.Sample
	if in.SampleSize > 0 || in.SampleFrom != "" {
		sample = &experience.Sample{Size: in.SampleSize, From: in.SampleFrom}
	}
	return appexp.Proposal{
		Scenario: in.Scenario, Topic: in.Topic, Kind: experience.Kind(in.Kind),
		Statement: in.Statement, Rationale: in.Rationale,
		Chain: in.Chain, Sample: sample,
		Conditions: in.Conditions, Preference: in.Preference,
		SessionID: in.SessionID, Anchor: in.Anchor,
		ProposedBy:    verification.Actor{Kind: verification.ActorKind(in.ProposedByKind), ID: in.ProposedByID},
		Collaborators: collab,
	}, nil
}
