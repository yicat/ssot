// Package decision 定义「待判定」：歧义的人工裁决。
//
// 见 docs/specs/decision.spec.md。它解决的是一个具体的失败模式：
//
//	接入层遇到「原文里有两个都说得通的取值」时，唯一正确的做法是不猜——
//	但「不猜」不等于「丢掉」。此前这类项只被打印到控制台，进程一退出就消失，
//	于是**歧义与缺失分不开**（都是「没抽到」）。
//
// 本包把歧义变成一条可处理、可追溯的待办：agent 提出全部候选并附上原文锚点与
// 上下文，人在界面上选一个（或判定都不对、或暂缓），选择过程留下核验记录，
// 选中的候选走**与普通候选完全相同的准入路径**。
//
// 本包是纯规则，不做任何 IO：加载、准入与落库由编排层负责。
package decision

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Status 是事项的状态。
type Status string

const (
	// StatusOpen 待判定：从未处理过，需要人看。
	StatusOpen Status = "open"
	// StatusDeferred 已暂缓：人看过并决定先放着。**这是未处理，不是结论。**
	StatusDeferred Status = "deferred"
	// StatusDecided 已裁决：结论与当前修订一致。
	StatusDecided Status = "decided"
	// StatusStale 需复核：已裁决，但新修订的候选里已不含当初选中的取值。
	StatusStale Status = "stale"
)

// Valid 报告状态是否合法。
func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusDeferred, StatusDecided, StatusStale:
		return true
	}
	return false
}

// NeedsAttention 报告该状态是否还需要人看。
//
// 界面的默认视图就是这一组——**不能让人自己猜哪些还没处理**。
func (s Status) NeedsAttention() bool {
	return s == StatusOpen || s == StatusDeferred || s == StatusStale
}

// Label 返回中文名。
func (s Status) Label() string {
	switch s {
	case StatusOpen:
		return "待判定"
	case StatusDeferred:
		return "已暂缓"
	case StatusDecided:
		return "已裁决"
	case StatusStale:
		return "需复核"
	}
	return string(s)
}

// None 是「都不对」的候选序号。
//
// 它是一个**合法结论**，不是缺省值：判定原文里的所有说法都不成立，
// 该谓词据此记为缺失。不得用「随便选一个」来逃避这个结论。
const None = -1

// Candidate 是一种说得通的取值。
//
// 只给一个数字，人无法判断——因此锚点与上下文是必填的。
type Candidate struct {
	Value value.Value
	// Anchor 是原文位置，用于回溯到原件逐字比对。
	Anchor string
	// Context 是该候选周围的原文片段。人可以不开原件就判断。
	Context string
	// Note 是 agent 的倾向说明。可以为空，**不影响裁决结果**。
	Note string
	// Parsing 决定该候选被选中后能给的最高分级。
	Parsing assertion.Parsing
}

// Validate 校验候选是否可处理。
func (c Candidate) Validate() error {
	if !c.Parsing.Valid() {
		return fmt.Errorf("候选的解析方式非法：%q", c.Parsing)
	}
	if c.Anchor == "" {
		return fmt.Errorf("候选缺少原文锚点——没有锚点就无法回溯比对")
	}
	if c.Context == "" {
		return fmt.Errorf("候选缺少上下文片段——只给一个数字，人无法判断")
	}
	return nil
}

// Resolution 是一次裁决。
type Resolution struct {
	// Choice 是选中的候选序号；None(-1) 表示「都不对」。
	Choice int
	// ChosenValue 是选中取值的副本。
	//
	// 存副本而非只存序号：候选列表会随修订变化，
	// 「当初选的是什么」必须独立于当前候选列表可读。
	ChosenValue value.Value
	// By 是裁决人。必须是**人**——agent 可以提出候选，但不能自己决定什么算数。
	By verification.Actor
	// Reason 是裁决理由，必填。
	Reason string
	// Method 是核验方法。人选了候选，方法默认「编审」——
	// 除非裁决人给出更强的依据。
	Method   verification.Method
	Evidence string
	At       time.Time
	// AssertionID 是裁决产出的断言。判「都不对」时为空串。
	AssertionID string
}

