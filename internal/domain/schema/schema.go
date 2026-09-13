// Package schema 管理实体类型集合，并提供**双向**漂移检测。
//
// 见 docs/specs/core.spec.md。漂移必须双向检查：
//
//	数据 → schema：数据中出现的字段未被声明
//	schema → 数据：schema 声明的字段在数据中从未出现
//
// 实测依据：某 wiki 的 schema 声明 13 个字段而数据中实际出现 15 个，
// 两个方向同时存在漂移。
package schema

import (
	"sort"

	"github.com/ngnl5/ssot/internal/domain/metamodel"
)

// Set 是实体类型的集合。
type Set struct {
	entities map[string]*metamodel.Entity
}

// NewSet 构造集合并校验每个实体。返回的问题若不为空，调用方应拒绝加载。
func NewSet(entities ...*metamodel.Entity) (*Set, []metamodel.Problem) {
	s := &Set{entities: map[string]*metamodel.Entity{}}
	var ps []metamodel.Problem
	for _, e := range entities {
		if _, dup := s.entities[e.Name]; dup {
			ps = append(ps, metamodel.Problem{Path: "name", Reason: "实体名重复：" + e.Name})
			continue
		}
		s.entities[e.Name] = e
	}
	return s, ps
}

// Lookup 按名查找实体。
func (s *Set) Lookup(name string) (*metamodel.Entity, bool) {
	e, ok := s.entities[name]
	return e, ok
}

// Names 返回全部实体名（排序）。
func (s *Set) Names() []string {
	out := make([]string, 0, len(s.entities))
	for n := range s.entities {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Drift 是某个实体上检出的漂移。
type Drift struct {
	Entity string
	// Undeclared：数据中出现的字段，schema 未声明（数据 → schema）
	Undeclared []string
	// Unused：schema 声明的字段，数据中从未出现（schema → 数据）
	Unused []string
	// FillRate：每个已声明字段的填充率（0..1）。仅统计出现次数。
	FillRate map[string]float64
	// Samples：参与统计的记录数
	Samples int
}

// HasAny 报告是否存在任一方向的漂移。
func (d Drift) HasAny() bool { return len(d.Undeclared) > 0 || len(d.Unused) > 0 }

// DetectDrift 对一批观测到的记录做双向漂移检测。
//
// records 是每条记录上实际出现的顶层字段名集合。
// 注意：**字段出现 ≠ 字段已填充**——空值与未知值由调用方在 values 层区分，
// 本函数只做键的存在性统计。
func DetectDrift(e *metamodel.Entity, records []map[string]bool) Drift {
	d := Drift{Entity: e.Name, FillRate: map[string]float64{}, Samples: len(records)}
	if len(records) == 0 {
		return d
	}

	declared := map[string]bool{}
	for _, f := range e.Fields {
		declared[f.Key] = true
	}

	undeclared := map[string]bool{}
	counts := map[string]int{}
	for _, r := range records {
		for k := range r {
			counts[k]++
			if !declared[k] {
				undeclared[k] = true
			}
		}
	}

	for _, f := range e.Fields {
		d.FillRate[f.Key] = float64(counts[f.Key]) / float64(len(records))
		if counts[f.Key] == 0 {
			d.Unused = append(d.Unused, f.Key)
		}
	}
	for k := range undeclared {
		d.Undeclared = append(d.Undeclared, k)
	}

	sort.Strings(d.Undeclared)
	sort.Strings(d.Unused)
	return d
}
