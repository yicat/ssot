// Package scopefile 读写一个 vault 的**收录范围声明**：`<vault>/.ssot/derived-scope.yml`。
//
// 为什么在 infrastructure：domain 只认纯规则（`vault.DerivedScope` / `vault.ExtractConfig`），
// yaml 的字段名与文件位置是**存储细节**，换掉（比如改成 TOML 或挪目录）不该动 domain。
//
// 语义照 docs/specs/derived.spec.md「收录范围」一节：
//   - **文件不存在 = 全收**（默认不静默少收），返回 ok=false 而不是错误；
//   - **文件存在但坏了 = 报错**（不许静默用默认值——与 settings.json 同一条规矩）；
//   - 生效声明与 Agent 的建议分两个文件，建议**不能生效**。
package scopefile

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// RelDir / 文件名：放在 vault 里的 `.ssot/` 下（vault 根目录保持干净）。
const (
	RelDir       = ".ssot"
	FileName     = "derived-scope.yml"
	ProposalName = "derived-scope.proposed.yml"
)

// Path 是生效声明的路径。
func Path(vaultRoot string) string { return filepath.Join(vaultRoot, RelDir, FileName) }

// ProposalPath 是 Agent 建议的路径。
func ProposalPath(vaultRoot string) string { return filepath.Join(vaultRoot, RelDir, ProposalName) }

// file 是 yaml 的形状（只在这里出现字段名）。
type file struct {
	Version int `yaml:"version"`
	Scope   struct {
		Exclude rule `yaml:"exclude"`
		Include rule `yaml:"include"`
	} `yaml:"scope"`
	Extract struct {
		EntityTypes []string `yaml:"entity_types"`
		Ignore      struct {
			NamePatterns []string `yaml:"name_patterns"`
			EmptyWords   []string `yaml:"empty_words"`
		} `yaml:"ignore"`
		Examples []string `yaml:"examples"`
	} `yaml:"extract"`
}

type rule struct {
	Paths []string `yaml:"paths"`
	Tags  []string `yaml:"tags"`
}

// Decl 是一份声明：生效的范围与原样保留的 yaml（`show` 要能原样打印给人看）。
type Decl struct {
	Scope   vault.DerivedScope
	Extract vault.ExtractConfig
	// Raw 是文件原文（没读到时为空）。
	Raw []byte
	// Exists 表示文件在不在。
	Exists bool
	// Path 是实际读的文件。
	Path string
}

// Load 读生效声明。文件不存在时返回 (Decl{Exists:false}, nil)——按 spec，那是「全收」，不是错误。
func Load(vaultRoot string) (Decl, error) { return load(vaultRoot, Path(vaultRoot)) }

// LoadProposal 读 Agent 的建议（同上语义）。
func LoadProposal(vaultRoot string) (Decl, error) { return load(vaultRoot, ProposalPath(vaultRoot)) }

func load(vaultRoot, p string) (Decl, error) {
	d := Decl{Path: p}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return d, nil
		}
		return d, err
	}
	d.Exists = true
	d.Raw = b
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil {
		// 坏了就报错，**不退回默认值**：静默按「全收」跑会让人以为排除生效了。
		return d, fmt.Errorf("%s 解析失败（不静默用默认值）：%w", p, err)
	}
	d.Scope = vault.DerivedScope{
		Exclude: vault.ScopeRule{Paths: trimAll(f.Scope.Exclude.Paths), Tags: trimAll(f.Scope.Exclude.Tags)},
		Include: vault.ScopeRule{Paths: trimAll(f.Scope.Include.Paths), Tags: trimAll(f.Scope.Include.Tags)},
	}
	d.Extract = vault.ExtractConfig{
		EntityTypes:        trimAll(f.Extract.EntityTypes),
		IgnoreNamePatterns: trimAll(f.Extract.Ignore.NamePatterns),
		EmptyWords:         trimAll(f.Extract.Ignore.EmptyWords),
		Examples:           f.Extract.Examples,
	}
	return d, nil
}

// Save 写生效声明（`scope set` 用）。
func Save(vaultRoot string, sc vault.DerivedScope, ex vault.ExtractConfig) error {
	return write(Path(vaultRoot), render(sc, ex, "生效声明：由人确认后写入（见 docs/specs/derived.spec.md）"))
}

// SaveProposal 写 Agent 的建议（`scope propose` 用）。
func SaveProposal(vaultRoot string, sc vault.DerivedScope, ex vault.ExtractConfig, reason string) error {
	head := "Agent 建议（**不能生效**）：等人用 `ssot vault scope set` 确认"
	if reason != "" {
		head += "\n# 理由：" + oneLine(reason)
	}
	return write(ProposalPath(vaultRoot), render(sc, ex, head))
}

func write(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// render 把规则写成 yaml（带一句头注释，说明这份文件是什么、谁能改）。
func render(sc vault.DerivedScope, ex vault.ExtractConfig, head string) []byte {
	var sb []byte
	sb = append(sb, []byte("# "+head+"\n# 缺省语义：没写规则 = 全收；include 优先于 exclude。\n")...)
	var f file
	f.Version = 1
	f.Scope.Exclude = rule{Paths: sc.Exclude.Paths, Tags: sc.Exclude.Tags}
	f.Scope.Include = rule{Paths: sc.Include.Paths, Tags: sc.Include.Tags}
	f.Extract.EntityTypes = ex.EntityTypes
	f.Extract.Ignore.NamePatterns = ex.IgnoreNamePatterns
	f.Extract.Ignore.EmptyWords = ex.EmptyWords
	f.Extract.Examples = ex.Examples
	out, err := yaml.Marshal(&f)
	if err != nil {
		// 结构体是我们自己定的，marshal 失败基本不可能；真失败也不能静默写半份。
		return append(sb, []byte("# 渲染失败："+err.Error()+"\n")...)
	}
	return append(sb, out...)
}

func trimAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = trimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// oneLine 把理由压成一行（yaml 注释里不能有换行）。
func oneLine(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' {
			out = append(out, ' ')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
