// Package artifact 是原件的存档适配器（只读）。
//
// 见 docs/specs/ingestion.spec.md：**原件必须原样存档，与解析结果分开保存。**
// 由此得到一个关键性质：准入层可以**离线重放**——改进分级规则、修解析 bug，
// 都不必重新抓取（而重新抓取要再过一次 Cloudflare）。
package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store 是原件目录。
type Store struct {
	root string
}

// New 打开原件目录。
func New(root string) (*Store, error) {
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("原件目录不可用 %s：%w", root, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s 不是目录", root)
	}
	return &Store{root: root}, nil
}

// Root 返回目录路径。
func (s *Store) Root() string { return s.root }

// List 返回全部原件名（排序）。
func (s *Store) List() ([]string, error) {
	ents, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// Read 读取一个原件，返回内容与可读的标题。
//
// 存档时的文件名把 `:` `/` `\` 替换成了 `_`（例如
// `Data:Attribute.json` -> `Data_Attribute.json`），这里还原标题。
func (s *Store) Read(name string) (content []byte, title string, err error) {
	b, err := os.ReadFile(filepath.Join(s.root, name))
	if err != nil {
		return nil, "", err
	}
	return b, Title(name), nil
}

// Title 把存档文件名还原成原件标题。
func Title(name string) string {
	t := strings.TrimSuffix(name, ".json")
	t = strings.TrimSuffix(t, ".schema")
	// `Data_Character_262` -> `Data:Character/262`
	if strings.HasPrefix(name, "Data_") {
		rest := strings.TrimPrefix(name, "Data_")
		rest = strings.TrimSuffix(rest, ".json")
		rest = strings.TrimSuffix(rest, ".schema")
		rest = strings.TrimSuffix(rest, ".tabx")
		if i := strings.Index(rest, "_"); i >= 0 {
			return "Data:" + rest[:i] + "/" + rest[i+1:]
		}
		return "Data:" + rest
	}
	return t
}

// Find 按原标题查找原件文件名。
func (s *Store) Find(title string) (string, error) {
	candidates := []string{
		strings.ReplaceAll(strings.ReplaceAll(title, ":", "_"), "/", "_"),
		strings.ReplaceAll(strings.ReplaceAll(title, ":", "_"), "/", "_") + ".json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(s.root, c)); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("原件中找不到 %s", title)
}
