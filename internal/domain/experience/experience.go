// Package experience 定义经验：从事实推导或归纳出来的、可复用的判断。
//
// 见 docs/specs/experience.spec.md。它是工具里最有价值的一层：
// 数据是公开的、抄得来的；**经验要踩坑才有**。
//
// 本包的核心不是「怎么存经验」，而是**人机分工与责任归属**：
//
//	责任级别由参与者与依据**推导**，不由人填
//	批准者必须是人；agent 只能提出与取证
//	会话记录是依据，只能追加，不得删改
//	总结型经验必须带样本量——没有样本量的「经验」只是意见
//
// 本包是纯规则，不做任何 IO。
package experience

import (
	"fmt"
	"sort"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Kind 是经验的类型。三类得到的手段不同，验证手段也不同。
type Kind string

const (
	// KindDerived 推导型：按公式从事实算出，算例可复现。
	KindDerived Kind = "derived"
	// KindJudgment 判定型：人工裁定，依据可查、可推翻。
	KindJudgment Kind = "judgment"
	// KindSummary 总结型：从多次实践归纳。
	//
	// 它天然带概率性、带时效、会互相矛盾，**不得当作确定事实处理**。
	KindSummary Kind = "summary"
)

// Valid 报告类型是否合法。
func (k Kind) Valid() bool {
	switch k {
	case KindDerived, KindJudgment, KindSummary:
		return true
	}
	return false
}

// Label 返回中文名。
func (k Kind) Label() string {
	switch k {
	case KindDerived:
		return "推导型"
	case KindJudgment:
		return "判定型"
	case KindSummary:
		return "总结型"
	}
	return string(k)
}

// Responsibility 是责任级别：该经验由谁确认，决定其可信度上限。
type Responsibility int

const (
	// LevelCoauthored 人机协作确认：人看过并确认了 agent 的产出与依据。最高。
	LevelCoauthored Responsibility = 1
	// LevelHuman 人工确认：人独立产出或复核。
	LevelHuman Responsibility = 2
	// LevelReproducible agent 产出，依据可复现（有推导链或算例）。
	LevelReproducible Responsibility = 3
	// LevelInferred agent 产出，推断。无可复现依据，不得作为权威。
	LevelInferred Responsibility = 4
)

// Valid 报告级别是否合法。
func (r Responsibility) Valid() bool { return r >= 1 && r <= 4 }

// Label 返回中文名。
func (r Responsibility) Label() string {
	switch r {
	case LevelCoauthored:
		return "人机协作确认"
	case LevelHuman:
		return "人工确认"
	case LevelReproducible:
		return "agent 产出·依据可复现"
	case LevelInferred:
		return "agent 产出·推断"
	}
	return fmt.Sprintf("级别 %d", int(r))
}

// MaxConfidence 返回该级别允许的最高分级。
//
// 级别 1 与 2 的上限同为 L2：两者的差别在**来源**（有没有 agent 参与），
// 不在分级本身。把协作直接抬到 L1 会让「可与原文逐字比对」这个含义被稀释——
// 经验始终是判断，不是直引。
func (r Responsibility) MaxConfidence() assertion.Confidence {
	switch r {
	case LevelReproducible:
		return assertion.L3
	case LevelInferred:
		return assertion.L4
	default:
		return assertion.L2
	}
}

// Status 是经验的状态。
type Status string

const (
	// StatusCandidate 候选：已产出、尚未确认。
	StatusCandidate Status = "candidate"
	// StatusEffective 生效。
	StatusEffective Status = "effective"
	// StatusRejected 已驳回。
	StatusRejected Status = "rejected"
	// StatusStale 失效：所依赖的断言被驳回。
	StatusStale Status = "stale"
	// StatusRecompute 待重算：所依赖的公式被修改。
	StatusRecompute Status = "recompute"
)

// Valid 报告状态是否合法。
func (s Status) Valid() bool {
	switch s {
	case StatusCandidate, StatusEffective, StatusRejected, StatusStale, StatusRecompute,
		StatusSuperseded:
		return true
	}
	return false
}

// Label 返回中文名。
func (s Status) Label() string {
	switch s {
	case StatusCandidate:
		return "候选"
	case StatusEffective:
		return "生效"
	case StatusRejected:
		return "已驳回"
	case StatusStale:
		return "失效"
	case StatusRecompute:
		return "待重算"
	case StatusSuperseded:
		return "已被取代"
	}
	return string(s)
}

// Usable 报告该状态的经验能否被引用。
func (s Status) Usable() bool { return s == StatusEffective }

// Sample 是总结型经验的样本。
//
// 没有样本量的「经验」只是意见，不得进入经验层。
type Sample struct {
	Size int
	From string
}

// Turn 是会话中的一段发言。
type Turn struct {
	// Seq 从 1 开始，表示它是第几段。
	Seq  int
	Role verification.Actor
	Text string
	At   time.Time
}

// Session 是一段会话记录。
//
// 它是经验的**依据**，因此只允许追加：可删改的依据不是依据。
// 由此得到一个直接收益——经验的溯源可以精确到「哪次对话的哪一句」，
// 这是它能被审查、能被推翻、能传给别人的前提。
type Session struct {
	ID       string
	Scenario string
	Title    string
	Turns    []Turn
	At       time.Time
}

// Validate 校验会话记录。
func (s Session) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("会话缺少 ID")
	}
	if s.Scenario == "" {
		return fmt.Errorf("会话 %s 没有归属场景", s.ID)
	}
	if s.Title == "" {
		return fmt.Errorf("会话 %s 缺少标题", s.ID)
	}
	if s.At.IsZero() {
		return fmt.Errorf("会话 %s 缺少时间", s.ID)
	}
	for i, t := range s.Turns {
		if !t.Role.Valid() {
			return fmt.Errorf("会话 %s 第 %d 段缺少参与者标识", s.ID, i+1)
		}
		if t.Text == "" {
			return fmt.Errorf("会话 %s 第 %d 段没有内容", s.ID, i+1)
		}
		if t.Seq != i+1 {
			return fmt.Errorf("会话 %s 第 %d 段的序号应为 %d，实际 %d", s.ID, i+1, i+1, t.Seq)
		}
	}
	return nil
}

