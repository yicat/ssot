// Package compose 是组合根：把领域、应用与基础设施装配成一个可用的项目。
//
// 它是唯一允许同时依赖各层的包——装配本来就需要看见全部零件。
// 其余包必须遵守 AGENTS.md 的分层铁律（api → application → domain ← infrastructure）。
package compose

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/application/scenario"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/infrastructure/projectfile"
	"github.com/ngnl5/ssot/internal/infrastructure/schemafile"
	"github.com/ngnl5/ssot/internal/infrastructure/store"
)

// Project 是一个已装配的项目：定义（schema、单位、场景）与实例（断言库）。
type Project struct {
	Dir    string
	File   projectfile.File
	Units  *unit.Table
	Schema *schema.Set
	Store  *store.Store

	// Scenarios 是该项目的场景声明，按名称排序。
	//
	// 场景**不拥有数据**：它们读同一批断言，只组织「读什么、算什么、产出什么」。
	// 因此场景列表属于项目定义，而非某个存储。
	Scenarios []scenario.Spec
	// ScenarioSkipped 是被跳过的场景目录及原因。**必须显示出来**：
	// 一场静默的跳过会让人以为「项目只有两个场景」。
	ScenarioSkipped []scenario.Skipped
}

// Name 返回项目名。
func (p *Project) Name() string {
	if p.File.Project != "" {
		return p.File.Project
	}
	return filepath.Base(p.Dir)
}

// Description 返回项目描述。
func (p *Project) Description() string { return p.File.Description }

// Scenario 按名称取场景声明。
func (p *Project) Scenario(name string) (scenario.Spec, bool) {
	return scenario.Find(p.Scenarios, name)
}

// Load 加载一个项目。
//
// withStore 为 false 时只加载定义，不开库——校验 schema 时不必创建文件。
// schema 校验不通过时返回全部问题，而不是只报第一个：
// 一次看到所有错误，比修一个跑一次快得多。
func Load(dir string, withStore bool) (*Project, error) {
	f, err := projectfile.Load(filepath.Join(dir, projectfile.Name))
	if err != nil {
		if !withStore {
			// 只校验 schema 的场景下允许没有 project.yml，
			// 但那时也拿不到项目名——由调用方自己决定怎么显示。
			f = projectfile.File{}
		} else {
			return nil, fmt.Errorf("加载项目定义：%w", err)
		}
	}

	units, err := schemafile.LoadUnits(filepath.Join(dir, "units.yml"))
	if err != nil {
		return nil, fmt.Errorf("加载单位表：%w", err)
	}
	set, problems, err := schemafile.LoadSet(filepath.Join(dir, "schema"), units)
	if err != nil {
		return nil, fmt.Errorf("加载 schema：%w", err)
	}
	if len(problems) > 0 {
		var sb strings.Builder
		sb.WriteString("schema 校验未通过：\n")
		for _, p := range problems {
			sb.WriteString("  " + p.String() + "\n")
		}
		return nil, fmt.Errorf("%s", sb.String())
	}

	specs, skipped, err := scenario.ProjectScenarios(dir)
	if err != nil {
		return nil, fmt.Errorf("加载场景：%w", err)
	}

	p := &Project{
		Dir: dir, File: f, Units: units, Schema: set,
		Scenarios: specs, ScenarioSkipped: skipped,
	}
	if withStore {
		st, err := store.Open(filepath.Join(dir, ".data", "store.db"))
		if err != nil {
			return nil, err
		}
		p.Store = st
	}
	return p, nil
}

// Close 释放资源。
func (p *Project) Close() error {
	if p.Store != nil {
		return p.Store.Close()
	}
	return nil
}

// FormulasDir 返回公式目录。
func (p *Project) FormulasDir() string { return filepath.Join(p.Dir, "formulas") }

// FormulaFiles 返回公式文件的路径，按名称排序。
//
// 只列文件、不编译：概览页要的是「有几条公式」，一个写坏的公式不该
// 让整个概览打不开——它应该在公式页上被单独指出。
func (p *Project) FormulaFiles() ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(p.FormulasDir(), "*.formula.yml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// ScenarioDir 返回某场景的目录。
func (p *Project) ScenarioDir(name string) string { return filepath.Join(p.Dir, "scenarios", name) }
