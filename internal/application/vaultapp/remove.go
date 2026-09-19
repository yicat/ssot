// 删除用例：删掉一篇文档（或一批），并把派生层里的残骸一起清掉。
//
// 「删就是删」：不搞隔离区、不搞审批队列。该有的只有本来就有的那几件——
//   - **git 留痕**：走既有的 version()（文件删掉后 `git add -- rel` 会把删除入暂存区）；
//   - **actor 记录**：谁删的（human / agent）进 commit trailer，和写入同一条规矩；
//   - **派生层跟着清**：块 / 向量 / 实体 / 关系不留残骸；
//   - **断链要说出来**：删完把「链到它的文档」报给调用方，别静默留下断链。
package vaultapp

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// RemoveResult 是一次删除的结果。
type RemoveResult struct {
	Path string
	// DerivedRows 是被清掉的派生行数（块 + 向量 + 抽取块 + 同步 + 实体 + 关系）。
	DerivedRows int
	// BrokenLinks 是**删完之后**指向它的双链来源（相对路径），要报给人看。
	BrokenLinks []string
	// Change 里带 git 留痕结果（Committed / CommitSHA / VersionNote）。
	Change Change
}

// WouldRemove 是「先看后删」：算一份影响，不动物。
func (s *Service) WouldRemove(rel string) (RemoveResult, error) {
	norm, err := s.checkDoc(rel)
	if err != nil {
		return RemoveResult{}, err
	}
	res := RemoveResult{Path: norm}
	if err := s.ensureIndex(); err != nil {
		return res, err
	}
	if st, err := s.index.PurgePreview(norm); err == nil {
		res.DerivedRows = st
	}
	links, err := s.LinkSources(norm)
	if err != nil {
		return res, err
	}
	res.BrokenLinks = links
	return res, nil
}

// Remove 删掉一篇文档：先清派生层，再删文件，最后留痕。
//
// 顺序是有意的：先清索引（万一清理失败，文件还在，重试即可）；文件删掉之后才留痕。
func (s *Service) Remove(rel string, actor vault.Actor) (RemoveResult, error) {
	norm, err := s.checkDoc(rel)
	if err != nil {
		return RemoveResult{}, err
	}
	if actor.Kind == "" {
		return RemoveResult{}, fmt.Errorf("删除必须给 -actor（human:名字 或 agent:名字）")
	}
	// 先记下「谁链到它」——文件删了之后反查就不能靠 loader 了。
	links, err := s.LinkSources(norm)
	if err != nil {
		return RemoveResult{}, err
	}
	res := RemoveResult{Path: norm, BrokenLinks: links}

	if err := s.ensureIndex(); err != nil {
		return res, err
	}
	n, err := s.index.PurgeDoc(norm)
	if err != nil {
		return res, err
	}
	res.DerivedRows = n

	full := filepath.Join(s.loader.Root, filepath.FromSlash(norm))
	if err := os.Remove(full); err != nil {
		return res, err
	}
	change := Change{Path: norm, Actor: actor, Action: "remove"}
	s.version(&change)
	res.Change = change
	return res, nil
}

// RemoveMany 按顺序删一批（一个失败就停，已删的保留——失败原因要能看见）。
func (s *Service) RemoveMany(paths []string, actor vault.Actor) ([]RemoveResult, error) {
	out := make([]RemoveResult, 0, len(paths))
	for _, p := range paths {
		r, err := s.Remove(p, actor)
		out = append(out, r)
		if err != nil {
			return out, fmt.Errorf("删 %s 失败：%w", p, err)
		}
	}
	return out, nil
}

// checkDoc 校验路径：必须在 vault 内、必须存在、必须是文档（.md）。
func (s *Service) checkDoc(rel string) (string, error) {
	norm := vault.NormalizeSlash(rel)
	if norm == "" {
		return "", fmt.Errorf("没给路径")
	}
	ok, err := s.loader.Exists(norm)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("文档不存在：%s", norm)
	}
	if ext := filepath.Ext(norm); ext != ".md" {
		return "", fmt.Errorf("只能删文档（.md），收到 %s；数据表请直接改文件", norm)
	}
	return norm, nil
}

// LinkSources 找出哪些文档链到它（断链预警用）。
func (s *Service) LinkSources(rel string) ([]string, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return nil, err
	}
	// 链接可能写成 `[[docs/甲]]`，也可能只写 `[[甲]]`，还可能带 `#锚点`——
	// 所以要按「规范化后相等」或「文件名相等」两种方式比（双链的解析规则在 vault.Resolve 里，
	// 这里只要不漏报：宁可多报一条，也不要静默留下断链）。
	wantFull := strings.TrimSuffix(vault.NormalizeSlash(rel), ".md")
	wantBase := path.Base(wantFull)
	var out []string
	for _, d := range docs {
		if d.Path == rel {
			continue
		}
		for _, l := range d.Links {
			got := strings.TrimSuffix(vault.NormalizeSlash(l.Target), ".md")
			if got == "" {
				continue
			}
			if got == wantFull || got == wantBase || path.Base(got) == wantBase {
				out = append(out, d.Path)
				break
			}
		}
	}
	return out, nil
}

// MatchDocs 按路径 glob 找出匹配的**文档**（批量删除用；`raw/x/**` 这种）。
//
// glob 的语义与收录范围共用一份实现（`vault.MatchPathGlob`），避免出现两种方言。
// 返回按路径排序，所以同样的模式、同样的 vault，结果顺序永远一样。
func (s *Service) MatchDocs(pattern string) ([]string, error) {
	pat := vault.NormalizeSlash(pattern)
	if pat == "" {
		return nil, nil
	}
	docs, err := s.loader.Load()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range docs {
		if vault.MatchPathGlob(pat, d.Path) {
			out = append(out, d.Path)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ExpandArgs 把命令行参数展开成文档路径列表：带 glob 元字符的按 glob 展开，
// 其余的按原样（留着让 Remove 去校验存在性）。
func (s *Service) ExpandArgs(args []string) (docs []string, patterns int, err error) {
	var out []string
	for _, a := range args {
		if strings.ContainsAny(a, "*?[") {
			patterns++
			matched, err := s.MatchDocs(a)
			if err != nil {
				return nil, patterns, err
			}
			if len(matched) == 0 {
				return nil, patterns, fmt.Errorf("没有文档匹配 %q", a)
			}
			out = append(out, matched...)
			continue
		}
		out = append(out, vault.NormalizeSlash(a))
	}
	return out, patterns, nil
}
