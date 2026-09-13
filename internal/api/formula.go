// 公式页的接口：公式清单与算例验证状态（见 docs/specs/workspace.spec.md）。
//
// 公式状态三级，含义不重叠：
//
//	算例全过 -> 已验证，可用
//	没有算例 -> 允许用，但产出必须标注「公式未验证」
//	算例未过 -> **拒绝使用**
//
// 界面上三者必须一眼可分：把「无算例」显示成绿色，等于把未验证的公式
// 伪装成验证过的。
package api

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/compose"
)

// FormulaParamView 是一个公式参数的静态声明。
type FormulaParamView struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Unit string `json:"unit"`
	Note string `json:"note"`
}

// FormulaCaseView 是一条算例的结果。
type FormulaCaseView struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Got    string `json:"got"`
	Want   string `json:"want"`
	Detail string `json:"detail"`
}

// FormulaView 是一条公式（面向界面）。
type FormulaView struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// Result 是结果表达式的**原文**。界面把它显示出来，
	// 因为一个不透明的公式没法被审阅——而算错的公式比算错的数据更难发现。
	Result   string             `json:"result"`
	Params   []FormulaParamView `json:"params"`
	Bindings []BindingView      `json:"bindings"`
	// Status 是 verified / unverified / failed / unreadable。
	Status string            `json:"status"`
	Cases  []FormulaCaseView `json:"cases"`
	// Error 仅在公式读不出来时非空。
	Error string `json:"error"`
	// File 是它来自哪个文件，便于去改。
	File string `json:"file"`
	// UsedBy 是哪些场景在用它。
	UsedBy []string `json:"usedBy"`
}

// BindingView 是「绑定名 → 取值来源路径」。
type BindingView struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FormulaService 暴露公式。
type FormulaService struct {
	session *compose.Session
}

// NewFormulaService 构造服务。
func NewFormulaService(s *compose.Session) *FormulaService { return &FormulaService{session: s} }

// List 返回项目里的全部公式及其验证状态。
//
// **逐个文件加载**而不是一次 LoadDir：一条写坏的公式不该让整页打不开，
// 它应该被单独指出是哪一条、错在哪。
func (s *FormulaService) List() ([]FormulaView, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	files, err := p.FormulaFiles()
	if err != nil {
		return nil, err
	}
	out := make([]FormulaView, 0, len(files))
	for _, path := range files {
		out = append(out, loadFormulaView(p, path))
	}
	return out, nil
}

func loadFormulaView(p *compose.Project, path string) FormulaView {
	v := FormulaView{
		Name:   strings.TrimSuffix(filepath.Base(path), ".formula.yml"),
		Params: []FormulaParamView{}, Bindings: []BindingView{},
		Cases: []FormulaCaseView{}, UsedBy: []string{}, File: path,
		Status: "unreadable",
	}
	f, err := formula.Load(path, p.Units)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	v.Name = f.Name
	v.Version = f.Version
	v.Description = f.Description
	v.Result = f.Result
	v.Error = ""

	for _, name := range sortedKeysString(f.Params) {
		ps := f.Params[name]
		v.Params = append(v.Params, FormulaParamView{
			Name: name, Type: ps.Type, Unit: ps.Unit, Note: ps.Note,
		})
	}
	for _, name := range sortedKeysString(f.Bindings) {
		v.Bindings = append(v.Bindings, BindingView{Name: name, Path: f.Bindings[name]})
	}

	cases, status := f.Verify(p.Units)
	v.Status = string(status)
	for _, c := range cases {
		detail := "通过"
		if !c.Passed {
			detail = fmt.Sprintf("期望 %s，实际 %s", c.Want, c.Got)
			if c.Err != nil {
				detail = c.Err.Error()
			}
		}
		v.Cases = append(v.Cases, FormulaCaseView{
			Name: c.Name, Passed: c.Passed, Got: c.Got, Want: c.Want, Detail: detail,
		})
	}
	for _, sc := range p.Scenarios {
		for _, used := range sc.Formulas {
			if used == f.Name {
				v.UsedBy = append(v.UsedBy, sc.Name)
			}
		}
	}
	return v
}

func sortedKeysString[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// 参数顺序必须稳定：每次打开页面顺序不同，人就无法判断「和上次是不是一样」。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