// IsNone 报告本次裁决是否为「都不对」。
func (r Resolution) IsNone() bool { return r.Choice == None }

// Validate 校验裁决的完整性。
func (r Resolution) Validate() error {
	if !r.By.Valid() {
		return fmt.Errorf("裁决人未标识——无追责的裁决等于没有裁决")
	}
	if !r.By.IsHuman() {
		return fmt.Errorf("裁决人必须是**人**；agent 可以提出候选与取证，但不能自己决定什么算数")
	}
	if r.Reason == "" {
		return fmt.Errorf("裁决缺少理由——只有结论没有理由的不是裁决")
	}
	if !r.Method.Valid() {
		return fmt.Errorf("未知的核验方法 %q", r.Method)
	}
	if r.Method == verification.Measurement && r.Evidence == "" {
		return fmt.Errorf("以「实测」裁决时必须记录依据（版本、配置、样本数）")
	}
	if r.At.IsZero() {
		return fmt.Errorf("裁决缺少时间")
	}
	return nil
}

// Deferral 是一次暂缓。
//
// 它**不是结论**：事项仍在待判定队列里，只是被标记成「人已看过、先放着」。
type Deferral struct {
	By     verification.Actor
	Reason string
	At     time.Time
}

// Item 是一个待判定事项：有多个候选值、必须由人选。
//
// **它不是断言**——在裁决并通过准入之前，它只是「原文里出现过的几种说法」。
type Item struct {
	// ID 由 (实体, 主体, 谓词, 原件) 决定，**跨修订稳定**：
	// 否则每次重新同步都会生成一个新事项，历史裁决就断链了。
	ID        string
	Entity    string
	Subject   string
	Predicate string
	// Reason 是歧义原因（人话，含候选个数）。
	Reason string
	// Context 是歧义所在的原文片段。
	Context  string
	Artifact string
	Revision string
	// CapturedAt 是原件的采集时间。裁决产出的断言必须沿用它——
	// 裁决是「对已有原件的一次判断」，不是一次新的采集，
	// 填成「裁决那一刻」会让溯源指向错误的时间点。
	CapturedAt time.Time
	Source     string

	Candidates []Candidate
	Status     Status
	Resolution *Resolution
	Deferral   *Deferral
	// History 是被取代的旧裁决。改判不覆盖历史——审计轨迹只追加。
	History []Resolution
}

// Upsert 报告一次接入发现的结果。
//
// 它是领域概念而非存储细节：**「这轮接入有没有改变什么」决定了要不要提示人**，
// 因此属于规则的一部分。
type Upsert struct {
	// Seen 是本轮接入见到的事项数
	Seen int
	// Created 是新建的
	Created int
	// Updated 是候选或修订发生变化的
	Updated int
	// Unchanged 是完全没变的（重复接入时应当全部落在这里）
	Unchanged int
	// Reopened 是被重新打开或标记需复核的
	Reopened int
}

// String 返回一行摘要。
func (u Upsert) String() string {
	return fmt.Sprintf("待判定 %d 项：新建 %d，更新 %d，重新打开 %d，未变 %d",
		u.Seen, u.Created, u.Updated, u.Reopened, u.Unchanged)
}

// ItemID 返回事项标识。跨修订稳定，因此**不含修订标识**。
func ItemID(entity, subject, predicate, artifact string) string {
	h := sha1.Sum([]byte(entity + "|" + subject + "|" + predicate + "|" + artifact))
	return "d" + hex.EncodeToString(h[:8])
}

