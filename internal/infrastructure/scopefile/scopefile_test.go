package scopefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

func vaultScope(path, tag string) vault.DerivedScope {
	s := vault.DerivedScope{Exclude: vault.ScopeRule{}}
	if path != "" {
		s.Exclude.Paths = []string{path}
	}
	if tag != "" {
		s.Exclude.Tags = []string{tag}
	}
	return s
}

func vaultExtract() vault.ExtractConfig {
	return vault.ExtractConfig{
		EntityTypes:        []string{"TYPE_A", "TYPE_B"},
		IgnoreNamePatterns: []string{"*.*", "*/*"},
		EmptyWords:         []string{"EMPTY_X"},
	}
}

// 测试里用中性占位符（DIR_X / TAG_X），不要用 `<...>` 那种形状。
func TestLoadMissingMeansEverything(t *testing.T) {
	root := t.TempDir()
	d, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if d.Exists {
		t.Error("没文件时 Exists 该是假")
	}
	// 缺省语义要**由规则本身**给出：全收。
	if ok, why := d.Scope.Covers("raw/任意/x.md", nil); !ok {
		t.Errorf("没声明时该全收，实际：%s", why)
	}
	if got := d.Extract.NormalizeType("TYPE_A"); got != "Other" {
		t.Errorf("没声明时只该有兜底 Other，实际 %q", got)
	}
}

func TestBrokenFileIsAnErrorNotEmptyDefaults(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, RelDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(root), []byte("scope: [这不是 mapping"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("坏文件该报错——静默按全收跑会让人以为排除生效了")
	} else if !strings.Contains(err.Error(), "不静默") {
		t.Errorf("报错要说清为什么不退回默认值：%v", err)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	scope := vaultScope("raw/DIR_X/**", "TAG_X")
	ex := vaultExtract()
	if err := Save(root, scope, ex); err != nil {
		t.Fatal(err)
	}
	d, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Exists || len(d.Raw) == 0 {
		t.Fatal("存了之后该读得回来")
	}
	// 生效的规则真的在起作用（这才是这份文件的用处）。
	if ok, _ := d.Scope.Covers("raw/DIR_X/x.md", nil); ok {
		t.Error("排除该生效")
	}
	if ok, why := d.Scope.Covers("docs/别处/x.md", nil); !ok {
		t.Errorf("不该被误伤：%s", why)
	}
	if got := d.Extract.NormalizeType("TYPE_A"); got != "TYPE_A" {
		t.Errorf("词表该读回来，实际 %q", got)
	}
	if ok, why := d.Extract.ShouldIgnoreName("a.md"); !ok {
		t.Errorf("名字形状规则该读回来：%s", why)
	}
}

func TestProposalIsSeparateAndCarriesReason(t *testing.T) {
	root := t.TempDir()
	if err := SaveProposal(root, vaultScope("raw/DIR_X/**", ""), vaultExtract(), "这批是逐条转写，占 70% 字符量"); err != nil {
		t.Fatal(err)
	}
	// 建议不生效：生效声明仍然「不存在 = 全收」。
	live, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if live.Exists {
		t.Error("写了建议不该让生效声明出现")
	}
	prop, err := LoadProposal(root)
	if err != nil {
		t.Fatal(err)
	}
	if !prop.Exists {
		t.Fatal("建议该读得到")
	}
	if !strings.Contains(string(prop.Raw), "不能生效") || !strings.Contains(string(prop.Raw), "占 70% 字符量") {
		t.Errorf("建议文件该写明「不能生效」与理由：\n%s", prop.Raw)
	}
}
