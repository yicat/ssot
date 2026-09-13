// Package alternatives 定义备选方案：多解输出与取舍呈现。
//
// 见 docs/specs/alternatives.spec.md。核心立场只有一句：
// **系统不替用户做取舍。** 它负责把取舍、代价、依据、不确定性摊开，让选择可见。
//
// 因此本包不产出「最优解」，它做三件事：
//
//	自证   —— 每个方案必须声明目标、约束、代价（含机会成本）与未核验比例
//	比较   —— 同一组维度上并排；被支配的剪枝；全部被剪枝时报冲突的约束
//	不裁决 —— 冲突来源各自成案，系统不择一
//
// 本包是纯规则，不做任何 IO。
package alternatives

import (
	"fmt"
	"sort"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
)

// Status 是方案的状态。
type Status string

const (
	// StatusCandidate 候选：已产出，尚未被选。
	StatusCandidate Status = "candidate"
	// StatusChosen 已选中。**其他备选仍然可访问。**
	StatusChosen Status = "chosen"
	// StatusStale 依据已变：来源修订、断言被驳回或数据过期。
	StatusStale Status = "stale"
	// StatusInvalid 不可执行：依赖的断言已被驳回。
	StatusInvalid Status = "invalid"
	// StatusSuperseded 已被新版本取代。历史保留。
	StatusSuperseded Status = "superseded"
)

// Valid 报告状态是否合法。
func (s Status) Valid() bool {
	switch s {
	case StatusCandidate, StatusChosen, StatusStale, StatusInvalid, StatusSuperseded:
		return true
	}
	return false
}

// Label 返回中文名。
func (s Status) Label() string {
	switch s {
	case StatusCandidate:
		return "候选"
	case StatusChosen:
		return "已选中"
	case StatusStale:
		return "依据已变"
	case StatusInvalid:
		return "不可执行"
	case StatusSuperseded:
		return "已被取代"
	}
	return string(s)
}

// Executable 报告该方案能否被执行。
//
// 「依据已变」可以执行但要提示；「不可执行」是硬拦截——
// 依赖的断言被驳回了，据它算出来的东西不该再被拿去用。
func (s Status) Executable() bool {
	return s == StatusCandidate || s == StatusChosen || s == StatusStale
}

// Metric 是一个可比的维度。
//
// 维度是开放的（资源、时间、风险……），但必须**可比**：
// 两个方案只有在同一组维度上才能被比较，否则「哪个更好」无从谈起。
type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	// LowerIsBetter 报告该维度是不是越小越好（代价类通常如此）。
	LowerIsBetter bool `json:"lowerIsBetter"`
}

// Plan 是一个方案。
type Plan struct {
	ID       string
	Scenario string
	// Title 一句话说明它做什么。
	Title string
	// Objective 是它优化的东西。**必填**——不声明目标的方案无法被比较。
	Objective string
	// Constraints 是它遵守的约束。
	Constraints []string
	// Assumptions 是它成立的前提。假设变化时方案可能失效。
	Assumptions []string
	// Preference 是它假设的偏好。
	//
	// 使用者未声明偏好时，**每个备选都必须标注它假设了哪种偏好**——
	// 否则「多解」就退化成一堆没有区别的选项。
	Preference string
	// Actions 是可执行的行动。
	Actions []string

	// Metrics 是可比维度。代价在这里：资源、时间、风险……
	Metrics []Metric
	// Opportunity 是机会成本。**必填**——它是取舍里最容易被人忽略、
	// 也最容易在执行后才被发现的一项。
	Opportunity string

	// Depends 是依赖：`assert:<ID>` 或 `formula:<名>@<版本>`。
	Depends []string
	// UnverifiedRatio 是所依赖断言中未核验的比例（0..1）。
	UnverifiedRatio float64
	// MaxConfidence 是方案级可信度，**不得高于依赖断言中的最低**。
	MaxConfidence assertion.Confidence

	// Sources 是提出该方案的来源名。用于独立性判定与分裂/合并。
	Sources []string
	// FromConflict 报告它是不是因为来源冲突而单独成案的。
	FromConflict bool

	Status Status
	At     time.Time
	// ChosenBy / ChooseReason 在选定后记录。
	ChosenBy     string
	ChooseReason string
}