// New 构造一个待判定事项。
//
// 候选不足两个时**拒绝**：单候选不是歧义，应当直接走准入；
// 零候选是「没有」，更不是「不确定」。
func New(entity, subject, predicate, reason, context, artifact, revision, source string, capturedAt time.Time, cands []Candidate) (Item, error) {
	it := Item{
		ID:         ItemID(entity, subject, predicate, artifact),
		Entity:     entity,
		Subject:    subject,
		Predicate:  predicate,
		Reason:     reason,
		Context:    context,
		Artifact:   artifact,
		Revision:   revision,
		CapturedAt: capturedAt,
		Source:     source,
		Candidates: dedupe(cands),
		Status:     StatusOpen,
	}
	if err := it.Validate(); err != nil {
		return Item{}, err
	}
	return it, nil
}

// dedupe 去掉取值重复的候选，保留首次出现的那个（及其锚点）。
func dedupe(cands []Candidate) []Candidate {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		k := assertion.ValueJSON(c.Value)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}

// Validate 校验事项的不变量。
func (i Item) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("事项缺少 ID")
	}
	if i.Entity == "" || i.Subject == "" || i.Predicate == "" {
		return fmt.Errorf("事项 %s 缺少实体、主体或谓词", i.ID)
	}
	if i.Artifact == "" {
		return fmt.Errorf("事项 %s 缺少来源原件——没有出处的歧义无法核验", i.ID)
	}
	if i.CapturedAt.IsZero() {
		return fmt.Errorf("事项 %s 缺少采集时间——溯源不完整", i.ID)
	}
	if i.Reason == "" {
		return fmt.Errorf("事项 %s 缺少歧义原因", i.ID)
	}
	if !i.Status.Valid() {
		return fmt.Errorf("事项 %s 的状态非法：%q", i.ID, i.Status)
	}
	for _, c := range i.Candidates {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("事项 %s 的候选不合法：%w", i.ID, err)
		}
	}
	if len(i.Candidates) < 2 {
		return fmt.Errorf("事项 %s 只有 %d 个候选——单候选不是歧义，应当直接走准入",
			i.ID, len(i.Candidates))
	}

	decided := i.Status == StatusDecided || i.Status == StatusStale
	if decided && i.Resolution == nil {
		return fmt.Errorf("事项 %s 状态为「%s」但没有裁决记录", i.ID, i.Status.Label())
	}
	if !decided && i.Resolution != nil {
		return fmt.Errorf("事项 %s 状态为「%s」却带着裁决记录", i.ID, i.Status.Label())
	}
	if i.Status == StatusDeferred && i.Deferral == nil {
		return fmt.Errorf("事项 %s 状态为已暂缓但没有暂缓记录", i.ID)
	}
	if i.Status == StatusOpen && i.Deferral != nil {
		return fmt.Errorf("事项 %s 已重新打开却仍带着暂缓记录", i.ID)
	}

	// 「需复核」的定义就是：裁决过，但当前候选里已经没有当初选中的取值。
	if i.Status == StatusStale && i.Resolution != nil && i.HasChosen() {
		return fmt.Errorf("事项 %s 标记为需复核，但原选项 %s 仍在候选中——状态与数据不一致",
			i.ID, i.Resolution.ChosenValue.String())
	}
	return nil
}

// HasChosen 报告本次裁决是否选中了某个候选（而非判定「都不对」）。
func (i Item) HasChosen() bool {
	if i.Resolution == nil || i.Resolution.IsNone() {
		return false
	}
	return i.Contains(i.Resolution.ChosenValue)
}

// Contains 报告候选中是否含该取值。
func (i Item) Contains(v value.Value) bool {
	want := assertion.ValueJSON(v)
	for _, c := range i.Candidates {
		if assertion.ValueJSON(c.Value) == want {
			return true
		}
	}
	return false
}

// ErrAlreadyDecided 表示事项已裁决、不支持覆盖式改判。
var ErrAlreadyDecided = fmt.Errorf("事项已裁决，不支持覆盖式改判——改判需要新的核验记录")

