// 词表：把原始标识符翻成看得懂的词。
//
// 界面上原本到处是 `shikigami.atk`、`L2`、`percent`、`human:ngnl5` 这类
// 标识符——它们**准确但不可读**。而项目定义里其实早就有中文：schema 的
// `description`、单位表的名字、领域枚举的中文名。词表只是把它们取出来。
//
// 两条原则：
//
//	不编词 —— 定义里没有中文的（例如货币单位 gem），界面退回显示原始标识，
//	          而不是起一个听起来合理的名字
//	双语并出 —— 中文名给人看，原始标识给对账用，两者都要在
package api

import (
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/alternatives"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/decision"
	"github.com/ngnl5/ssot/internal/domain/experience"
)

// Term 是一个词条：给人看的名字 + 完整说明。
type Term struct {
	// Key 是原始标识符。**必须一起显示**——中文名给人看，标识符给对账用。
	Key string `json:"key"`
	// Label 是短名。空串表示定义里没有中文，界面应退回显示 Key。
	Label string `json:"label"`
	// Note 是完整说明。
	Note string `json:"note"`
	// Extra 是附加信息（单位、类型、目标实体等），可为空。
	Extra string `json:"extra"`
}

// Glossary 是本项目的词表。
type Glossary struct {
	// Entities 是实体类型：shikigami -> 式神。
	Entities map[string]Term `json:"entities"`
	// Fields 是字段：`shikigami.atk` -> 攻击。
	Fields map[string]Term `json:"fields"`
	// Units 是单位：percent -> 百分比。
	Units map[string]Term `json:"units"`
	// Types 是字段类型：number -> 数值。
	Types map[string]Term `json:"types"`
	// Subjects 是主体的显示名：`shikigami/262` -> 姑获鸟。
	//
	// 它是**从数据里读出来的**，不是编的：主体标识是稳定身份，
	// 而名字就在同一条记录上。
	Subjects map[string]string `json:"subjects"`

	Confidences  map[string]Term `json:"confidences"`
	Statuses     map[string]Term `json:"statuses"`
	ActorKinds   map[string]Term `json:"actorKinds"`
	ExpKinds     map[string]Term `json:"expKinds"`
	ExpStatuses  map[string]Term `json:"expStatuses"`
	DecStatuses  map[string]Term `json:"decStatuses"`
	PlanStatuses map[string]Term `json:"planStatuses"`
	Methods      map[string]Term `json:"methods"`
}

