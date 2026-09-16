package vaultapp

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// gitRun 在 root 跑一条 git 命令，返回 stdout；失败直接让测试挂。
//
// 测试里**不依赖机器上的 git 身份配置**：身份在 init 之后写进临时仓库的本地配置，
// 否则换一台机器（或 CI）就会因为「没配 user.name」而红——那种红跟被测逻辑无关。
func gitRun(t *testing.T, root string, args ...string) string {
	t.Helper()
	// -c core.quotepath=false：路径里有中文时 git 默认输出八进制转义（"\346\234\272…"），
	// 断言里比对路径会莫名其妙失败——那是显示格式，不是内容不对。
	full := append([]string{"-C", root, "-c", "core.quotepath=false"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s 失败：%v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitInit 把 root 变成它自己的 git 仓库，并做一次基线提交。
func gitInit(t *testing.T, root string) {
	t.Helper()
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "config", "user.name", "测试")
	gitRun(t, root, "config", "user.email", "test@example.com")
	gitRun(t, root, "add", "-A")
	// --allow-empty：外层目录可能是空的（只用来当容器），空提交让它也有 HEAD。
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "基线")
}

func agent(t *testing.T, name string) vault.Actor {
	t.Helper()
	a, err := vault.ParseActor("agent:" + name)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func human(t *testing.T, name string) vault.Actor {
	t.Helper()
	a, err := vault.ParseActor("human:" + name)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestWriteCommitsWithTrailer 是 agent.spec.md §3 的落地断言：
// 写入 → git 里留痕，trailer 记 Edited-By，而且**只提交改动的那一个文件**。
func TestWriteCommitsWithTrailer(t *testing.T) {
	root := newVault(t)
	gitInit(t, root)

	// 另一篇文档先弄脏：它不该被卷进这次提交（这是「只 add 那一个文件」的证明）。
	write(t, root, "docs/机制/伤害计算.md", "---\ntitle: 伤害计算\nstatus: draft\n---\n\n我手边改了一半。\n")

	ch, err := New(root).Write("docs/式神/茨木童子.md", "整理后的正文。\n", agent(t, "整理"))
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Committed || len(ch.CommitSHA) != 40 {
		t.Fatalf("写入该在 git 里留痕：committed=%v sha=%q note=%q", ch.Committed, ch.CommitSHA, ch.VersionNote)
	}
	// agent 改过的已发布文档必须回落 draft（agent.spec.md §2）。
	if ch.From != vault.StatusPublished || ch.To != vault.StatusDraft {
		t.Errorf("published 被 agent 改过之后该回落 draft：%s → %s", ch.From, ch.To)
	}

	msg := gitRun(t, root, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "Edited-By: agent:整理") {
		t.Errorf("提交信息里该有 trailer：%q", msg)
	}
	if !strings.Contains(msg, "docs/式神/茨木童子.md") {
		t.Errorf("提交信息该说清改了哪个文件：%q", msg)
	}
	files := gitRun(t, root, "show", "--name-only", "--format=")
	if strings.TrimSpace(files) != "docs/式神/茨木童子.md" {
		t.Errorf("一次提交只该带改动的那一个文件，实际：%q", files)
	}
	// 手边那份改动还在（没被卷走，也没被还原）。
	dirty := gitRun(t, root, "status", "--porcelain")
	if !strings.Contains(dirty, "docs/机制/伤害计算.md") {
		t.Errorf("别的文件的改动不该被提交，也不该消失：%q", dirty)
	}
}

// TestWriteWithoutRepoReportsNoTrace：不是 git 仓库时写入照样成功，
// 但必须**明说没留痕**——静默最坏：人以为有历史，其实没有。
func TestWriteWithoutRepoReportsNoTrace(t *testing.T) {
	root := newVault(t)
	ch, err := New(root).Write("docs/机制/伤害计算.md", "新正文。\n", agent(t, "整理"))
	if err != nil {
		t.Fatalf("没有 git 仓库不该让写入失败：%v", err)
	}
	if ch.Committed {
		t.Error("不是 git 仓库却说提交了")
	}
	if !strings.Contains(ch.VersionNote, "git init") {
		t.Errorf("该告诉人怎么补上：%q", ch.VersionNote)
	}
}

// TestNestedUnderAnotherRepoIsNotOwnRepo 是最容易踩的那个边界：
// vault 躺在别的大仓库里（`projects/demo` 就是这种），`git rev-parse` 也会成功，
// 这时候提交会把 vault 内容提进**那个仓库**——正是 vault.spec.md §5 要避免的。
func TestNestedUnderAnotherRepoIsNotOwnRepo(t *testing.T) {
	outer := t.TempDir()
	gitInit(t, outer)
	root := filepath.Join(outer, "vault")
	write(t, root, "docs/a.md", "---\ntitle: A\nstatus: draft\n---\n\n正文。\n")
	before := gitRun(t, outer, "rev-parse", "HEAD")

	ch, err := New(root).Write("docs/a.md", "改过。\n", agent(t, "整理"))
	if err != nil {
		t.Fatal(err)
	}
	if ch.Committed {
		t.Error("vault 不是自己的仓库时**不该**提交（会提到外层仓库里去）")
	}
	if !strings.Contains(ch.VersionNote, "独立的 git 仓库") {
		t.Errorf("该说清是「不是独立仓库」：%q", ch.VersionNote)
	}
	if after := gitRun(t, outer, "rev-parse", "HEAD"); after != before {
		t.Errorf("外层仓库不该多出提交：%s → %s", before, after)
	}
	if files := gitRun(t, outer, "show", "--name-only", "--format=", "HEAD"); strings.Contains(files, "vault/") {
		t.Errorf("vault 内容不该出现在外层仓库的提交里：%q", files)
	}
}

// TestHumanPublishCommits：人发布也要留痕（trailer 记 human）。
func TestHumanPublishCommits(t *testing.T) {
	root := newVault(t)
	gitInit(t, root)
	ch, err := New(root).SetStatus("docs/机制/伤害计算.md", vault.StatusPublished, human(t, "我"))
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Committed {
		t.Fatalf("人的发布也该留痕：%q", ch.VersionNote)
	}
	if msg := gitRun(t, root, "log", "-1", "--format=%B"); !strings.Contains(msg, "Edited-By: human:我") {
		t.Errorf("trailer 该记 human：%q", msg)
	}
}

// TestNoChangeDoesNotCommit：内容没变化时不产生空提交。
func TestNoChangeDoesNotCommit(t *testing.T) {
	root := newVault(t)
	gitInit(t, root)
	before := gitRun(t, root, "rev-list", "--count", "HEAD")

	// 抄一遍现有正文 = 内容不变。
	doc, err := New(root).Read("docs/机制/伤害计算.md")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := New(root).Write("docs/机制/伤害计算.md", doc.Body, agent(t, "整理"))
	if err != nil {
		t.Fatal(err)
	}
	if ch.Committed {
		t.Error("内容没变却产生了提交")
	}
	if !strings.Contains(ch.VersionNote, "没有变化") {
		t.Errorf("该说清是「没变化」而不是失败：%q", ch.VersionNote)
	}
	if after := gitRun(t, root, "rev-list", "--count", "HEAD"); after != before {
		t.Errorf("提交数该不变：%s → %s", before, after)
	}
}

// TestAgentCannotPublishUnchanged 是 agent.spec.md §1 的门（回归）：
// 门在能力层，任何入口都绕不过去。
func TestAgentCannotPublishUnchanged(t *testing.T) {
	root := newVault(t)
	gitInit(t, root)
	before := gitRun(t, root, "rev-list", "--count", "HEAD")
	_, err := New(root).SetStatus("docs/机制/伤害计算.md", vault.StatusPublished, agent(t, "核验"))
	if err == nil {
		t.Fatal("agent 发布必须失败")
	}
	if !strings.Contains(err.Error(), "只有人") {
		t.Errorf("失败信息要说清原因：%v", err)
	}
	if after := gitRun(t, root, "rev-list", "--count", "HEAD"); after != before {
		t.Errorf("被拒绝的写入不该留下提交：%s → %s", before, after)
	}
}
