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
	"github.com/ngnl5/ssot/internal/infrastructure/vaultgit"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultindex"
)

// Service 是一个 vault 上的用例入口。
type Service struct {
	loader *vaultfs.Loader
	index  *vaultindex.Index
	git    *vaultgit.Repo
	// embedDir 是嵌入模型目录。留空表示「这台机器上还没配模型」——
	// 那时向量相关的用例会给出人话错误，而不是静默退化（见 docs/OPEN.md #15）。
	embedDir string
}

// New 构造 Service。
func New(root string) *Service {
	return &Service{loader: vaultfs.New(root), index: vaultindex.New(root), git: vaultgit.New(root)}
}

// SetEmbedModelDir 指定嵌入模型目录（组合根从配置/环境变量读出来注入）。
func (s *Service) SetEmbedModelDir(dir string) { s.embedDir = dir }

// EmbedModelDir 返回当前的嵌入模型目录（可能为空）。
func (s *Service) EmbedModelDir() string { return s.embedDir }

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

// IndexPath 是派生索引的位置（`<vault>/.data/index.db`）。
func (s *Service) IndexPath() string { return s.index.Path() }

// Reindex 重建派生索引。
//
// 索引是**全派生**的：删了能重建，也可以随时重建——所以不担心它过期。
func (s *Service) Reindex() error { return s.index.Rebuild() }

// ChunkStat 是派生层块的统计（文档、嵌入块、抽取块、stale 数）。
func (s *Service) ChunkStat() (vault.ChunkStat, error) {
	if err := s.ensureIndex(); err != nil {
		return vault.ChunkStat{}, err
	}
	return s.index.ChunkStat()
}

// ensureIndex 索引缺失、或**结构版本对不上**时先建起来。
//
// 「搜不到东西」不该是因为忘了建索引——那种误导比慢几十毫秒严重得多。
// 结构变过（加了表/列）也一样：索引是派生的，重建就好，不该让人撞上 no such table。
func (s *Service) ensureIndex() error {
	if s.index.Ready() {
		return nil
	}
	return s.index.Rebuild()
}

// Search 在文档的标题与正文里检索。
func (s *Service) Search(q string, limit int) ([]vault.Hit, error) {
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	return s.index.Search(q, limit)
}

// CountMatches 数命中多少篇（不受 limit 影响）——给调用方报「一共几条、返回了几条」。
func (s *Service) CountMatches(q string) (int, error) {
	if err := s.ensureIndex(); err != nil {
		return 0, err
	}
	return s.index.CountMatches(q)
}

// TableInfos 列出数据表（含推断出来的列与行数）。
func (s *Service) TableInfos() ([]vault.TableInfo, error) {
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	return s.index.Tables()
}

// QueryTables 对索引跑一条只读查询：能查数据表，也能查文档的 front matter
// （docs 表里有 path/title/status/tags/source）。
func (s *Service) QueryTables(stmt string, limit int) (vault.ResultSet, error) {
	if err := s.ensureIndex(); err != nil {
		return vault.ResultSet{}, err
	}
	return s.index.Query(stmt, limit)
}

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
	// Committed / CommitSHA 报告这次改动有没有在 git 里留痕（agent.spec.md §3）。
	Committed bool
	CommitSHA string
	// VersionNote 说明**为什么没留痕**（不是 git 仓库、没变化、git 报错）。
	// 留痕失败不影响写入成功，但必须说出来——静默最坏：人以为有历史，其实没有。
	VersionNote string
}

// CommitMessage 是这次改动的提交信息，带 `Edited-By` trailer（见 agent.spec.md §3）。
//
// 第一段说做了什么，最后一段是 trailer——git 只认最后一段里的 trailer。
func (c Change) CommitMessage() string {
	return c.commitSubject() + "\n\n" + c.trailer()
}

func (c Change) commitSubject() string { return fmt.Sprintf("vault: %s %s", c.Action, c.Path) }

func (c Change) trailer() string { return "Edited-By: " + c.Actor.Trailer() }

// version 给这次改动留痕：提交**改动的那个文件**。
//
// 谁提交已定成能力层（agent.spec.md §3）：留痕不能靠 agent 记得做，
// 也不能靠人记得做——否则 spec 里「每次写入的 commit trailer 里有 Edited-By」永远验不了。
//
// 三种「没提交成」都要如实写出原因，但都不算写入失败：
// vault 不是独立仓库（最常见，起步态）、内容没变化、git 自己报错（如没配 user.name）。
func (s *Service) version(c *Change) {
	if !s.git.IsRepo() {
		c.VersionNote = "本次改动未留痕：vault 不是独立的 git 仓库（在 vault 根目录跑 git init 即可）"
		return
	}
	sha, err := s.git.Commit(c.Path, c.commitSubject(), c.trailer())
	if err != nil {
		c.VersionNote = "本次改动未留痕：" + err.Error()
		return
	}
	if sha == "" {
		c.VersionNote = "本次改动未留痕：文件内容没有变化，没有产生新提交"
		return
	}
	c.Committed = true
	c.CommitSHA = sha
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
	c := Change{Path: doc.Path, From: doc.Status, To: st, Actor: actor, Action: "状态改为 " + string(st)}
	s.version(&c)
	return c, nil
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
	c := Change{Path: target, From: from, To: to, Actor: actor, Action: "写入"}
	s.version(&c)
	return c, nil
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
