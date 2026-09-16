// Package vaultapp 是 vault 的用例编排：把领域规则（domain/vault）与文件系统
// （infrastructure/vaultfs）串成一条条用例。
//
// 它是对外能力的**唯一入口**——CLI、将来的 MCP 与界面都走这里，
// 所以权限规则只在这一层实现一次（见 docs/specs/agent.spec.md §4：
// 「MCP 与 CLI 走同一套权限规则，同一条规则在两个入口行为必须一致」）。
package vaultapp

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultfs"
)

// Service 是一个 vault 上的用例入口。
type Service struct {
	loader *vaultfs.Loader
}

// New 构造 Service。
func New(root string) *Service { return &Service{loader: vaultfs.New(root)} }

// Root 返回 vault 根目录。
func (s *Service) Root() string { return s.loader.Root }

// Item 是列表里的一项。
type Item struct {
	Path   string
	Layer  vault.Layer
	Title  string
	Status vault.Status
	Links  int
}

// List 列出全部文档。
func (s *Service) List() ([]Item, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(docs))
	for _, d := range docs {
		out = append(out, Item{Path: d.Path, Layer: d.Layer, Title: d.Title, Status: d.Status, Links: len(d.Links)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Tables 列出数据表。
func (s *Service) Tables() ([]string, error) { return s.loader.Tables() }

// Docs 读出全部文档（含正文与双链），供内部与需要全量的用例使用。
func (s *Service) Docs() ([]vault.Doc, error) { return s.loader.Load() }

// Read 读一篇文档。路径可以只写文件名（`[[茨木童子]]` 那种写法也认）。
func (s *Service) Read(rel string) (vault.Doc, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return vault.Doc{}, err
	}
	doc, err := pick(docs, rel)
	if err != nil {
		return vault.Doc{}, err
	}
	return doc, nil
}

// BacklinkResult 是一篇文档的反链情况。
type BacklinkResult struct {
	Target    string
	Backlinks []vault.Backlink
	// Issues 是它链出去的问题链接（断链 / 歧义分开），见 vault.LinkIssues。
	Issues []vault.LinkIssue
}

// Backlinks 算某篇文档的反链，以及它自己链出去的问题链接。
//
// 两件一起给：只报反链会让人以为文档很干净，而断链与歧义恰恰是质量问题
// （见 vault.spec.md「怎么验证」）。
func (s *Service) Backlinks(rel string) (BacklinkResult, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return BacklinkResult{}, err
	}
	doc, err := pick(docs, rel)
	if err != nil {
		return BacklinkResult{}, err
	}
	return BacklinkResult{
		Target:    doc.Path,
		Backlinks: vault.Backlinks(docs, doc.Path),
		Issues:    vault.LinkIssues(docs, doc.Path),
	}, nil
}

// Resolve 解析一条双链（可以带方括号，也可以只写目标）到具体文档与锚点。
func (s *Service) Resolve(ref string) (vault.Resolution, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return vault.Resolution{}, err
	}
	link, err := vault.ParseLink(ref)
	if err != nil {
		return vault.Resolution{}, err
	}
	return vault.Resolve(docs, link)
}

// Change 描述一次写入的结果。
type Change struct {
	Path   string
	From   vault.Status
	To     vault.Status
	Actor  vault.Actor
	Action string
}

// CommitMessage 是建议的提交信息，带 `Edited-By` trailer（见 agent.spec.md §3）。
//
// 我们**不代跑 git**：版本层是 git（vault.spec.md §5），但提交与否由人和
// 上层流程决定——工具悄悄提交会让人找不到「我什么时候改的」。
func (c Change) CommitMessage() string {
	return fmt.Sprintf("vault: %s %s\n\nEdited-By: %s", c.Action, c.Path, c.Actor.Trailer())
}

