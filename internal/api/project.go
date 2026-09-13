// 会话、项目与场景的接口（见 docs/specs/workspace.spec.md）。
//
// 本文件只做参数转换与调用转发：谁属于项目、谁属于场景这类判断在
// compose 与 application 里，CLI 与 GUI 因此看到同一套规则。
package api

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/infrastructure/projectfile"
)

// ProjectRef 是一个可选的项目（面向界面）。
type ProjectRef struct {
	Dir  string `json:"dir"`
	Name string `json:"name"`
	// Path 是完整路径。两个项目目录同名是允许的，
	// 因此界面上必须显示完整路径才能区分。
	Path        string `json:"path"`
	Description string `json:"description"`
	// Current 标记它是不是当前打开的那个。
	Current bool `json:"current"`
}

// ScenarioRef 是一个可选的场景（面向界面）。
type ScenarioRef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Inputs 是需要外部输入的项数。
	Inputs int `json:"inputs"`
	// RequiresMet 报告 requires 是否全部满足。
	RequiresMet bool `json:"requiresMet"`
	// RequiresTotal 是 requires 的项数，用于显示「3 项中 2 项满足」。
	RequiresTotal int `json:"requiresTotal"`
	// Skipped 说明该场景为什么不可用；空串表示可用。
	Skipped string `json:"skipped"`
}

// SessionState 是当前会话（面向界面）。
type SessionState struct {
	Dir        string `json:"dir"`
	Name       string `json:"name"`
	ProjectDir string `json:"projectDir"`
	// Description 是 project.yml 里的描述。
	Description string `json:"description"`
	// Scenarios 是该项目的场景，按名称排序。
	Scenarios []ScenarioRef `json:"scenarios"`
	// Scenario 是当前场景名；null 表示未选中或该项目没有场景。
	Scenario *string `json:"scenario"`
	// ScenarioSkipped 是被跳过的场景目录及原因。**必须显示**：
	// 一场静默的跳过会让人以为「项目只有两个场景」。
	ScenarioSkipped []SkippedRef `json:"scenarioSkipped"`
}

// SkippedRef 是一个被跳过的场景目录。
type SkippedRef struct {
	Dir    string `json:"dir"`
	Reason string `json:"reason"`
}

// EntitySummary 是一个实体的规模。
type EntitySummary struct {
	Entity     string `json:"entity"`
	Subjects   int    `json:"subjects"`
	Assertions int    `json:"assertions"`
}

// ProjectOverview 是项目概览（面向界面）。
type ProjectOverview struct {
	Dir         string `json:"dir"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`

	Entities      []EntitySummary `json:"entities"`
	Assertions    int             `json:"assertions"`
	ByStatus      map[string]int  `json:"byStatus"`
	ByConfidence  map[string]int  `json:"byConfidence"`
	Verifications int             `json:"verifications"`
	Conflicts     int             `json:"conflicts"`
	Decisions     int             `json:"decisions"`

	ScenarioCount int `json:"scenarioCount"`
	FormulaCount  int `json:"formulaCount"`
	// EntityTypes 是 schema 中声明的实体类型数（含尚无数据的）。
	EntityTypes int `json:"entityTypes"`
}

// ProjectService 暴露项目与会话。
type ProjectService struct {
	session *compose.Session
}

// NewProjectService 构造服务。
func NewProjectService(s *compose.Session) *ProjectService { return &ProjectService{session: s} }

// Projects 列出项目根目录下的全部项目，按目录名排序。
func (s *ProjectService) Projects() ([]ProjectRef, error) {
	refs, err := projectfile.Discover(s.session.Root())
	if err != nil {
		return nil, err
	}
	cur := s.session.Dir()
	out := make([]ProjectRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, ProjectRef{
			Dir: r.Dir, Name: r.Display(), Path: r.Path,
			Description: r.File.Description, Current: r.Dir == cur,
		})
	}
	return out, nil
}

// Open 切换项目。**失败时会话不变**，因此界面不会塌成空白。
func (s *ProjectService) Open(dir string) (SessionState, error) {
	if dir == "" {
		return SessionState{}, fmt.Errorf("必须指明项目目录")
	}
	if err := s.session.Open(dir); err != nil {
		return SessionState{}, err
	}
	return s.Current()
}

// Current 返回当前会话。
func (s *ProjectService) Current() (SessionState, error) {
	p, err := s.session.Project()
	if err != nil {
		return SessionState{}, err
	}
	st := SessionState{
		Dir: p.Dir, Name: p.Name(), ProjectDir: p.Dir, Description: p.Description(),
		Scenarios: []ScenarioRef{}, ScenarioSkipped: []SkippedRef{},
	}
	for _, sc := range p.Scenarios {
		st.Scenarios = append(st.Scenarios, ScenarioRef{
			Name: sc.Name, Description: sc.Description,
			Inputs: len(sc.Inputs), RequiresTotal: len(sc.Requires),
		})
	}
	for _, sk := range p.ScenarioSkipped {
		st.ScenarioSkipped = append(st.ScenarioSkipped, SkippedRef{Dir: sk.Dir, Reason: sk.Reason})
	}
	if name := s.session.Scenario(); name != "" {
		st.Scenario = &name
	}
	return st, nil
}

// Overview 返回项目概览：规模、状态与分级分布、缺口。
func (s *ProjectService) Overview() (ProjectOverview, error) {
	p, err := s.session.Project()
	if err != nil {
		return ProjectOverview{}, err
	}
	ov := ProjectOverview{
		Dir: p.Dir, Name: p.Name(), Path: p.Dir, Description: p.Description(),
		Entities: []EntitySummary{}, ByStatus: map[string]int{}, ByConfidence: map[string]int{},
	}

	byEntity, err := p.Store.CountByEntity()
	if err != nil {
		return ov, err
	}
	// 以 schema 声明的实体为准，而不是以库里有数据的实体为准：
	// 「定义了但一条数据都没有」正是最该被看见的缺口，
	// 若只遍历库里有的实体，它会永远不出现。
	for _, name := range p.Schema.Names() {
		n, err := p.Store.SubjectCount(name)
		if err != nil {
			return ov, err
		}
		ov.Entities = append(ov.Entities, EntitySummary{
			Entity: name, Subjects: n, Assertions: byEntity[name],
		})
	}
	ov.EntityTypes = len(ov.Entities)

	if ov.Assertions, err = p.Store.Count(); err != nil {
		return ov, err
	}
	if ov.ByStatus, err = p.Store.StatusCounts(); err != nil {
		return ov, err
	}
	if ov.ByConfidence, err = p.Store.ConfidenceCounts(); err != nil {
		return ov, err
	}
	if ov.Verifications, err = p.Store.VerificationCount(); err != nil {
		return ov, err
	}
	conflicts, err := p.Store.Conflicts()
	if err != nil {
		return ov, err
	}
	ov.Conflicts = len(conflicts)

	counts, err := p.Store.DecisionCounts()
	if err != nil {
		return ov, err
	}
	for _, n := range counts {
		ov.Decisions += n
	}

	ov.ScenarioCount = len(p.Scenarios)
	files, err := p.FormulaFiles()
	if err != nil {
		return ov, err
	}
	ov.FormulaCount = len(files)
	return ov, nil
}
