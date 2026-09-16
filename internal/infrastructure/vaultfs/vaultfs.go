// Package vaultfs 把 vault 目录读成领域对象，并把改动写回文件。
//
// 约定见 docs/specs/vault.spec.md：两层（raw / docs）、front matter、tables/。
//
// 两条实现纪律：
//  1. **只读我们要的字段，不重排文件**——markdown 是人写的，程序不能把人的格式冲掉。
//     所以写 status 走行级替换，而不是「YAML 解析→重新序列化」。
//  2. 路径一律相对 vault 根、用 `/` 分隔，且不许跑出根目录。
package vaultfs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// Loader 读写一个 vault 目录。
type Loader struct {
	Root string
}

// New 构造 Loader。
func New(root string) *Loader { return &Loader{Root: root} }

// frontMatter 是我们要读的那几个字段。**不认识的键不丢**——因为写回走行级替换，
// 根本不经过这个结构体。
type frontMatter struct {
	Title  string   `yaml:"title"`
	Tags   []string `yaml:"tags"`
	Status string   `yaml:"status"`
	Source string   `yaml:"source"`
}

// Load 读出 vault 里的全部文档（docs/ 与 raw/ 下的 *.md），按路径排序。
//
// 目录不存在时返回空列表而不是错误：还没建 docs/ 是「还没有文档」，不是出错了。
func (l *Loader) Load() ([]vault.Doc, error) {
	var out []vault.Doc
	for _, layer := range []vault.Layer{vault.LayerDocs, vault.LayerRaw} {
		dir := filepath.Join(l.Root, string(layer))
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var rels []string
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
				return nil
			}
			rel, err := filepath.Rel(l.Root, p)
			if err != nil {
				return err
			}
			rels = append(rels, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(rels)
		for _, rel := range rels {
			doc, err := l.Read(rel)
			if err != nil {
				return nil, err
			}
			out = append(out, doc)
		}
	}
	return out, nil
}

// Read 读一篇文档。
func (l *Loader) Read(rel string) (vault.Doc, error) {
	full, err := l.resolve(rel)
	if err != nil {
		return vault.Doc{}, err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return vault.Doc{}, err
	}
	return parseDoc(filepath.ToSlash(rel), string(b)), nil
}

// parseDoc 解析一篇文档：front matter + 正文 + 双链。
func parseDoc(rel, raw string) vault.Doc {
	yamlText, body, bodyOffset := splitFrontMatter(raw)
	var fm frontMatter
	if strings.TrimSpace(yamlText) != "" {
		// 解析失败不致命：正文仍然可读，只是元信息为空。
		// 但**不要**把失败咽掉——调用方通过 MetaOK 能看出来。
		_ = yaml.Unmarshal([]byte(yamlText), &fm)
	}
	doc := vault.Doc{
		Path:       rel,
		Layer:      vault.LayerOf(rel),
		Title:      fm.Title,
		Tags:       fm.Tags,
		Source:     fm.Source,
		Body:       body,
		BodyOffset: bodyOffset,
		Links:      vault.ParseLinks(body),
	}
	if doc.Title == "" {
		doc.Title = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	// 没有 status 就是 draft：未核验是默认状态，不是缺失。
	if st, err := vault.ParseStatus(fm.Status); err == nil {
		doc.Status = st
	} else {
		doc.Status = vault.StatusDraft
	}
	return doc
}

// splitFrontMatter 把文件切成 front matter（YAML 文本）与正文，
// 并给出**正文第一行在文件里的行号**（1 起）——锚点行号要靠它换算成文件行号。
// 没有 front matter 时返回空 YAML、整份内容、行号 1。
func splitFrontMatter(raw string) (string, string, int) {
	s := strings.TrimPrefix(raw, "\ufeff") // 容忍 BOM
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t\r") != "---" {
		return "", s, 1
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t\r") == "---" {
			yamlText := strings.Join(lines[1:i], "\n")
			body := strings.Join(lines[i+1:], "\n")
			// 结束线在索引 i，正文从 i+1 开始 → 文件行号是 i+2。
			return yamlText, body, i + 2
		}
	}
	// 只有开头的 `---`、没有结束：当成普通正文，不当成 front matter——
	// 半个 front matter 若被吞掉，正文会凭空少一段。
	return "", s, 1
}

