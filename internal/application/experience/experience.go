// Package experience 是经验的用例编排。
//
// 见 docs/specs/experience.spec.md。它把四件事从界面里抽出来，
// 使 CLI 与 GUI 走同一套逻辑：
//
//	提出   —— agent 或人提交候选经验
//	确认   —— 由人批准；agent 不得批准
//	查冲突 —— 同一场景同一话题的不同说法并存并标记
//	重算   —— 依赖被驳回时失效、依赖公式变更时待重算
//
// 规则本身在 domain/experience 里（纯函数、可测）；本包只负责取数与编排。
package experience

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/domain/experience"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Port 是经验所需的读写端口。
//
// 注意它**没有**「修改会话发言」与「删除条目」——那两件事在存储层就没有，
// 在这里也不该出现：可删改的依据不是依据。
type Port interface {
	PutEntry(experience.Entry) error
	UpdateEntryStatus(id, status, reason string, approvedBy *verification.Actor) error
	Entry(id string) (experience.Entry, bool, error)
	Entries(scenario string) ([]experience.Entry, error)

	UpsertSession(experience.Session) error
	AppendSessionTurn(id string, role verification.Actor, text string, at time.Time) (experience.Session, error)
	Sessions(scenario string) ([]experience.Session, error)
	Session(id string) (experience.Session, bool, error)

	// SetSupersedes 记录「本条目取代了哪一条」。
	// 它单独成一个方法，是因为取代关系与状态是两件事：
	// 状态会随依赖变化再变，取代关系一旦成立就不该再改。
	SetSupersedes(id, supersedes string) error

	// AssertionStatus 返回某断言的核验状态，供依赖检查使用。
	AssertionStatus(id string) (string, bool, error)
}

// Proposal 是一次提出。
type Proposal struct {
	Scenario  string
	Topic     string
	Kind      experience.Kind
	Statement string
	Rationale string
	// Chain 是推导链。每一项形如 `assert:<断言ID>` 或 `formula:<名>@<版本>`。
	Chain      []string
	Sample     *experience.Sample
	Conditions string
	Preference string

	SessionID string
	Anchor    string

	ProposedBy    verification.Actor
	Collaborators []verification.Actor
}

// EntryID 是经验的身份：**由内容决定**。
//
// 同一个 (场景, 话题, 判断) 永远得到同一个 ID，因此重复提出是幂等的，
// 不会把同一句话堆成十几条。改判会得到不同的判断文字，
// 因此是不同的条目——这正是「修订只追加」想要的。
func EntryID(scenario, topic, statement string) string {
	h := sha1.Sum([]byte(scenario + "|" + topic + "|" + statement))
	return "e" + hex.EncodeToString(h[:8])
}

// Propose 提出一条候选经验。
//
// 重复提出**同一句话**返回已有条目，而不是再写一条。
func Propose(p Port, in Proposal, now time.Time) (experience.Entry, error) {
	if err := validateChain(in.Chain); err != nil {
		return experience.Entry{}, err
	}
	id := EntryID(in.Scenario, in.Topic, in.Statement)
	if existing, ok, err := p.Entry(id); err != nil {
		return experience.Entry{}, err
	} else if ok {
		return existing, nil
	}
	e, err := experience.New(id, in.Scenario, in.Topic, in.Kind, in.Statement, in.Rationale,
		in.Chain, in.Sample, in.SessionID, in.Anchor,
		in.ProposedBy, in.Collaborators, now)
	if err != nil {
		return experience.Entry{}, err
	}
	if err := p.PutEntry(e); err != nil {
		return experience.Entry{}, err
	}
	return e, nil
}

// validateChain 校验推导链的写法。
//
// 写成自由文本的话，「可被重放」这条要求就无从落实：
// 得先能机械地解析出它依赖了什么，才谈得上重放。
func validateChain(chain []string) error {
	for _, c := range chain {
		switch {
		case strings.HasPrefix(c, "assert:") && len(c) > len("assert:"):
		case strings.HasPrefix(c, "formula:") && strings.Contains(c, "@"):
		default:
			return fmt.Errorf("推导链项 %q 写法不合法：应为 assert:<断言ID> 或 formula:<名>@<版本>", c)
		}
	}
	return nil
}

// Approve 批准一条候选经验。by 必须是人。
func Approve(p Port, id string, by verification.Actor, reason string) (experience.Entry, error) {
	e, ok, err := p.Entry(id)
	if err != nil {
		return experience.Entry{}, err
	}
	if !ok {
		return experience.Entry{}, fmt.Errorf("经验 %s 不存在", id)
	}
	next, err := e.Approve(by, reason)
	if err != nil {
		return experience.Entry{}, err
	}
	byCopy := by
	if err := p.UpdateEntryStatus(next.ID, string(next.Status), next.ApproveReason, &byCopy); err != nil {
		return experience.Entry{}, err
	}
	return next, nil
}

// Reject 驳回一条经验。驳回是审计轨迹，条目保留。
func Reject(p Port, id string, by verification.Actor, reason string) (experience.Entry, error) {
	e, ok, err := p.Entry(id)
	if err != nil {
		return experience.Entry{}, err
	}
	if !ok {
		return experience.Entry{}, fmt.Errorf("经验 %s 不存在", id)
	}
	next, err := e.Reject(by, reason)
	if err != nil {
		return experience.Entry{}, err
	}
	byCopy := by
	if err := p.UpdateEntryStatus(next.ID, string(next.Status), next.ApproveReason, &byCopy); err != nil {
		return experience.Entry{}, err
	}
	return next, nil
}

