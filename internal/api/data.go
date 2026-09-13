// 数据页的接口：断言库浏览、schema 与数据质量（见 docs/specs/workspace.spec.md）。
//
// 「质量」在这里有确切含义，不是形容词：
//
//	双向漂移  —— 数据里有而 schema 没声明的字段 / schema 声明而数据里从未出现的字段
//	填充率    —— 每个已声明字段在多少条记录里出现过
//	覆盖率    —— 每个谓词覆盖了多少个主体
//
// 三者都必须能追溯到具体字段名。只给一个「质量分」是没有用的：
// 它既不能告诉人哪里有洞，也不能在洞被补上之后证明它真的补上了。
package api

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/metamodel"
	"sort"
)

// FieldSchema 是一个字段的声明（面向界面）。
type FieldSchema struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Unit        string   `json:"unit"`
	Required    bool     `json:"required"`
	Identity    bool     `json:"identity"`
	Unique      bool     `json:"unique"`
	Target      string   `json:"target"`
	Values      []string `json:"values"`
	Min         *float64 `json:"min"`
	Max         *float64 `json:"max"`
}

// EntitySchema 是一个实体的声明（面向界面）。
type EntitySchema struct {
	Entity      string        `json:"entity"`
	Description string        `json:"description"`
	SchemaRev   string        `json:"schemaRev"`
	Fields      []FieldSchema `json:"fields"`
}

// FieldQuality 是一个字段的数据质量。
type FieldQuality struct {
	Key string `json:"key"`
	// Declared 报告该字段是否在 schema 中声明。
	//
	// 未声明的字段出现在数据里就是**漂移**：它会被准入层过滤掉，
	// 于是数据「看起来接进来了」但根本没入库。
	Declared bool `json:"declared"`
	Required bool `json:"required"`
	// Coverage 是覆盖的主体数占比（0..1）；未声明字段为 0。
	Coverage float64 `json:"coverage"`
	// Subjects 是出现该字段的主体数。
	Subjects int `json:"subjects"`
}

// EntityQuality 是一个实体的数据质量。
type EntityQuality struct {
	Entity     string         `json:"entity"`
	Subjects   int            `json:"subjects"`
	Assertions int            `json:"assertions"`
	Fields     []FieldQuality `json:"fields"`
	// Undeclared 是数据中出现、schema 未声明的字段。
	Undeclared []string `json:"undeclared"`
	// Unused 是 schema 声明、数据中从未出现的字段。
	Unused []string `json:"unused"`
	// RequiredMissing 是必填但没有任何数据的主体级字段。
	RequiredMissing []string `json:"requiredMissing"`
}

