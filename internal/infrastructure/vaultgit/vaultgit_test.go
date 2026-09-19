package vaultgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 这一层的规矩是「vault 必须是它自己的仓库」——判错了就会把 vault 的内容
// 提交进**工具仓库**（`docs/specs/vault.spec.md` §5 要避免的就是这个）。
// 所以三种情形都钉住：不是仓库、是仓库、只是大仓库里的一个子目录。
func TestIsRepoOnlyWhenRootIsToplevel(t *testing.T) {
	requireGit(t)

	t.Run("空目录不是仓库", func(t *testing.T) {
		if New(t.TempDir()).IsRepo() {
			t.Error("没有任何 .git 的目录不该被判成仓库")
		}
	})

	t.Run("init 之后是仓库", func(t *testing.T) {
		root := t.TempDir()
		gitRun(t, root, "init", "-q")
		if !New(root).IsRepo() {
			t.Error("git init 之后该判成仓库")
		}
	})

	t.Run("大仓库里的子目录不算（否则会提到工具仓库里去）", func(t *testing.T) {
		outer := t.TempDir()
		gitRun(t, outer, "init", "-q")
		inner := filepath.Join(outer, "projects", "demo")
		if err := os.MkdirAll(inner, 0o755); err != nil {
			t.Fatal(err)
		}
		if New(inner).IsRepo() {
			t.Error("子目录不该被判成「自己就是仓库」")
		}
	})

	t.Run("路径末尾多个分隔符也算同一个（Windows 上 git 给正斜杠）", func(t *testing.T) {
		root := t.TempDir()
		gitRun(t, root, "init", "-q")
		if !New(root + string(os.PathSeparator)).IsRepo() {
			t.Error("末尾多一个分隔符该仍判成同一个仓库")
		}
	})
}

// Commit 只提交**一个文件**：不用 add -A（否则会把用户手边未完成的改动一起卷进去，
// 也会把 .data/ 这类派生文件带进去）。这条规矩必须测——它保护的是用户的未完成工作。
func TestCommitTouchesOnlyThatFile(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	writeFile(t, root, "docs/a.md", "甲\n")
	writeFile(t, root, "docs/b.md", "乙（用户手边未完成的改动）\n")

	repo := New(root)
	sha, err := repo.Commit("docs/a.md", "写甲", "Edited-By: agent:整理")
	if err != nil {
		t.Fatalf("提交失败：%v", err)
	}
	if sha == "" {
		t.Fatal("该返回 commit SHA")
	}
	if len(sha) != 40 {
		t.Errorf("SHA 该是 40 位，拿到 %q", sha)
	}

	// a.md 进版本了；b.md 还是未跟踪。
	status := gitRun(t, root, "status", "--porcelain")
	if !strings.Contains(status, "?? docs/b.md") {
		t.Errorf("b.md 该仍是未跟踪状态（只提交指定文件）：%q", status)
	}
	files := gitRun(t, root, "show", "--name-only", "--format=", "HEAD")
	if strings.TrimSpace(files) != "docs/a.md" {
		t.Errorf("这次提交该只含 docs/a.md，实际：%q", files)
	}
}

// trailer（`Edited-By: …`）必须**独占一段**：git 只认最后一段里的 trailer，
// 挤在正文里就不算留痕了（`docs/specs/agent.spec.md` §3）。
func TestCommitPutsTrailerInOwnParagraph(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	writeFile(t, root, "docs/a.md", "甲\n")

	repo := New(root)
	if _, err := repo.Commit("docs/a.md", "写甲", "Edited-By: agent:整理"); err != nil {
		t.Fatal(err)
	}
	// %b 是正文（不含 subject），trailer 该原样在最后一段里。
	body := gitRun(t, root, "log", "-1", "--format=%b")
	if !strings.Contains(body, "Edited-By: agent:整理") {
		t.Errorf("正文里该有 trailer：%q", body)
	}
	full := gitRun(t, root, "log", "-1", "--format=%B")
	if !strings.Contains(strings.ReplaceAll(full, "\r\n", "\n"), "写甲\n\nEdited-By: agent:整理") {
		t.Errorf("trailer 该与 subject 之间空一行（独占一段）：%q", full)
	}

	// 不给 trailer 时不该留出空段。
	writeFile(t, root, "docs/a.md", "甲改\n")
	if _, err := repo.Commit("docs/a.md", "改甲", ""); err != nil {
		t.Fatal(err)
	}
	if full := gitRun(t, root, "log", "-1", "--format=%B"); strings.Contains(full, "\n\n\n") {
		t.Errorf("没有 trailer 时不该多出空段：%q", full)
	}
}

// 没改动不是错误：调用方按「没产生新版本」处理（返回空 SHA，不报错）。
func TestCommitWithoutChangesIsNotAnError(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	writeFile(t, root, "docs/a.md", "甲\n")

	repo := New(root)
	if _, err := repo.Commit("docs/a.md", "写甲", ""); err != nil {
		t.Fatal(err)
	}
	sha, err := repo.Commit("docs/a.md", "再写一次（其实没变）", "")
	if err != nil {
		t.Fatalf("没改动不该报错：%v", err)
	}
	if sha != "" {
		t.Errorf("没改动该返回空 SHA，拿到 %q", sha)
	}
}

