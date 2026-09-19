// raw 层（以及 docs/tables）的**管理视图**：按目录分组看体量与派生占用。
//
// 为什么要有它：人要知道「哪一层占了什么」，Agent 要拿**可统计的事实**去提建议
// （「这层 125 篇、占 70% 字符量、派生层里 12,509 行都来自它」），而不是凭目录名猜。
// 这里只统计，不下判断——判断在声明文件里（docs/specs/derived.spec.md）。
package vaultapp

import (
	"path"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// GroupStat 是一组（一个目录）的体量与派生占用。
type GroupStat struct {
	Dir       string // 目录（如 raw/剧情、docs/式神；单层文件归到它的父目录）
	Docs      int
	Chars     int // 所有文档的正文总字符数（体量）
	Chunks    int // 派生层里的块（512 口径）
	Extracts  int // 抽取块（2000 口径）
	Entities  int // 实体来源行
	Relations int // 关系来源行
	Inbound   int // 被**别的组**的文档链过来的次数（判断「能不能安全删」的依据）
}

// GroupStats 统计各目录的体量与派生占用，按体量降序（大的在前，便于一眼看出大头）。
func (s *Service) GroupStats() ([]GroupStat, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return nil, err
	}
	groupOf := map[string]string{} // doc → 组
	stats := map[string]*GroupStat{}
	for _, d := range docs {
		g := groupDir(d.Path)
		groupOf[d.Path] = g
		st := stats[g]
		if st == nil {
			st = &GroupStat{Dir: g}
			stats[g] = st
		}
		st.Docs++
		st.Chars += len([]rune(d.Body))
	}

	// 引用：按「链接目标的文件名」匹配到组（与删除时的断链判定同一套宽松口径）。
	baseToGroup := map[string]string{}
	for _, d := range docs {
		baseToGroup[strings.TrimSuffix(path.Base(d.Path), ".md")] = groupOf[d.Path]
	}
	for _, d := range docs {
		from := groupOf[d.Path]
		seen := map[string]bool{}
		for _, l := range d.Links {
			got := strings.TrimSuffix(vault.NormalizeSlash(l.Target), ".md")
			if got == "" {
				continue
			}
			g, ok := baseToGroup[path.Base(got)]
			if !ok || g == from || seen[g] {
				continue
			}
			seen[g] = true
			stats[g].Inbound++
		}
	}

	// 派生占用：按 doc 前缀数（用 substr 比 LIKE 稳，文档名里可能有 `%`）。
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	for g, st := range stats {
		prefix := g + "/"
		for _, q := range []struct {
			table string
			dst   *int
		}{
			{"chunk", &st.Chunks},
			{"extract_chunk", &st.Extracts},
			{"entity", &st.Entities},
			{"relation", &st.Relations},
		} {
			n, err := s.index.CountByDocPrefix(q.table, prefix, g)
			if err != nil {
				return nil, err
			}
			*q.dst = n
		}
	}

	out := make([]GroupStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Chars != out[j].Chars {
			return out[i].Chars > out[j].Chars
		}
		return out[i].Dir < out[j].Dir
	})
	return out, nil
}

// groupDir 取一组文档所属的目录：`a/b/c.md` → `a/b`；`a/x.md` → `a`。
func groupDir(rel string) string {
	rel = vault.NormalizeSlash(rel)
	dir := path.Dir(rel)
	if dir == "." || dir == "/" {
		return "(根)"
	}
	return dir
}

// Restore 把一篇**被删掉的文档**从 git 历史里恢复回来。
//
// 删除是普通操作，那回滚也该一样顺手：删错了不该只能手工翻 git。
// 恢复只动文件；派生层会在下次 index/extract 时把它补回来（增量在 P5 收口）。
func (s *Service) Restore(rel string, actor vault.Actor) (Change, error) {
	norm := vault.NormalizeSlash(rel)
	if norm == "" {
		return Change{}, errNeedPath
	}
	if actor.Kind == "" {
		return Change{}, errNeedActor
	}
	if ok, _ := s.loader.Exists(norm); ok {
		return Change{}, errAlreadyThere
	}
	if !s.git.IsRepo() {
		return Change{}, errNotRepo
	}
	if err := s.git.RestoreDeleted(norm); err != nil {
		return Change{}, err
	}
	change := Change{Path: norm, Actor: actor, Action: "restore"}
	s.version(&change)
	return change, nil
}

var (
	errNeedPath     = errString("没给路径")
	errNeedActor    = errString("恢复必须给 -actor（谁恢复的要留痕）")
	errAlreadyThere = errString("文件还在，不需要恢复")
	errNotRepo      = errString("vault 不是独立的 git 仓库，没法从历史恢复")
)

type errString string

func (e errString) Error() string { return string(e) }