// Validate 校验方案的自证要求。
//
// 这里拒绝的每一条，都是「事后才发现」的典型来源：
// 没写目标的方案无法比较，没写机会成本的方案让人低估代价，
// 没写未核验比例的方案让人高估可信度。
func (p Plan) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("方案缺少 ID")
	}
	if p.Scenario == "" {
		return fmt.Errorf("方案 %s 没有归属场景", p.ID)
	}
	if p.Title == "" {
		return fmt.Errorf("方案 %s 缺少标题", p.ID)
	}
	if p.Objective == "" {
		return fmt.Errorf("方案 %s 没有声明它优化的目标——不声明目标的方案无法被比较", p.ID)
	}
	if p.Opportunity == "" {
		return fmt.Errorf("方案 %s 没有给出机会成本——它是取舍里最容易被忽略的一项", p.ID)
	}
	if !p.Status.Valid() {
		return fmt.Errorf("方案 %s 的状态非法：%q", p.ID, p.Status)
	}
	if p.At.IsZero() {
		return fmt.Errorf("方案 %s 缺少生成时间", p.ID)
	}
	if len(p.Sources) == 0 {
		return fmt.Errorf("方案 %s 没有来源——没有出处的方案无法被追溯", p.ID)
	}
	if !p.MaxConfidence.Valid() {
		return fmt.Errorf("方案 %s 的可信度非法：%q", p.ID, p.MaxConfidence)
	}
	if p.UnverifiedRatio < 0 || p.UnverifiedRatio > 1 {
		return fmt.Errorf("方案 %s 的未核验比例应在 0~1，实际 %v", p.ID, p.UnverifiedRatio)
	}
	for _, m := range p.Metrics {
		if m.Name == "" {
			return fmt.Errorf("方案 %s 有一个没有名字的比较维度", p.ID)
		}
	}
	return nil
}

// Key 是方案在比较时的身份。
func (p Plan) Key() string { return p.Scenario + "|" + p.Title }

// MetricByName 按名取维度。
func (p Plan) MetricByName(name string) (Metric, bool) {
	for _, m := range p.Metrics {
		if m.Name == name {
			return m, true
		}
	}
	return Metric{}, false
}

// sameConditions 报告两个方案的比较前提是否一致。
//
// 目标不同、假设不同、偏好不同，就不是「同一组维度上的两个选择」，
// 把它们比出个高下毫无意义。
func sameConditions(a, b Plan) bool {
	return a.Objective == b.Objective &&
		eqStrings(a.Assumptions, b.Assumptions) &&
		a.Preference == b.Preference
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// Dominates 报告 a 是否支配 b。
//
// 支配的定义是严格的：a 在**所有**维度上都不劣于 b，且至少一个维度严格更优。
// 任取一个维度只看一眼就下结论，正是「系统替用户做取舍」的开端。
//
// 只有比较前提一致、且维度可比（同名维度两边都有）时才谈得上支配；
// 其余情形一律返回 false——**不可比就是不可比**。
func Dominates(a, b Plan) bool {
	if a.ID == b.ID || !sameConditions(a, b) {
		return false
	}
	strict := false
	compared := 0
	for _, mb := range b.Metrics {
		ma, ok := a.MetricByName(mb.Name)
		if !ok {
			// 维度对不上：无从判断，不作支配结论
			return false
		}
		compared++
		if worse(ma, mb) {
			return false
		}
		if better(ma, mb) {
			strict = true
		}
	}
	return compared > 0 && strict
}

func better(a, b Metric) bool {
	if a.LowerIsBetter {
		return a.Value < b.Value
	}
	return a.Value > b.Value
}

func worse(a, b Metric) bool {
	if a.LowerIsBetter {
		return a.Value > b.Value
	}
	return a.Value < b.Value
}

// PruneResult 是一次剪枝的结果。
type PruneResult struct {
	// Kept 是 Pareto 最优集。
	Kept []Plan
	// Pruned 是被支配而剪掉的，附带支配它的那个方案。
	Pruned []PrunedPlan
}

// PrunedPlan 记录一个被剪掉的方案及原因。
type PrunedPlan struct {
	Plan      Plan
	By        string
	Dominance string
}

// Prune 剪掉被支配的方案。
//
// 剪掉必须**说得出是被谁支配的**：一个凭空消失的备选，
// 会让人以为系统没算出来，而不是「它确实更差」。
func Prune(plans []Plan) PruneResult {
	var res PruneResult
	for _, p := range plans {
		killer := ""
		for _, q := range plans {
			if Dominates(q, p) {
				killer = q.ID
				break
			}
		}
		if killer == "" {
			res.Kept = append(res.Kept, p)
			continue
		}
		res.Pruned = append(res.Pruned, PrunedPlan{
			Plan: p, By: killer,
			Dominance: fmt.Sprintf("%s 在所有维度上都不劣于它，且至少一个维度更优", killer),
		})
	}
	sort.Slice(res.Kept, func(i, j int) bool { return res.Kept[i].ID < res.Kept[j].ID })
	sort.Slice(res.Pruned, func(i, j int) bool { return res.Pruned[i].Plan.ID < res.Pruned[j].Plan.ID })
	return res
}

// MergeEquivalent 合并各维度差异都低于阈值的方案。
//
// 不合并会产生**伪多样性**：一排看起来不同的方案，选哪个结果都一样，
// 而那会让人以为自己做了个有意义的决定。
func MergeEquivalent(plans []Plan, threshold float64) ([]Plan, [][]string) {
	var kept []Plan
	var merged [][]string
	used := map[string]bool{}

	for i, p := range plans {
		if used[p.ID] {
			continue
		}
		group := []string{p.ID}
		used[p.ID] = true
		for _, q := range plans[i+1:] {
			if used[q.ID] || !sameConditions(p, q) {
				continue
			}
			if differenceWithin(p, q, threshold) {
				group = append(group, q.ID)
				used[q.ID] = true
			}
		}
		sort.Strings(group)
		merged = append(merged, group)
		kept = append(kept, p)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].ID < kept[j].ID })
	sort.Slice(merged, func(i, j int) bool { return merged[i][0] < merged[j][0] })
	return kept, merged
}