// AssertionPage 是一页断言。
type AssertionPage struct {
	Items  []Item `json:"items"`
	Total  int    `json:"total"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
	Where  string `json:"where"`
}

// DataService 暴露断言库与质量。
type DataService struct {
	session *compose.Session
}

// NewDataService 构造服务。
func NewDataService(s *compose.Session) *DataService { return &DataService{session: s} }

// Schema 返回全部实体声明。
func (s *DataService) Schema() ([]EntitySchema, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	out := make([]EntitySchema, 0, len(p.Schema.Names()))
	for _, name := range p.Schema.Names() {
		e, _ := p.Schema.Lookup(name)
		es := EntitySchema{
			Entity: e.Name, Description: e.Description, SchemaRev: e.SchemaRev,
			Fields: make([]FieldSchema, 0, len(e.Fields)),
		}
		for _, f := range e.Fields {
			es.Fields = append(es.Fields, FieldSchema{
				Key: f.Key, Description: f.Description, Type: string(f.Type),
				Unit: f.Unit, Required: f.Required, Identity: f.Identity,
				Unique: f.Unique, Target: f.Target, Values: f.Values,
				Min: f.Min, Max: f.Max,
			})
		}
		out = append(out, es)
	}
	return out, nil
}

// Quality 返回每个实体的数据质量。
//
// 以 schema 声明的实体为准：**「声明了却没有数据」正是最该被看见的缺口**，
// 只遍历库里有数据的实体，它永远不会出现。
func (s *DataService) Quality() ([]EntityQuality, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	out := make([]EntityQuality, 0, len(p.Schema.Names()))
	for _, name := range p.Schema.Names() {
		e, ok := p.Schema.Lookup(name)
		if !ok {
			continue
		}
		q, err := entityQuality(p, e)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

func entityQuality(p *compose.Project, e *metamodel.Entity) (EntityQuality, error) {
	q := EntityQuality{
		Entity: e.Name, Fields: []FieldQuality{},
		Undeclared: []string{}, Unused: []string{}, RequiredMissing: []string{},
	}

	subjects, err := p.Store.SubjectCount(e.Name)
	if err != nil {
		return q, err
	}
	q.Subjects = subjects

	byPred, err := p.Store.PredicateSubjectCounts(e.Name)
	if err != nil {
		return q, err
	}
	byEntity, err := p.Store.CountByEntity()
	if err != nil {
		return q, err
	}
	q.Assertions = byEntity[e.Name]

	declared := map[string]metamodel.Field{}
	for _, f := range e.Fields {
		declared[f.Key] = f
	}

	// 已声明字段
	for _, f := range e.Fields {
		n := byPred[f.Key]
		cov := 0.0
		if subjects > 0 {
			cov = float64(n) / float64(subjects)
		}
		q.Fields = append(q.Fields, FieldQuality{
			Key: f.Key, Declared: true, Required: f.Required, Coverage: cov, Subjects: n,
		})
		if n == 0 {
			q.Unused = append(q.Unused, f.Key)
			if f.Required {
				q.RequiredMissing = append(q.RequiredMissing, f.Key)
			}
		}
	}

	// 数据里出现、schema 未声明的字段
	for key, n := range byPred {
		if _, ok := declared[key]; ok {
			continue
		}
		q.Undeclared = append(q.Undeclared, key)
		cov := 0.0
		if subjects > 0 {
			cov = float64(n) / float64(subjects)
		}
		q.Fields = append(q.Fields, FieldQuality{
			Key: key, Declared: false, Coverage: cov, Subjects: n,
		})
	}
	sort.Strings(q.Undeclared)
	sort.Strings(q.Unused)
	sort.Strings(q.RequiredMissing)
	return q, nil
}

// Assertions 分页返回断言。
func (s *DataService) Assertions(f FilterInput, limit, offset int) (AssertionPage, error) {
	p, err := s.session.Project()
	if err != nil {
		return AssertionPage{}, err
	}
	if limit <= 0 {
		limit = 100
	}
	df := f.toDomain()
	as, err := p.Store.SelectRange(df, limit, offset)
	if err != nil {
		return AssertionPage{}, err
	}
	total, err := p.Store.CountFiltered(df)
	if err != nil {
		return AssertionPage{}, err
	}
	page := AssertionPage{
		Items: make([]Item, 0, len(as)), Total: total, Offset: offset, Limit: limit,
		Where: df.Describe(),
	}
	for _, a := range as {
		page.Items = append(page.Items, toItem(a))
	}
	return page, nil
}

// SubjectDetail 返回某主体上的全部断言。
func (s *DataService) SubjectDetail(entity, subject string) ([]Item, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	if entity == "" || subject == "" {
		return nil, fmt.Errorf("需要实体与主体")
	}
	as, err := p.Store.BySubject(entity, subject)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(as))
	for _, a := range as {
		out = append(out, toItem(a))
	}
	return out, nil
}

// Subjects 返回某实体下的全部主体，供界面做主体选择。
func (s *DataService) Subjects(entity string) ([]string, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	if entity == "" {
		return nil, fmt.Errorf("需要实体名")
	}
	out, err := p.Store.Subjects(entity)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// EntityNames 返回 schema 中声明的实体名。
func (s *DataService) EntityNames() ([]string, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	return p.Schema.Names(), nil
}
