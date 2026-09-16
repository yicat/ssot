// 文档库接口：让界面能看 vault（列文档、读文档、双链、反链、发布态）。
//
// ⚠️ 这一层只做参数转换与转发，规则全在 case 层（application/vaultapp）——
// 尤其是「谁能发布」那条门，界面上点按钮和命令行敲命令走的是同一份实现。
package api

import (
	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

// VaultItem 是列表里的一项（面向界面）。
type VaultItem struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Layer  string `json:"layer"`
	Status string `json:"status"`
	Links  int    `json:"links"`
}

// VaultLink 是正文里的一条双链（面向界面）。
type VaultLink struct {
	Raw     string `json:"raw"`
	Target  string `json:"target"`
	Heading string `json:"heading"`
	Block   string `json:"block"`
	Alias   string `json:"alias"`
	Embed   bool   `json:"embed"`
	Offset  int    `json:"offset"`
}

// VaultDoc 是一篇文档（面向界面）。
type VaultDoc struct {
	Path       string      `json:"path"`
	Title      string      `json:"title"`
	Layer      string      `json:"layer"`
	Status     string      `json:"status"`
	Tags       []string    `json:"tags"`
	Source     string      `json:"source"`
	Body       string      `json:"body"`
	BodyOffset int         `json:"bodyOffset"`
	Links      []VaultLink `json:"links"`
}

// VaultBacklink 是一条反链（面向界面）。
type VaultBacklink struct {
	From    string `json:"from"`
	Raw     string `json:"raw"`
	Block   string `json:"block"`
	Heading string `json:"heading"`
	Embed   bool   `json:"embed"`
	Target  string `json:"target"`
	Offset  int    `json:"offset"`
}

// VaultIssue 是一条问题链接（面向界面）。
type VaultIssue struct {
	Raw    string `json:"raw"`
	Target string `json:"target"`
	Kind   string `json:"kind"` // broken | ambiguous
	Reason string `json:"reason"`
}

// VaultBacklinkResult 是反链面板要的全部内容。
type VaultBacklinkResult struct {
	Target    string          `json:"target"`
	Backlinks []VaultBacklink `json:"backlinks"`
	Issues    []VaultIssue    `json:"issues"`
}

// VaultResolution 是一条双链被解析到的结果（面向界面）。
type VaultResolution struct {
	Path        string   `json:"path"`
	Heading     string   `json:"heading"`
	Block       string   `json:"block"`
	HeadingLine int      `json:"headingLine"`
	BlockLine   int      `json:"blockLine"`
	BlockText   string   `json:"blockText"`
	Candidates  []string `json:"candidates"`
}

// VaultChange 是一次写入的结果（面向界面）。
type VaultChange struct {
	Path          string `json:"path"`
	From          string `json:"from"`
	To            string `json:"to"`
	Actor         string `json:"actor"`
	CommitMessage string `json:"commitMessage"`
	// Committed / CommitSHA / VersionNote 说明在 git 里留痕的结果：
	// 成功给 SHA，没成功给原因（不是 git 仓库 / 没变化 / git 报错）。
	Committed   bool   `json:"committed"`
	CommitSHA   string `json:"commitSha"`
	VersionNote string `json:"versionNote"`
}

// VaultOverview 是一次拿齐的界面数据：列文档 + 数据表。
//
// 一次拿齐而不是两次调用：界面一进来就要它俩，分成两次会让左栏闪一下。
type VaultOverview struct {
	Root   string      `json:"root"`
	Items  []VaultItem `json:"items"`
	Tables []string    `json:"tables"`
}

// VaultHit 是一条检索命中（面向界面）。
type VaultHit struct {
	Path        string `json:"path"`
	Layer       string `json:"layer"`
	Status      string `json:"status"`
	Title       string `json:"title"`
	Snippet     string `json:"snippet"`
	TitleMatch  bool   `json:"titleMatch"`
	Occurrences int    `json:"occurrences"`
}

// VaultTableInfo 是索引里的一张数据表（面向界面）。
type VaultTableInfo struct {
	Name    string   `json:"name"`
	File    string   `json:"file"`
	Format  string   `json:"format"`
	Rows    int      `json:"rows"`
	Columns []string `json:"columns"`
}

// VaultResultSet 是一次只读查询的结果（面向界面）。
type VaultResultSet struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// VaultService 暴露文档库。
type VaultService struct {
	session *compose.Session
}

// NewVaultService 构造服务。
func NewVaultService(s *compose.Session) *VaultService { return &VaultService{session: s} }

// service 针对**当前项目**建用例层；一个项目都没打开时明确报错。
func (s *VaultService) service() (*vaultapp.Service, error) {
	p, err := s.session.Project()
	if err != nil {
		return nil, err
	}
	return vaultapp.New(p.Dir), nil
}

// Overview 列出当前项目的文档与数据表。
func (s *VaultService) Overview() (VaultOverview, error) {
	svc, err := s.service()
	if err != nil {
		return VaultOverview{}, err
	}
	items, err := svc.List()
	if err != nil {
		return VaultOverview{}, err
	}
	tables, err := svc.Tables()
	if err != nil {
		return VaultOverview{}, err
	}
	out := VaultOverview{Root: svc.Root(), Items: make([]VaultItem, 0, len(items)), Tables: tables}
	for _, it := range items {
		out.Items = append(out.Items, VaultItem{
			Path: it.Path, Title: it.Title, Layer: string(it.Layer), Status: string(it.Status), Links: it.Links,
		})
	}
	return out, nil
}