// Resolve 应用一次裁决，返回新事项。它只做规则校验，不碰存储与准入。
//
// 选中候选时返回的事项**还没有断言 ID**：断言要经过准入层校验才会产生，
// 由编排层在校验通过后调用 BindAssertion 补上。
func (i Item) Resolve(r Resolution) (Item, error) {
	if i.Status == StatusDecided || i.Status == StatusStale {
		return Item{}, ErrAlreadyDecided
	}
	if !i.Status.NeedsAttention() {
		return Item{}, fmt.Errorf("事项 %s 的状态为「%s」，不可裁决", i.ID, i.Status.Label())
	}
	if r.Choice < None || r.Choice >= len(i.Candidates) {
		return Item{}, fmt.Errorf("候选序号 %d 越界：本事项有 %d 个候选（都不对为 %d）",
			r.Choice, len(i.Candidates), None)
	}
	if err := r.Validate(); err != nil {
		return Item{}, err
	}
	// 「选原文里的哪个数字」只有两种正当依据：人读原文判断（编审），
	// 或人在游戏里量过（实测）。重算与多源都不适用于本场景，不得冒充。
	if r.Method != verification.Editorial && r.Method != verification.Measurement {
		return Item{}, fmt.Errorf("裁决方法只能是「编审」或「实测」，「%s」不适用于在原文候选间取舍",
			r.Method.Label())
	}

	out := i.clone()
	out.Deferral = nil
	if r.Choice == None {
		out.Resolution = &r
		out.Status = StatusDecided
		return out, nil
	}

	c := i.Candidates[r.Choice]
	if assertion.ValueJSON(r.ChosenValue) != assertion.ValueJSON(c.Value) {
		return Item{}, fmt.Errorf("裁决取值为 %s，与第 %d 个候选 %s 不一致——不得凭序号与取值两套口径",
			r.ChosenValue.String(), r.Choice, c.Value.String())
	}
	out.Resolution = &r
	out.Status = StatusDecided
	return out, nil
}

// BindAssertion 记录裁决所产出的断言 ID。由编排层在准入通过后调用。
func (i *Item) BindAssertion(id string) error {
	if i.Resolution == nil {
		return fmt.Errorf("事项 %s 没有裁决记录，无法绑定断言", i.ID)
	}
	if i.Resolution.IsNone() {
		return fmt.Errorf("事项 %s 判为「都不对」，不应产出断言", i.ID)
	}
	if id == "" {
		return fmt.Errorf("事项 %s 的断言 ID 为空", i.ID)
	}
	i.Resolution.AssertionID = id
	return nil
}

// Defer 暂缓该事项。**不产生断言**，事项仍在待判定队列里。
func (i Item) Defer(d Deferral) (Item, error) {
	if i.Status == StatusDecided || i.Status == StatusStale {
		return Item{}, ErrAlreadyDecided
	}
	if !d.By.Valid() || !d.By.IsHuman() {
		return Item{}, fmt.Errorf("暂缓必须由人记录——agent 不得替人决定什么算数")
	}
	if d.Reason == "" {
		return Item{}, fmt.Errorf("暂缓缺少理由——只说「先放着」不构成一条可追溯的记录")
	}
	if d.At.IsZero() {
		return Item{}, fmt.Errorf("暂缓缺少时间")
	}
	out := i.clone()
	out.Status = StatusDeferred
	out.Deferral = &d
	return out, nil
}