// Glossary 返回本项目的词表。
func (s *ProjectService) Glossary() (Glossary, error) {
	p, err := s.session.Project()
	if err != nil {
		return Glossary{}, err
	}
	g := Glossary{
		Entities: map[string]Term{}, Fields: map[string]Term{},
		Units: map[string]Term{}, Types: map[string]Term{},
		Subjects:     map[string]string{},
		Confidences:  map[string]Term{},
		Statuses:     map[string]Term{},
		ActorKinds:   map[string]Term{},
		ExpKinds:     map[string]Term{},
		ExpStatuses:  map[string]Term{},
		DecStatuses:  map[string]Term{},
		PlanStatuses: map[string]Term{},
		Methods:      map[string]Term{},
	}

	// 实体与字段：中文直接取自 schema 的 description
	for _, name := range p.Schema.Names() {
		e, ok := p.Schema.Lookup(name)
		if !ok {
			continue
		}
		g.Entities[name] = Term{Key: name, Label: e.Description, Note: e.Description}
		for _, f := range e.Fields {
			key := name + "." + f.Key
			extra := string(f.Type.Label())
			if f.Unit != "" {
				extra += " · " + unitLabel(p, f.Unit)
			}
			if f.Target != "" {
				extra += " · 指向 " + entityLabel(g, f.Target)
			}
			g.Fields[key] = Term{
				Key: key, Label: f.Description, Note: f.Description, Extra: extra,
			}
		}
	}

	// 单位
	for _, u := range p.Units.All() {
		label := u.Label
		note := ""
		if label == "" {
			// 定义里没有中文就不编：退回维度名，并说明为什么没有名字。
			label = ""
			note = "引擎未给该单位中文名（维度：" + u.Dimension.Label() + "）；项目可在 units.yml 里补"
		} else {
			note = u.Dimension.Label() + "维度"
		}
		g.Units[u.Name] = Term{Key: u.Name, Label: label, Note: note, Extra: u.Dimension.Label()}
	}

	// 字段类型
	for _, t := range []string{"text", "number", "bool", "enum", "ref", "object", "list"} {
		g.Types[t] = Term{Key: t, Label: typeLabel(t)}
	}

	// 主体显示名：从数据里的 name 谓词读
	g.Subjects = subjectNames(p)

	// 分级与状态
	for _, c := range []assertion.Confidence{assertion.L1, assertion.L2, assertion.L3, assertion.L4} {
		g.Confidences[string(c)] = Term{Key: string(c), Label: c.Label(), Note: c.Hint()}
	}
	for _, st := range []assertion.Status{
		assertion.StatusPending, assertion.StatusAutoChecked, assertion.StatusVerified,
		assertion.StatusDisputed, assertion.StatusRejected, assertion.StatusExpired,
		assertion.StatusUnmodeled,
	} {
		g.Statuses[string(st)] = Term{Key: string(st), Label: st.Label()}
	}
	g.ActorKinds["human"] = Term{Key: "human", Label: "人"}
	g.ActorKinds["agent"] = Term{Key: "agent", Label: "机器"}

	for _, k := range []experience.Kind{
		experience.KindDerived, experience.KindJudgment, experience.KindSummary,
	} {
		g.ExpKinds[string(k)] = Term{Key: string(k), Label: k.Label()}
	}
	for _, st := range []experience.Status{
		experience.StatusCandidate, experience.StatusEffective, experience.StatusRejected,
		experience.StatusStale, experience.StatusRecompute,
	} {
		g.ExpStatuses[string(st)] = Term{Key: string(st), Label: st.Label()}
	}
	g.ExpStatuses["superseded"] = Term{Key: "superseded", Label: "已被取代"}

	for _, st := range []decision.Status{
		decision.StatusOpen, decision.StatusDeferred, decision.StatusDecided, decision.StatusStale,
	} {
		g.DecStatuses[string(st)] = Term{Key: string(st), Label: st.Label()}
	}
	for _, st := range []alternatives.Status{
		alternatives.StatusCandidate, alternatives.StatusChosen, alternatives.StatusStale,
		alternatives.StatusInvalid, alternatives.StatusSuperseded,
	} {
		g.PlanStatuses[string(st)] = Term{Key: string(st), Label: st.Label()}
	}
	return g, nil
}

// subjectNames 读每个主体的 name 断言，得到「标识 → 名字」。
//
// 读不到名字的主体不出现在词表里——界面那时显示标识，
// 而不是显示一个空字符串让人以为数据坏了。
func subjectNames(p *compose.Project) map[string]string {
	out := map[string]string{}
	for _, name := range p.Schema.Names() {
		e, ok := p.Schema.Lookup(name)
		if !ok {
			continue
		}
		if _, has := e.FieldByKey("name"); !has {
			continue
		}
		as, err := p.Store.ByEntity(name)
		if err != nil {
			continue
		}
		for _, a := range as {
			if a.Predicate != "name" || !a.Value.IsPresent() {
				continue
			}
			if s, ok := a.Value.Data.(string); ok && s != "" {
				out[name+"/"+a.Subject] = s
			}
		}
	}
	return out
}

func unitLabel(p *compose.Project, name string) string {
	if u, ok := p.Units.Lookup(name); ok && u.Label != "" {
		return u.Label + "（" + name + "）"
	}
	return name
}

func entityLabel(g Glossary, name string) string {
	if t, ok := g.Entities[name]; ok && t.Label != "" {
		return t.Label + "（" + name + "）"
	}
	return name
}

func typeLabel(t string) string {
	switch t {
	case "text":
		return "文本"
	case "number":
		return "数值"
	case "bool":
		return "布尔"
	case "enum":
		return "枚举"
	case "ref":
		return "引用"
	case "object":
		return "对象"
	case "list":
		return "列表"
	}
	return t
}

// Unused 报告词表里没有被任何字段引用的实体（供体检用）。
func (g Glossary) Unused() []string {
	var out []string
	for name := range g.Entities {
		used := false
		for key := range g.Fields {
			if strings.HasPrefix(key, name+".") {
				used = true
				break
			}
		}
		if !used {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