// Append 追加一段发言。
//
// **只有追加，没有修改与删除。** 不提供那两个方法不是遗漏：
// 一旦能改，经验所指向的「第 3 段第 12 句」就可能已经不是当初那句话了。
func (s Session) Append(role verification.Actor, text string, at time.Time) (Session, error) {
	if !role.Valid() {
		return Session{}, fmt.Errorf("追加发言必须标识参与者——无追责的记录不是依据")
	}
	if text == "" {
		return Session{}, fmt.Errorf("追加的发言不能为空")
	}
	out := s
	out.Turns = append(append([]Turn(nil), s.Turns...), Turn{
		Seq: len(s.Turns) + 1, Role: role, Text: text, At: at,
	})
	if err := out.Validate(); err != nil {
		return Session{}, err
	}
	return out, nil
}

// TurnAt 按序号取一段发言。
func (s Session) TurnAt(seq int) (Turn, bool) {
	for _, t := range s.Turns {
		if t.Seq == seq {
			return t, true
		}
	}
	return Turn{}, false
}

// Entry 是一条经验。
//
// **它不是断言**：断言是事实且上下文中立，经验是判断且围绕用途。
// 因此它挂在场景下，带提出者与批准者。
type Entry struct {
	ID       string
	Scenario string
	// Topic 是冲突比较键：同一场景下、同一个话题的两条不同说法即为冲突。
	// 没有它，「与既有经验比」无从落地。
	Topic string
	Kind  Kind
	// Statement 是判断本身。
	Statement string
	// Rationale 是为什么这么判断。
	Rationale string
	// Chain 是推导链：依赖的断言 ID 与公式名。推导型必填。
	Chain []string
	// Sample 仅总结型需要，且必须非空。
	Sample *Sample
	// Conditions 是适用条件范围。
	Conditions string
	// Preference 是该判断假设的偏好。
	Preference string

	// ProposedBy 是提出者；Collaborators 是其他参与者。
	// **责任级别由这两者与 Chain 推导**，不由调用方填写。
	ProposedBy    verification.Actor
	Collaborators []verification.Actor

	// ApprovedBy 是批准者，必须是人。未批准时为 nil。
	ApprovedBy *verification.Actor
	// ApproveReason 是批准理由。只有状态没有理由的不是确认。
	ApproveReason string

	// SessionID 与 Anchor 指向依据：哪次会议、其中的哪一句。
	SessionID string
	Anchor    string

	Status Status
	At     time.Time
	// Supersedes 指向被本条目取代的旧条目。修订只追加，旧条目保留。
	Supersedes string
}