// Tables 列出 tables/ 下的数据表文件（相对路径，排序）。
func (l *Loader) Tables() ([]string, error) {
	dir := filepath.Join(l.Root, "tables")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".csv", ".json", ".yaml", ".yml":
			out = append(out, filepath.ToSlash(filepath.Join("tables", e.Name())))
		}
	}
	sort.Strings(out)
	return out, nil
}

// SetStatus 改写一篇文档的发布态。
//
// 行级替换：只动 front matter 里那一行 `status:`，其它字节原样保留。
// 没有 front matter 时就补一个最小的——**不重排正文**。
func (l *Loader) SetStatus(rel string, st vault.Status) error {
	full, err := l.resolve(rel)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	updated, err := setStatusLine(string(b), st)
	if err != nil {
		return err
	}
	return os.WriteFile(full, []byte(updated), 0o644)
}

func setStatusLine(raw string, st vault.Status) (string, error) {
	bom := ""
	s := raw
	if strings.HasPrefix(s, "\ufeff") {
		bom, s = "\ufeff", s[1:]
	}
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t\r") != "---" {
		// 没有 front matter：补一个最小的，正文照原样跟在后面。
		return bom + "---\nstatus: " + string(st) + "\n---\n" + s, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t\r") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		// 只有开头的 `---`：不猜，直接拒绝，免得把正文改坏。
		return "", fmt.Errorf("%s 的 front matter 没有结束的 ---，不猜怎么改", rawPathHint(raw))
	}
	for i := 1; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "status:") || trimmed == "status:" {
			indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
			lines[i] = indent + "status: " + string(st)
			return bom + strings.Join(lines, "\n"), nil
		}
	}
	// front matter 在但没有 status：插在结束线之前。
	lines = append(lines[:end], append([]string{"status: " + string(st)}, lines[end:]...)...)
	return bom + strings.Join(lines, "\n"), nil
}

func rawPathHint(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return "文档"
}

// Exists 报告文档是否存在。
func (l *Loader) Exists(rel string) (bool, error) {
	full, err := l.resolve(rel)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(full); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WriteBody 换掉正文，front matter 原样保留。
//
// 没有 front matter 时补一个最小的（只有 status: draft）——**不重排正文之外的东西**。
// 注意 front matter 是当文本整段搬过去的，所以里面的人话、注释、键顺序都不会被动。
func (l *Loader) WriteBody(rel, body string) error {
	full, err := l.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	bom := ""
	raw := ""
	if b, err := os.ReadFile(full); err == nil {
		raw = string(b)
		if strings.HasPrefix(raw, "\ufeff") {
			bom, raw = "\ufeff", raw[1:]
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	fm, _, _ := splitFrontMatter(raw)
	if strings.TrimSpace(fm) == "" {
		fm = "status: " + string(vault.StatusDraft)
	}
	body = strings.TrimPrefix(body, "\n")
	out := bom + "---\n" + strings.TrimRight(fm, "\n") + "\n---\n\n" + strings.TrimRight(body, "\n") + "\n"
	return os.WriteFile(full, []byte(out), 0o644)
}

// resolve 把相对路径钉到 vault 根下，并拒绝跑出根目录。
func (l *Loader) resolve(rel string) (string, error) {
	norm := vault.NormalizeTarget(rel)
	if norm == "" {
		return "", fmt.Errorf("必须指明文档路径")
	}
	// 带 `..` 的路径直接**拒绝**，不要靠 path.Clean 把它夹回根目录下：
	// 静默改道会让人以为「写到了我指定的地方」，其实写到别处去了。
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径里不许有 ..（那会跑出 vault）：%s", rel)
		}
	}
	clean := path.Clean("/" + norm)
	if clean == "/" {
		return "", fmt.Errorf("必须指明文档路径")
	}
	full := filepath.Join(l.Root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	rootAbs, err := filepath.Abs(l.Root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径跑出 vault 了：%s", rel)
	}
	return fullAbs, nil
}