// Read 读一篇文档（含正文与双链）。
func (s *VaultService) Read(path string) (VaultDoc, error) {
	svc, err := s.service()
	if err != nil {
		return VaultDoc{}, err
	}
	doc, err := svc.Read(path)
	if err != nil {
		return VaultDoc{}, err
	}
	out := VaultDoc{
		Path: doc.Path, Title: doc.Title, Layer: string(doc.Layer), Status: string(doc.Status),
		Tags: doc.Tags, Source: doc.Source, Body: doc.Body, BodyOffset: doc.BodyOffset,
		Links: make([]VaultLink, 0, len(doc.Links)),
	}
	for _, l := range doc.Links {
		out.Links = append(out.Links, VaultLink{
			Raw: l.Raw, Target: l.Target, Heading: l.Heading, Block: l.Block, Alias: l.Alias, Embed: l.Embed, Offset: l.Offset,
		})
	}
	return out, nil
}

// Backlinks 算反链与问题链接。
func (s *VaultService) Backlinks(path string) (VaultBacklinkResult, error) {
	svc, err := s.service()
	if err != nil {
		return VaultBacklinkResult{}, err
	}
	res, err := svc.Backlinks(path)
	if err != nil {
		return VaultBacklinkResult{}, err
	}
	out := VaultBacklinkResult{
		Target:    res.Target,
		Backlinks: make([]VaultBacklink, 0, len(res.Backlinks)),
		Issues:    make([]VaultIssue, 0, len(res.Issues)),
	}
	for _, b := range res.Backlinks {
		out.Backlinks = append(out.Backlinks, VaultBacklink{
			From: b.From, Raw: b.Link.Raw, Block: b.Link.Block, Heading: b.Link.Heading,
			Embed: b.Link.Embed, Target: b.Link.Target, Offset: b.Link.Offset,
		})
	}
	for _, is := range res.Issues {
		out.Issues = append(out.Issues, VaultIssue{
			Raw: is.Link.Raw, Target: is.Link.Target, Kind: string(is.Kind), Reason: is.Reason,
		})
	}
	return out, nil
}

// Resolve 解析一条双链（界面点双链时用）。
func (s *VaultService) Resolve(ref string) (VaultResolution, error) {
	svc, err := s.service()
	if err != nil {
		return VaultResolution{}, err
	}
	res, err := svc.Resolve(ref)
	out := VaultResolution{
		Path: res.Path, Heading: res.Heading, Block: res.Block,
		HeadingLine: res.HeadingLine, BlockLine: res.BlockLine, BlockText: res.BlockText,
		Candidates: res.Candidates,
	}
	if err != nil {
		// 解析失败时把候选一并带回去（歧义要能让人做决定），错误照旧往上抛。
		return out, err
	}
	return out, nil
}

// SetStatus 改发布态。actor 形如 `human:名字` / `agent:名字`——
// **只有人能发布**，这条规则在 case 层，界面绕不过去。
func (s *VaultService) SetStatus(path, status, actor string) (VaultChange, error) {
	svc, err := s.service()
	if err != nil {
		return VaultChange{}, err
	}
	st, err := vault.ParseStatus(status)
	if err != nil {
		return VaultChange{}, err
	}
	a, err := vault.ParseActor(actor)
	if err != nil {
		return VaultChange{}, err
	}
	change, err := svc.SetStatus(path, st, a)
	if err != nil {
		return VaultChange{}, err
	}
	return toChange(change), nil
}

// Write 写正文（界面暂时用不到，但留着让界面与命令行走同一条路）。
func (s *VaultService) Write(path, body, actor string) (VaultChange, error) {
	svc, err := s.service()
	if err != nil {
		return VaultChange{}, err
	}
	a, err := vault.ParseActor(actor)
	if err != nil {
		return VaultChange{}, err
	}
	change, err := svc.Write(path, body, a)
	if err != nil {
		return VaultChange{}, err
	}
	return toChange(change), nil
}

func toChange(c vaultapp.Change) VaultChange {
	return VaultChange{
		Path: c.Path, From: string(c.From), To: string(c.To),
		Actor: c.Actor.Trailer(), CommitMessage: c.CommitMessage(),
		Committed: c.Committed, CommitSHA: c.CommitSHA, VersionNote: c.VersionNote,
	}
}

// Search 在标题与正文里检索（走派生索引；索引缺失时后端会先建）。
func (s *VaultService) Search(query string, limit int) ([]VaultHit, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	hits, err := svc.Search(query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]VaultHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, VaultHit{
			Path: h.Path, Layer: string(h.Layer), Status: string(h.Status), Title: h.Title,
			Snippet: h.Snippet, TitleMatch: h.TitleMatch, Occurrences: h.Occurrences,
		})
	}
	return out, nil
}

// TableInfos 列出数据表（含推断出来的列与行数）。
func (s *VaultService) TableInfos() ([]VaultTableInfo, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	tables, err := svc.TableInfos()
	if err != nil {
		return nil, err
	}
	out := make([]VaultTableInfo, 0, len(tables))
	for _, t := range tables {
		out = append(out, VaultTableInfo{
			Name: t.Name, File: t.File, Format: t.Format, Rows: t.Rows, Columns: t.Columns,
		})
	}
	return out, nil
}

// Query 对派生索引跑一条只读查询（数据表 + 文档 front matter）。
func (s *VaultService) Query(stmt string, limit int) (VaultResultSet, error) {
	svc, err := s.service()
	if err != nil {
		return VaultResultSet{}, err
	}
	rs, err := svc.QueryTables(stmt, limit)
	if err != nil {
		return VaultResultSet{}, err
	}
	return VaultResultSet{Columns: rs.Columns, Rows: rs.Rows}, nil
}

// Reindex 重建派生索引（界面上给一个「重建索引」的入口）。
func (s *VaultService) Reindex() error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.Reindex()
}