// Participants 返回全部参与者，含提出者。
func (e Entry) Participants() []verification.Actor {
	out := make([]verification.Actor, 0, len(e.Collaborators)+1)
	out = append(out, e.ProposedBy)
	out = append(out, e.Collaborators...)
	return out
}

// Level 推导责任级别。
//
// 这一条是**算出来的**而不是填出来的：给级别留一个输入框，
// 就一定会有人填错，而级别错会让整层可信度失真。
func (e Entry) Level() Responsibility {
	var hasHuman, hasAgent bool
	for _, a := range e.Participants() {
		if !a.Valid() {
			continue
		}
		if a.IsHuman() {
			hasHuman = true
		} else {
			hasAgent = true
		}
	}
	switch {
	case hasHuman && hasAgent:
		return LevelCoauthored
	case hasHuman:
		return LevelHuman
	case hasAgent && len(e.Chain) > 0:
		return LevelReproducible
	default:
		return LevelInferred
	}
}

// Validate 校验经验的不变量。
func (e Entry) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("经验缺少 ID")
	}
	if e.Scenario == "" {
		// 经验挂在场景下，不是项目级：无归属的经验无从判断它适不适用。
		return fmt.Errorf("经验 %s 没有归属场景——无归属的经验拒绝记录", e.ID)
	}
	if e.Topic == "" {
		return fmt.Errorf("经验 %s 没有话题——没有比较键就无法发现冲突", e.ID)
	}
	if !e.Kind.Valid() {
		return fmt.Errorf("经验 %s 的类型非法：%q", e.ID, e.Kind)
	}
	if e.Statement == "" {
		return fmt.Errorf("经验 %s 缺少判断本身", e.ID)
	}
	if !e.ProposedBy.Valid() {
		return fmt.Errorf("经验 %s 没有提出者——无追责的经验等于没有经验", e.ID)
	}
	for _, a := range e.Collaborators {
		if !a.Valid() {
			return fmt.Errorf("经验 %s 的参与者标识不合法：%q", e.ID, a.ID)
		}
	}
	if e.SessionID == "" || e.Anchor == "" {
		return fmt.Errorf("经验 %s 缺少依据——必须能回溯到会话记录中的具体位置", e.ID)
	}
	if !e.Status.Valid() {
		return fmt.Errorf("经验 %s 的状态非法：%q", e.ID, e.Status)
	}
	if e.At.IsZero() {
		return fmt.Errorf("经验 %s 缺少时间", e.ID)
	}

	switch e.Kind {
	case KindDerived:
		if len(e.Chain) == 0 {
			return fmt.Errorf("推导型经验 %s 缺少推导链——不可重放的推导不是推导", e.ID)
		}
	case KindSummary:
		if e.Sample == nil || e.Sample.Size <= 0 {
			// 没有样本量的「经验」只是意见。
			return fmt.Errorf("总结型经验 %s 缺少样本量——那是意见，不是经验", e.ID)
		}
	}

	if e.Status == StatusEffective {
		if e.ApprovedBy == nil {
			return fmt.Errorf("经验 %s 已生效却没有批准者", e.ID)
		}
		if !e.ApprovedBy.IsHuman() {
			return fmt.Errorf("经验 %s 的批准者不是人——agent 可以提出与取证，但不能自己决定什么算数", e.ID)
		}
		if e.ApproveReason == "" {
			return fmt.Errorf("经验 %s 缺少批准理由——只有状态没有理由的不是确认", e.ID)
		}
	}
	if e.ApprovedBy != nil && !e.ApprovedBy.IsHuman() {
		return fmt.Errorf("经验 %s 的批准者不是人", e.ID)
	}
	return nil
}

