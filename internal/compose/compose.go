// Package compose 是组合根：把领域、应用与基础设施装配成一个可用的项目。
//
// 它是唯一允许同时依赖各层的包——装配本来就需要看见全部零件。
// 其余包必须遵守 AGENTS.md 的分层铁律（api → application → domain ← infrastructure）。
package compose

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/infrastructure/schemafile"
	"github.com/ngnl5/ssot/internal/infrastructure/store"
)

// Project 是一个已装配的项目：定义（schema、单位）与实例（断言库）。
type Project struct {
	Dir    string
	Units  *unit.Table
	Schema *schema.Set
	Store  *store.Store
}

// Load 加载一个项目。
//
// withStore 为 false 时只加载定义，不开库——校验 schema 时不必创建文件。
// schema 校验不通过时返回全部问题，而不是只报第一个：
// 一次看到所有错误，比修一个跑一次快得多。
func Load(dir string, withStore bool) (*Project, error) {
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

	p := &Project{Dir: dir, Units: units, Schema: set}
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

// ScenarioDir 返回某场景的目录。
func (p *Project) ScenarioDir(name string) string { return filepath.Join(p.Dir, "scenarios", name) }
