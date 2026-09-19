package vaultapp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// 管理视图要能回答「哪一层占了什么」：分组、体量、被别的组引用的次数。
// 这是 agent 提范围建议时的**可统计事实**（不许凭目录名猜），所以口径要钉住。
func TestGroupStats(t *testing.T) {
	root := t.TempDir()
	// 三组：docs/式神（2 篇，被 raw 引到）、raw/剧情（2 篇）、raw/式神（1 篇引了 docs）。
	write(t, root, "docs/式神/茨木童子.md", "---\ntitle: 茨木童子\n---\n\n正文四。\n")
	write(t, root, "docs/式神/酒吞童子.md", "---\ntitle: 酒吞童子\n---\n\n正文五。\n")
	write(t, root, "raw/剧情/一章.md", "---\ntitle: 一章\n---\n\n见 [[茨木童子]]。\n")
	write(t, root, "raw/剧情/二章.md", "---\ntitle: 二章\n---\n\n不引任何东西。\n")
	write(t, root, "raw/式神/来源.md", "---\ntitle: 来源\n---\n\n也引 [[酒吞童子]]。\n")

	svc := New(root)
	stats, err := svc.GroupStats()
	if err != nil {
		t.Fatal(err)
	}
	byDir := map[string]GroupStat{}
	for _, s := range stats {
		byDir[s.Dir] = s
	}
	if len(stats) != 3 {
		t.Fatalf("该分成三组，拿到 %+v", stats)
	}
	if byDir["docs/式神"].Docs != 2 {
		t.Errorf("docs/式神 该有 2 篇：%+v", byDir["docs/式神"])
	}
	if byDir["raw/剧情"].Docs != 2 {
		t.Errorf("raw/剧情 该有 2 篇：%+v", byDir["raw/剧情"])
	}
	// 被**别的组**引用的次数：docs/式神 被 raw/剧情 与 raw/式神 各引一次。
	if byDir["docs/式神"].Inbound != 2 {
		t.Errorf("docs/式神 该被别的组引 2 次：%+v", byDir["docs/式神"])
	}
	// 自己组内互相引用不算（否则「能不能安全删」会被自己人撑起来）。
	if byDir["raw/剧情"].Inbound != 0 {
		t.Errorf("组内引用不该计入：%+v", byDir["raw/剧情"])
	}
	// 按体量降序：大的在前，一眼看出大头。
	for i := 1; i < len(stats); i++ {
		if stats[i-1].Chars < stats[i].Chars {
			t.Errorf("该按体量降序：%+v", stats)
		}
	}
	// 派生占用：`GroupStats` 会**顺手建索引**（`ensureIndex`），所以块数不是 0 ——
	// 2 篇短文档 = 2 个块、2 个抽取块；实体/关系要真抽取过才有。
	if byDir["docs/式神"].Chunks != 2 || byDir["docs/式神"].Extracts != 2 {
		t.Errorf("块数该按篇数算出来（每篇一个块）：%+v", byDir["docs/式神"])
	}
	if byDir["docs/式神"].Entities != 0 {
		t.Errorf("没抽取过就不该有实体：%+v", byDir["docs/式神"])
	}
}

// 分组规则：`a/b/c.md` → `a/b`；根下的文件归到「(根)」这个特别的组名。
func TestGroupDir(t *testing.T) {
	cases := map[string]string{
		"a/b/c.md":   "a/b",
		"a/x.md":     "a",
		"x.md":       "(根)",
		"a\\b\\c.md": "a/b", // Windows 反斜杠也要归一化
	}
	for in, want := range cases {
		if got := groupDir(in); got != want {
			t.Errorf("groupDir(%q) = %q，想要 %q", in, got, want)
		}
	}
}

// Restore：删错了要能顺手回滚（删除是普通操作，回滚也该一样）。
// 四个前置条件的报错也逐个钉住——它们都是「人话」的一部分。
func TestRestoreFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("没有 git，跳过")
	}
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME":     "测试",
		"GIT_AUTHOR_EMAIL":    "test@example.com",
		"GIT_COMMITTER_NAME":  "测试",
		"GIT_COMMITTER_EMAIL": "test@example.com",
	} {
		t.Setenv(k, v)
	}

	root := t.TempDir()
	gitRunIn(t, root, "init", "-q")

	svc := New(root)
	human := vault.Actor{Kind: "human", Name: "我"}
	// ⚠️ 必须**先写**（写的时候会进版本），再删。
	// 试过「先造文件、再直接删」：那个文件从未进过版本，git 里根本没有它的删除记录，
	// 恢复自然找不到历史——那不是代码的问题，是我的场景不真实（踩过）。
	if _, err := svc.Write("docs/甲.md", "原始正文。", human); err != nil {
		t.Fatalf("写文档失败：%v", err)
	}
	if _, err := svc.RemoveMany([]string{"docs/甲.md"}, human); err != nil {
		t.Fatalf("再删掉它（并留痕）：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "甲.md")); !os.IsNotExist(err) {
		t.Fatal("前置条件：文件该已被删除")
	}

	// 恢复：要有 actor（谁恢复的要留痕）、要有删除记录、vault 要独立成仓库。
	if _, err := svc.Restore("docs/甲.md", vault.Actor{}); err == nil || !strings.Contains(err.Error(), "actor") {
		t.Errorf("没给 actor 该报错：%v", err)
	}
	if _, err := svc.Restore("", human); err == nil {
		t.Error("没给路径该报错")
	}
	if _, err := svc.Restore("docs/从来没删过.md", human); err == nil {
		t.Error("历史里没删过它，该报错")
	}

	ch, err := svc.Restore("docs/甲.md", human)
	if err != nil {
		t.Fatalf("恢复失败：%v", err)
	}
	if ch.Action != "restore" || ch.Path != "docs/甲.md" {
		t.Errorf("改变记录不对：%+v", ch)
	}
	b, err := os.ReadFile(filepath.Join(root, "docs", "甲.md"))
	if err != nil {
		t.Fatalf("恢复后该能读到：%v", err)
	}
	if !strings.Contains(string(b), "原始正文") {
		t.Errorf("恢复的内容不对：%q", string(b))
	}
	// 恢复本身也要留痕（不然「谁恢复的」这个问题答不出来）。
	if ch.VersionNote != "" && !ch.Committed {
		t.Errorf("该给这次恢复留痕：%+v", ch)
	}

	// 文件还在时说「不需要恢复」，而不是再恢复一遍。
	if _, err := svc.Restore("docs/甲.md", human); err == nil || !strings.Contains(err.Error(), "还在") {
		t.Errorf("文件还在时该明确说清：%v", err)
	}
}

// 不是独立仓库时，恢复要给人话（这是 vault 的起步态，很常见）。
func TestRestoreWithoutGitRepo(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/甲.md", "---\ntitle: 甲\n---\n\n正文。\n")
	svc := New(root)
	_, err := svc.Restore("docs/被删的.md", vault.Actor{Kind: "human", Name: "我"})
	if err == nil {
		t.Fatal("vault 不是 git 仓库时该报错")
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("错误信息该说清「不是 git 仓库」：%v", err)
	}
}

// gitRunIn 在测试里跑 git（与 vaultgit 的测试同款：带身份、禁签名、禁交互）。
func gitRunIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s 失败：%v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