func differenceWithin(a, b Plan, threshold float64) bool {
	for _, mb := range b.Metrics {
		ma, ok := a.MetricByName(mb.Name)
		if !ok {
			return false
		}
		d := ma.Value - mb.Value
		if d < 0 {
			d = -d
		}
		if d > threshold {
			return false
		}
	}
	return true
}

// ConflictConstraints 报告一组方案为什么互斥。
//
// 全部方案都被剪枝时，要求的输出是**冲突的约束清单**，不是空结果——
// 空结果会让人以为「算不出来」，而实际是「你的要求自相矛盾」，
// 后者是信息，前者只是故障。
func ConflictConstraints(structural []string, all []Plan) []string {
	out := append([]string(nil), structural...)
	if len(all) == 0 {
		return out
	}
	// 约束互相打架时，方案要么全被剪掉、要么压根没生成；
	// 这里把每个方案遵守的约束摆出来，让人自己看出矛盾在哪。
	seen := map[string][]string{}
	for _, p := range all {
		for _, c := range p.Constraints {
			seen[c] = append(seen[c], p.ID)
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, c := range names {
		ids := seen[c]
		sort.Strings(ids)
		out = append(out, fmt.Sprintf("%s（方案 %v）", c, ids))
	}
	return out
}

// Request 是使用者提出的要求。
type Request struct {
	// Preference 是使用者明确声明的偏好。空串表示未声明。
	Preference string
	// Constraints 是必须满足的约束。方案要全部满足才可呈现。
	//
	// 它也是「所有方案都被剪枝」的真正来源：约束互相打架时，
	// 不是系统算不出来，而是要求本身自相矛盾。
	Constraints []string
}

// Evaluation 是一次呈现的结果。
type Evaluation struct {
	// Plans 是剪枝与合并之后留下的方案。
	Plans []Plan
	// Pruned 是被剪掉的。
	Pruned []PrunedPlan
	// Merged 是被合并的组（每组第一个是保留者）。
	Merged [][]string
	// Constraints 在没有任何方案可呈现时给出冲突的约束清单。
	Constraints []string
	// Note 是人话说明：只有一个方案时要说出「未发现实质不同的备选」。
	Note string
	// PreferenceInferred 报告是否在建议一个偏好（**需人确认才生效**）。
	PreferenceInferred string
}

// Evaluate 把一组方案整理成可呈现的结果。
//
// 它严格执行四条规则：
//
//	约束过紧（没有方案全部满足）时，报出**冲突的约束清单**，而不是空结果
//	未声明偏好时，方案必须**至少覆盖两种偏好**——否则那就成了
//	  「针对该玩家的单一方案」，而使用者从没说自己是谁
//	被支配的剪掉；差异低于阈值的合并
//	剪完之后一个不剩，同样报出清单与说明
func Evaluate(plans []Plan, req Request, threshold float64) (Evaluation, error) {
	var ev Evaluation
	for _, p := range plans {
		if err := p.Validate(); err != nil {
			return ev, err
		}
	}
	// 已驳回依据或已被取代的方案不参与呈现。
	live := make([]Plan, 0, len(plans))
	for _, p := range plans {
		if p.Status == StatusInvalid || p.Status == StatusSuperseded {
			continue
		}
		live = append(live, p)
	}

	// 约束过紧：先说清是哪几条打架，这是信息，不是故障。
	feasible := make([]Plan, 0, len(live))
	for _, p := range live {
		if satisfies(p, req.Constraints) {
			feasible = append(feasible, p)
		}
	}
	if len(feasible) == 0 {
		ev.Constraints = ConflictConstraints(req.Constraints, live)
		ev.Note = "没有方案满足全部约束：这不是算不出来，而是你的要求互相打架"
		return ev, nil
	}

	if req.Preference == "" {
		if kinds := distinctPreferences(feasible); len(kinds) < 2 {
			return ev, fmt.Errorf(
				"未声明偏好时不得给出单一方案：需要至少两个标注了不同偏好的备选，"+
					"当前只有 %d 种偏好（%v）", len(kinds), kinds)
		}
		ev.PreferenceInferred = inferPreference(feasible)
	}

	merged, groups := MergeEquivalent(feasible, threshold)
	pruned := Prune(merged)
	ev.Plans = pruned.Kept
	ev.Pruned = pruned.Pruned
	ev.Merged = groups

	if len(ev.Plans) == 0 {
		ev.Constraints = ConflictConstraints(req.Constraints, live)
		if len(ev.Constraints) == 0 {
			ev.Constraints = []string{"没有可呈现的方案，且没有可摆出的约束——请检查方案是否都已失效"}
		}
		ev.Note = "所有方案都被剪枝：这不是算不出来，而是约束互相打架"
		return ev, nil
	}
	if len(ev.Plans) == 1 && len(ev.Pruned) == 0 {
		ev.Note = "未发现实质不同的备选——这不是「这个方案最好」，而是「没得比」"
	}
	return ev, nil
}

// satisfies 报告方案是否满足了全部要求。
func satisfies(p Plan, want []string) bool {
	if len(want) == 0 {
		return true
	}
	have := map[string]bool{}
	for _, c := range p.Constraints {
		have[c] = true
	}
	for _, c := range want {
		if !have[c] {
			return false
		}
	}
	return true
}

func distinctPreferences(plans []Plan) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range plans {
		if p.Preference == "" || seen[p.Preference] {
			continue
		}
		seen[p.Preference] = true
		out = append(out, p.Preference)
	}
	sort.Strings(out)
	return out
}

// inferPreference 从历史选择里推断偏好。
//
// 它只是**建议**：调用方必须让使用者显式确认之后才生效
// （见 Evaluate 的 PreferenceInferred 字段）。
func inferPreference(plans []Plan) string {
	count := map[string]int{}
	for _, p := range plans {
		if p.Status == StatusChosen && p.Preference != "" {
			count[p.Preference]++
		}
	}
	best, n := "", 0
	for k, v := range count {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

// Order 给出展示顺序：**已选中的在最前，其余按可信度与标题**。
//
// 选定之后其他备选仍在列表里——「长期选择某一类方案」可以让它排前面，
// 但**不得删除其他备选**。
func Order(plans []Plan, preferred string) []Plan {
	out := append([]Plan(nil), plans...)
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Status == StatusChosen) != (out[j].Status == StatusChosen) {
			return out[i].Status == StatusChosen
		}
		if preferred != "" {
			pi := out[i].Preference == preferred
			pj := out[j].Preference == preferred
			if pi != pj {
				return pi
			}
		}
		if out[i].MaxConfidence != out[j].MaxConfidence {
			return rank(out[i].MaxConfidence) > rank(out[j].MaxConfidence)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func rank(c assertion.Confidence) int {
	switch c {
	case assertion.L1:
		return 4
	case assertion.L2:
		return 3
	case assertion.L3:
		return 2
	case assertion.L4:
		return 1
	}
	return 0
}