// Refresh 用新的候选与修订更新事项，并按规则重算状态。
//
// 规则（见 spec「失效与重开」）：
//
//	修订未变化      -> 状态一律不变（重复接入必须幂等）
//	未裁决         -> 修订变化则回到待判定（暂缓失效，原文变了要重新看）
//	判「都不对」    -> 修订变化则重新打开（原文变了，结论未必还成立）
//	选中某候选      -> 候选里仍有该值则维持已裁决；已消失则标记需复核
//
// 无论修订是否变化，候选列表都取本次抽取的结果：它描述的是「当前抽取器
// 在当前原文里看到的几种说法」。原文没变而候选变了，说明抽取器改了判断——
// 此时若原选项已不在候选中，结论同样要复核。
func (i Item) Refresh(cands []Candidate, revision, reason, context string) Item {
	cands = dedupe(cands)
	out := i.clone()
	// 候选不足两个时保留旧候选：宁可让人看到过期的候选，
	// 也不能因为一次抽取退化就把待办变没。
	if len(cands) >= 2 {
		out.Candidates = cands
	}
	if revision != "" {
		out.Revision = revision
	}
	if reason != "" {
		out.Reason = reason
	}
	if context != "" {
		out.Context = context
	}

	changed := revision != i.Revision
	chosenGone := i.Resolution != nil && !i.Resolution.IsNone() &&
		!out.Contains(i.Resolution.ChosenValue)

	switch i.Status {
	case StatusOpen:
		return out
	case StatusDeferred:
		if changed {
			out.Status = StatusOpen
			out.Deferral = nil
		}
		return out
	case StatusDecided:
		if i.Resolution == nil || i.Resolution.IsNone() {
			if changed {
				out.Status = StatusOpen
				out.Resolution = nil
			}
			return out
		}
		if chosenGone {
			out.Status = StatusStale
		}
		return out
	case StatusStale:
		out.Status = StatusStale
		return out
	}
	return out
}

// ResolveStale 对「需复核」事项再次裁决：旧结论进历史，不覆盖。
func (i Item) ResolveStale(r Resolution) (Item, error) {
	if i.Status != StatusStale {
		return Item{}, fmt.Errorf("事项 %s 的状态为「%s」，不是需复核", i.ID, i.Status.Label())
	}
	c := i.clone()
	if c.Resolution != nil {
		c.History = append(c.History, *c.Resolution)
	}
	c.Status = StatusOpen // 借用待判定的校验路径
	c.Resolution = nil
	return c.Resolve(r)
}

func (i Item) clone() Item {
	out := i
	out.Candidates = append([]Candidate(nil), i.Candidates...)
	out.History = append([]Resolution(nil), i.History...)
	if i.Resolution != nil {
		r := *i.Resolution
		out.Resolution = &r
	}
	if i.Deferral != nil {
		d := *i.Deferral
		out.Deferral = &d
	}
	return out
}

// Order 给出队列顺序：影响面降序，影响面相同则候选数少者优先。
//
// 同分再按 ID 升序——**排序必须确定且可复现**，否则两次打开界面顺序不同，
// 人就无法判断「上次看到哪了」。
//
// impact 以 `实体|主体` 为键，值是该主体上已有断言被引用的次数。
func Order(items []Item, impact map[string]int, limit int) []Item {
	out := append([]Item(nil), items...)
	key := func(i Item) string { return i.Entity + "|" + i.Subject }
	sort.Slice(out, func(a, b int) bool {
		ia, ib := impact[key(out[a])], impact[key(out[b])]
		if ia != ib {
			return ia > ib
		}
		if len(out[a].Candidates) != len(out[b].Candidates) {
			return len(out[a].Candidates) < len(out[b].Candidates)
		}
		return out[a].ID < out[b].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ImpactOf 返回某事项的影响面。
func ImpactOf(i Item, impact map[string]int) int {
	return impact[i.Entity+"|"+i.Subject]
}

// ImpactReason 返回排序理由。排在前面必须有可见的理由，否则人只能盲信排序。
func ImpactReason(i Item, impact map[string]int) string {
	n := ImpactOf(i, impact)
	switch {
	case n > 0 && len(i.Candidates) > 2:
		return fmt.Sprintf("被引用 %d 次，%d 个候选", n, len(i.Candidates))
	case n > 0:
		return fmt.Sprintf("被引用 %d 次", n)
	case len(i.Candidates) > 2:
		return fmt.Sprintf("%d 个候选，判断成本低优先", len(i.Candidates))
	default:
		return "候选仅 2 个，判断成本最低"
	}
}