// HasChanges 要能看见**未跟踪的新文件**（只看 diff 会漏掉「刚创建的文档」）。
func TestHasChangesSeesUntrackedFiles(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	repo := New(root)

	writeFile(t, root, "docs/新.md", "新\n")
	changed, err := repo.HasChanges("docs/新.md")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("未跟踪的新文件该算「有改动」")
	}

	if _, err := repo.Commit("docs/新.md", "加新", ""); err != nil {
		t.Fatal(err)
	}
	changed, err = repo.HasChanges("docs/新.md")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("提交之后该算「没改动」")
	}
}

// RestoreDeleted：从**删除它的那次提交**的父版本里恢复；没删过就明确报错，不装作成功。
func TestRestoreDeleted(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	writeFile(t, root, "docs/a.md", "原始内容\n")

	repo := New(root)
	if _, err := repo.Commit("docs/a.md", "写甲", ""); err != nil {
		t.Fatal(err)
	}
	// 删掉并提交这次删除。
	if err := os.Remove(filepath.Join(root, "docs", "a.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit("docs/a.md", "删甲", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "a.md")); !os.IsNotExist(err) {
		t.Fatal("前置条件：文件该是删除状态")
	}

	if err := repo.RestoreDeleted("docs/a.md"); err != nil {
		t.Fatalf("恢复失败：%v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, "docs", "a.md"))
	if err != nil {
		t.Fatalf("恢复后该能读到：%v", err)
	}
	// ⚠️ 本机 git 的 core.autocrlf 会把恢复出来的 LF 换成 CRLF——那是 git 的行为，
	// 不是恢复内容不对，所以比之前先归一化行尾。
	if got := strings.ReplaceAll(string(b), "\r\n", "\n"); got != "原始内容\n" {
		t.Errorf("恢复的内容不对：%q", got)
	}

	// 历史里没删过的文件：报错，而不是悄悄成功。
	if err := repo.RestoreDeleted("docs/不存在.md"); err == nil {
		t.Error("历史里没有删除记录时该报错")
	}
}

// 「路径不存在」时的真实行为：**静默当成「没变化」**（返回空 SHA、不报错）。
//
// 为什么把这条钉住而不是「修掉」：这一层分不清三种情况——
//   1. 路径写错了（该报错）；
//   2. 删掉了一个从未跟踪的文件（合法，没有版本可记）；
//   3. 文件被 .gitignore 忽略（合法，本来就不该进版本）。
// 三种在 git 眼里都是「没有改动」，要分开得让调用方（它知道是写还是删）参与判断。
// 所以**不擅自改契约**，先把现状写进测试，措辞问题记在 `OPEN.md` #34。
func TestCommitOfMissingUntrackedPathIsSilentNoOp(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRun(t, root, "init", "-q")

	sha, err := New(root).Commit("docs/不存在.md", "写个不存在的", "")
	if err != nil {
		t.Fatalf("现状是「不报错」（见本测试的说明）：%v", err)
	}
	if sha != "" {
		t.Errorf("该返回空 SHA（没有产生提交），拿到 %q", sha)
	}
}

// 失败时要能看出**是哪条 git 命令失败、git 说了什么**（不然排查只剩一句「失败」）。
func TestGitErrorCarriesStderr(t *testing.T) {
	requireGit(t)

	t.Run("空仓库（还没有任何提交）", func(t *testing.T) {
		root := t.TempDir()
		gitRun(t, root, "init", "-q")
		err := New(root).RestoreDeleted("docs/不存在.md")
		if err == nil {
			t.Fatal("空仓库里恢复该失败")
		}
		// 这时 git log 自己就报错（does not have any commits yet），错误里该带上命令与原因。
		if !strings.Contains(err.Error(), "git log") {
			t.Errorf("错误信息该说清是哪条命令：%v", err)
		}
	})

	t.Run("有提交、但历史里没删过它", func(t *testing.T) {
		root := t.TempDir()
		gitRun(t, root, "init", "-q")
		writeFile(t, root, "docs/a.md", "甲\n")
		if _, err := New(root).Commit("docs/a.md", "写甲", ""); err != nil {
			t.Fatal(err)
		}
		err := New(root).RestoreDeleted("docs/不存在.md")
		if err == nil {
			t.Fatal("历史里没有删除记录时该报错，而不是装作成功")
		}
		if !strings.Contains(err.Error(), "找不到") {
			t.Errorf("错误信息该说清「历史里找不到」：%v", err)
		}
	})
}

// —— 测试脚手架

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("没有 git，跳过")
	}
	// 提交需要身份：本机可能没配全局 user.name/email（CI 上也一样），
	// 用环境变量给上——`git()` 会继承测试进程的环境。
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME":     "测试",
		"GIT_AUTHOR_EMAIL":    "test@example.com",
		"GIT_COMMITTER_NAME":  "测试",
		"GIT_COMMITTER_EMAIL": "test@example.com",
	} {
		t.Setenv(k, v)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s 失败：%v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
