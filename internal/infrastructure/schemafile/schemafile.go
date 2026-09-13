// Package schemafile 从 YAML 加载 schema 与单位表。
//
// 见 docs/specs/project.spec.md：schema 是**项目的定义部分**——
// 由人编写、可版本控制、可分享。引擎不内置任何领域概念。
package schemafile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ngnl5/ssot/internal/domain/metamodel"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"

	"gopkg.in/yaml.v3"
)

// File 是一个 schema 文件。
type File struct {
	Entity           string  `yaml:"entity"`
	Description      string  `yaml:"description"`
	MetamodelVersion int     `yaml:"metamodelVersion"`
	SchemaRev        string  `yaml:"schemaRev"`
	Fields           []Field `yaml:"fields"`
}

// Field 是一个字段声明。
type Field struct {
	Key       string   `yaml:"key"`
	Desc      string   `yaml:"description"`
	Type      string   `yaml:"type"`
	Required  bool     `yaml:"required"`
	Identity  bool     `yaml:"identity"`
	Unique    bool     `yaml:"unique"`
	Unit      string   `yaml:"unit"`
	Min       *float64 `yaml:"min"`
	Max       *float64 `yaml:"max"`
	Precision *int     `yaml:"precision"`
	Values    []string `yaml:"values"`
	Target    string   `yaml:"target"`
	Items     *Field   `yaml:"items"`
	Fields    []Field  `yaml:"fields"`
}

// UnitsFile 是单位表文件。
type UnitsFile struct {
	Units []UnitDef `yaml:"units"`
}

// UnitDef 是一个单位定义。
type UnitDef struct {
	Name      string  `yaml:"name"`
	Dimension string  `yaml:"dimension"`
	Scale     float64 `yaml:"scale"`
}

// LoadUnits 加载单位表。未提供文件时返回内置表。
func LoadUnits(path string) (*unit.Table, error) {
	base := unit.Default()
	if path == "" {
		return base, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, err
	}
	var f UnitsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, u := range f.Units {
		if u.Name == "" {
			return nil, fmt.Errorf("%s: 单位缺少 name", path)
		}
		if u.Dimension == "" {
			return nil, fmt.Errorf("%s: 单位 %s 缺少 dimension", path, u.Name)
		}
		base.Register(unit.Unit{Name: u.Name, Dimension: unit.Dimension(u.Dimension), Scale: u.Scale})
	}
	return base, nil
}

// LoadSet 加载目录下的全部 schema 文件并校验。
//
// 返回的 problems 非空时调用方应拒绝加载——**严格模式**：
// schema 是我们自己写的，typo 必须被抓到。
func LoadSet(dir string, ur metamodel.UnitRegistry) (*schema.Set, []metamodel.Problem, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.schema.yml"))
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("%s 下没有找到 *.schema.yml", dir)
	}
	sort.Strings(paths)

	var entities []*metamodel.Entity
	var problems []metamodel.Problem

	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		var f File
		if err := yaml.Unmarshal(b, &f); err != nil {
			problems = append(problems, metamodel.Problem{Path: p, Reason: "YAML 解析失败：" + err.Error()})
			continue
		}
		e := f.toEntity()
		if ps := e.Validate(ur); len(ps) > 0 {
			for _, pr := range ps {
				problems = append(problems, metamodel.Problem{Path: p + " " + pr.Path, Reason: pr.Reason})
			}
			continue
		}
		entities = append(entities, e)
	}

	if len(problems) > 0 {
		return nil, problems, nil
	}

	set, ps := schema.NewSet(entities...)
	if len(ps) > 0 {
		return nil, ps, nil
	}
	return set, nil, nil
}

func (f File) toEntity() *metamodel.Entity {
	e := &metamodel.Entity{
		Name:        f.Entity,
		Description: f.Description,
		MetaVersion: f.MetamodelVersion,
		SchemaRev:   f.SchemaRev,
		Fields:      make([]metamodel.Field, 0, len(f.Fields)),
	}
	for _, fl := range f.Fields {
		e.Fields = append(e.Fields, fl.toField())
	}
	return e
}

func (f Field) toField() metamodel.Field {
	out := metamodel.Field{
		Key:         f.Key,
		Description: f.Desc,
		Type:        metamodel.TypeName(f.Type),
		Required:    f.Required,
		Identity:    f.Identity,
		Unique:      f.Unique,
		Min:         f.Min,
		Max:         f.Max,
		Precision:   f.Precision,
		Unit:        f.Unit,
		Values:      f.Values,
		Target:      f.Target,
	}
	if f.Items != nil {
		it := f.Items.toField()
		out.Items = &it
	}
	for _, sub := range f.Fields {
		out.Fields = append(out.Fields, sub.toField())
	}
	return out
}