// Supersede 用新判断取代旧条目：旧条目保留，只把状态改成「已被取代」。
func Supersede(p Port, oldID string, in Proposal, now time.Time) (experience.Entry, experience.Entry, error) {
	old, ok, err := p.Entry(oldID)
	if err != nil {
		return experience.Entry{}, experience.Entry{}, err
	}
	if !ok {
		return experience.Entry{}, experience.Entry{}, fmt.Errorf("经验 %s 不存在", oldID)
	}
	nextID := EntryID(in.Scenario, in.Topic, in.Statement)
	if nextID == oldID {
		return experience.Entry{}, experience.Entry{}, fmt.Errorf("新判断与旧条目内容相同，无需取代")
	}
	next, err := Propose(p, in, now)
	if err != nil {
		return experience.Entry{}, experience.Entry{}, err
	}
	if err := p.SetSupersedes(next.ID, oldID); err != nil {
		return experience.Entry{}, experience.Entry{}, err
	}
	next.Supersedes = oldID
	stale, err := old.Supersede(next)
	if err != nil {
		return experience.Entry{}, experience.Entry{}, err
	}
	stale.ApproveReason = "已被 " + next.ID + " 取代"
	if err := p.UpdateEntryStatus(stale.ID, string(stale.Status), stale.ApproveReason, stale.ApprovedBy); err != nil {
		return experience.Entry{}, experience.Entry{}, err
	}
	return next, stale, nil
}

// ConflictGroup 是同一场景同一话题的一组不同说法。
type ConflictGroup struct {
	Scenario string
	Topic    string
	Entries  []experience.Entry
}

// Conflicts 返回冲突分组。
//
// **只分组，不择一。** 两条相反的经验并存，因为它们是两个判断，
// 而不是同一个事实的两种说法——后者的处理方式是核验，前者是呈现。
func Conflicts(entries []experience.Entry) []ConflictGroup {
	byKey := map[string][]experience.Entry{}
	var order []string
	for _, e := range entries {
		if e.Status == experience.StatusRejected {
			continue
		}
		k := e.ConflictKey()
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], e)
	}
	sort.Strings(order)

	var out []ConflictGroup
	for _, k := range order {
		group := byKey[k]
		// 说法全都相同的不是冲突
		distinct := map[string]bool{}
		for _, e := range group {
			distinct[e.Statement] = true
		}
		if len(distinct) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].ID < group[j].ID })
		out = append(out, ConflictGroup{
			Scenario: group[0].Scenario, Topic: group[0].Topic, Entries: group,
		})
	}
	return out
}

// RefreshResult 报告一次依赖检查的结果。
type RefreshResult struct {
	Checked   int
	Staled    []string
	Recompute []string
}

// Refresh 检查全部经验的依赖，把失效与待重算标出来。
//
// 这件事必须由系统做：一条经验依赖的断言被驳回之后，
// 靠人去想起来「那条经验该失效了」是不现实的。
func Refresh(p Port, formulas map[string]*formula.Formula, units *unit.Table, scenario string) (RefreshResult, error) {
	entries, err := p.Entries(scenario)
	if err != nil {
		return RefreshResult{}, err
	}
	var res RefreshResult
	for _, e := range entries {
		if e.Status != experience.StatusEffective && e.Status != experience.StatusCandidate {
			continue
		}
		res.Checked++
		reason, next, err := checkDeps(p, formulas, units, e)
		if err != nil {
			return res, err
		}
		if reason == "" {
			continue
		}
		if err := p.UpdateEntryStatus(e.ID, string(next), reason, e.ApprovedBy); err != nil {
			return res, err
		}
		switch next {
		case experience.StatusStale:
			res.Staled = append(res.Staled, e.ID)
		case experience.StatusRecompute:
			res.Recompute = append(res.Recompute, e.ID)
		}
	}
	return res, nil
}

// checkDeps 返回失效原因与应处的状态；无需变更时返回空原因。
func checkDeps(p Port, formulas map[string]*formula.Formula, units *unit.Table, e experience.Entry) (
	string, experience.Status, error) {

	for _, item := range e.Chain {
		switch {
		case strings.HasPrefix(item, "assert:"):
			id := strings.TrimPrefix(item, "assert:")
			status, ok, err := p.AssertionStatus(id)
			if err != nil {
				return "", "", err
			}
			if !ok {
				return fmt.Sprintf("依赖的断言 %s 已不存在", id), experience.StatusStale, nil
			}
			if status == "rejected" {
				return fmt.Sprintf("依赖的断言 %s 已被驳回", id), experience.StatusStale, nil
			}
		case strings.HasPrefix(item, "formula:"):
			name, ver := splitFormula(item)
			f, ok := formulas[name]
			if !ok {
				return fmt.Sprintf("依赖的公式 %s 已不存在", name), experience.StatusStale, nil
			}
			if ver != "" && f.Version != ver {
				return fmt.Sprintf("依赖的公式 %s 已从 %s 改为 %s", name, ver, f.Version),
					experience.StatusRecompute, nil
			}
			if _, st := f.Verify(units); st == formula.StatusFailed {
				return fmt.Sprintf("依赖的公式 %s 算例未通过", name), experience.StatusStale, nil
			}
		}
	}
	return "", "", nil
}

func splitFormula(item string) (name, version string) {
	body := strings.TrimPrefix(item, "formula:")
	if i := strings.LastIndex(body, "@"); i >= 0 {
		return body[:i], body[i+1:]
	}
	return body, ""
}