// New 构造一条候选经验。
func New(id, scenario, topic string, kind Kind, statement, rationale string,
	chain []string, sample *Sample, sessionID, anchor string,
	proposedBy verification.Actor, collaborators []verification.Actor, at time.Time) (Entry, error) {

	e := Entry{
		ID: id, Scenario: scenario, Topic: topic, Kind: kind,
		Statement: statement, Rationale: rationale,
		Chain: chain, Sample: sample,
		SessionID: sessionID, Anchor: anchor,
		ProposedBy: proposedBy, Collaborators: collaborators,
		Status: StatusCandidate, At: at,
	}
	if err := e.Validate(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Approve 批准一条候选经验。
//
// 批准者必须是人。提出者与批准者相同时**允许**（自提自批），
// 但记录里必须体现——所以这里不抹掉提出者。
func (e Entry) Approve(by verification.Actor, reason string) (Entry, error) {
	if e.Status == StatusEffective {
		return Entry{}, fmt.Errorf("经验 %s 已经生效", e.ID)
	}
	if e.Status == StatusRejected {
		return Entry{}, fmt.Errorf("经验 %s 已被驳回——驳回是审计轨迹，不能靠再批准抹掉", e.ID)
	}
	if !by.Valid() {
		return Entry{}, fmt.Errorf("批准者未标识——无追责的确认等于没有确认")
	}
	if !by.IsHuman() {
		return Entry{}, fmt.Errorf(
			"批准者必须是**人**；agent 可以提出候选经验与取证，但不能自己决定什么算数")
	}
	if reason == "" {
		return Entry{}, fmt.Errorf("批准缺少理由——只有状态没有理由的不是确认")
	}
	out := e
	out.Status = StatusEffective
	byCopy := by
	out.ApprovedBy = &byCopy
	out.ApproveReason = reason
	if err := out.Validate(); err != nil {
		return Entry{}, err
	}
	return out, nil
}

// Reject 驳回一条经验。驳回只追加状态，不删除条目。
func (e Entry) Reject(by verification.Actor, reason string) (Entry, error) {
	if !by.Valid() || !by.IsHuman() {
		return Entry{}, fmt.Errorf("驳回必须由人记录")
	}
	if reason == "" {
		return Entry{}, fmt.Errorf("驳回缺少理由")
	}
	out := e
	out.Status = StatusRejected
	out.ApproveReason = reason
	byCopy := by
	out.ApprovedBy = &byCopy
	return out, nil
}

// Supersede 用新判断取代旧条目：**只追加，旧条目保留**。
func (e Entry) Supersede(next Entry) (Entry, error) {
	if e.Scenario != next.Scenario || e.Topic != next.Topic {
		return Entry{}, fmt.Errorf(
			"取代只能发生在同一场景的同一话题上：%s.%s 与 %s.%s",
			e.Scenario, e.Topic, next.Scenario, next.Topic)
	}
	out := e
	out.Status = StatusSuperseded
	return out, nil
}

// StatusSuperseded 是被取代的旧条目所处的状态。
//
// 它不在 spec 的五态里，是「修订只追加」这条规则的落地：
// 旧条目既不是被驳回、也不是失效，它是**被新的说法取代了**——
// 保留可查，但不再作为当前判断。
const StatusSuperseded Status = "superseded"

// Stale 标记失效（依赖的断言被驳回）。
func (e Entry) Stale(reason string) Entry {
	out := e
	out.Status = StatusStale
	out.ApproveReason = reason
	return out
}

// Recompute 标记待重算（依赖的公式被修改）。
func (e Entry) Recompute(reason string) Entry {
	out := e
	out.Status = StatusRecompute
	out.ApproveReason = reason
	return out
}

// ConflictKey 是冲突比较键。
func (e Entry) ConflictKey() string { return e.Scenario + "|" + e.Topic }

// Conflicts 返回与其它条目冲突的那些。
//
// **只标记，不择一**：两条相反的经验并存，因为它们是两个判断，
// 而不是同一个事实的两种说法。
func (e Entry) Conflicts(others []Entry) []string {
	var out []string
	for _, o := range others {
		if o.ID == e.ID || o.ConflictKey() != e.ConflictKey() {
			continue
		}
		if o.Statement == e.Statement {
			continue // 说法相同不是冲突
		}
		out = append(out, o.ID)
	}
	sort.Strings(out)
	return out
}

// Order 给出展示顺序：**级别高的在前，同级按时间倒序**。
//
// 排序必须确定：每次打开页面顺序不同，人就无法判断「和上次是不是一样」。
func Order(entries []Entry) []Entry {
	out := append([]Entry(nil), entries...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level() != out[j].Level() {
			return out[i].Level() < out[j].Level()
		}
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