// SetStatus 改发布态。
//
// **门在这里**：只有人能发布或归档。这条故意落在能力层而不是 agent 后端里——
// 后端可替换，门放在后端，换一个后端就绕过去了（agent.spec.md §1）。
func (s *Service) SetStatus(rel string, st vault.Status, actor vault.Actor) (Change, error) {
	docs, err := s.loader.Load()
	if err != nil {
		return Change{}, err
	}
	doc, err := pick(docs, rel)
	if err != nil {
		return Change{}, err
	}
	if st != vault.StatusDraft && !actor.CanChangeStatus() {
		return Change{}, fmt.Errorf("只有人能发布或归档：%s 不能把 %s 改成 %s（agent 的改动只能停在 draft，等人复核）",
			actor.Trailer(), doc.Path, st)
	}
	if err := s.loader.SetStatus(doc.Path, st); err != nil {
		return Change{}, err
	}
	return Change{Path: doc.Path, From: doc.Status, To: st, Actor: actor, Action: "状态改为 " + string(st)}, nil
}

// Write 写入正文（front matter 原样保留）。
//
// agent 写入时**强制回落 draft**：哪怕原来是 published，也要变回 draft 等人复核，
// 否则 agent 能绕开人改掉已发布的内容（agent.spec.md §2）。
func (s *Service) Write(rel, body string, actor vault.Actor) (Change, error) {
	target, err := s.writeTarget(rel)
	if err != nil {
		return Change{}, err
	}
	from := vault.StatusDraft
	existed, err := s.loader.Exists(target)
	if err != nil {
		return Change{}, err
	}
	if existed {
		doc, err := s.Read(target)
		if err != nil {
			return Change{}, err
		}
		from = doc.Status
	}
	if err := s.loader.WriteBody(target, body); err != nil {
		return Change{}, err
	}
	to := vault.StatusDraft
	if existed {
		to = actor.StatusAfterEdit(from)
		if to != from {
			if err := s.loader.SetStatus(target, to); err != nil {
				return Change{}, err
			}
		}
	}
	return Change{Path: target, From: from, To: to, Actor: actor, Action: "写入"}, nil
}

// writeTarget 决定写入落到哪个路径。
//
// 规则（写下来免得靠猜）：带层前缀的按原样并补 `.md`；只写名字的落到 `docs/<名字>.md`。
// **同名文档已在别处时不猜**——报错让调用方给全路径，免得造出两篇同名的。
func (s *Service) writeTarget(rel string) (string, error) {
	t := vault.NormalizeTarget(rel)
	if t == "" {
		return "", fmt.Errorf("必须指明文档路径")
	}
	// 文档写入只处理 markdown。数据表走的是另一条工具（table.write），
	// 让 `write tables/x.csv` 悄悄落成 `docs/tables/x.csv.md` 只会让人困惑。
	if ext := path.Ext(t); ext != "" && !strings.EqualFold(ext, ".md") {
		return "", fmt.Errorf("文档写入只处理 .md，收到 %q——数据表请走专门的数据表工具", t)
	}
	if vault.LayerOf(t) != "" {
		if !strings.HasSuffix(t, ".md") {
			t += ".md"
		}
		return t, nil
	}
	if !strings.HasSuffix(t, ".md") {
		t += ".md"
	}
	candidate := "docs/" + path.Base(t)
	docs, err := s.loader.Load()
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		if d.Path == candidate {
			return candidate, nil
		}
	}
	base := strings.TrimSuffix(path.Base(t), ".md")
	var same []string
	for _, d := range docs {
		if strings.TrimSuffix(path.Base(d.Path), ".md") == base {
			same = append(same, d.Path)
		}
	}
	if len(same) > 0 {
		return "", fmt.Errorf("已存在同名文档 %s——写入请给完整路径，免得造出第二篇同名的", strings.Join(same, "、"))
	}
	return candidate, nil
}

// pick 按路径或文件名找一篇文档；同名多篇时报歧义，让调用方指明（系统不裁决）。
func pick(docs []vault.Doc, rel string) (vault.Doc, error) {
	target := vault.NormalizeTarget(rel)
	if target == "" {
		return vault.Doc{}, fmt.Errorf("必须指明文档路径")
	}
	for _, d := range docs {
		if d.Path == target {
			return d, nil
		}
	}
	res, err := vault.Resolve(docs, vault.Link{Target: target})
	if err != nil {
		return vault.Doc{}, err
	}
	for _, d := range docs {
		if d.Path == res.Path {
			return d, nil
		}
	}
	return vault.Doc{}, fmt.Errorf("找不到文档 %q", rel)
}
