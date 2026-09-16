// Package vaultgit 是 git 的薄封装。
//
// vault 的版本层就是 git（docs/specs/vault.spec.md §5）：应用只做「提交这一个文件」
// 这类薄动作，不自建修订存储。
//
// ⚠️ 一条容易踩的边界：**vault 必须是它自己的仓库**。`git rev-parse` 在子目录里也会
// 成功——如果 vault 只是某个大仓库里的一个子目录（比如 `projects/demo` 躺在工具仓库里），
// 直接提交会把 vault 内容提进**工具仓库**，正是 spec 里要避免的「两边版本互相搅」。
// 所以 IsRepo 判定的是「root 就是工作区顶」，不是「root 在某个工作区里」。
package vaultgit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Repo 是一个 vault 根目录上的 git 封装。
type Repo struct{ Root string }

// New 构造 Repo。
func New(root string) *Repo { return &Repo{Root: root} }

// commitTimeout 是单次 git 命令的超时。
//
// 有超时是为了不让 git 卡住 MCP 调用：DSH 侧单次工具调用默认 60s 就放弃，
// 一个卡在 hooks 或凭据提示上的 git 会把这个调用一起拖死。
const commitTimeout = 10 * time.Second

// git 跑一条 git 命令，返回 stdout（失败时把 stderr 也带上）。
func (r *Repo) git(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commitTimeout)
	defer cancel()
	// 带上 -c commit.gpgsign=false：个人机器上开了签名而 agent 没有密钥时，
	// 提交会失败或挂住；vault 的留痕只需要 trailer，不需要签名。
	full := append([]string{"-C", r.Root, "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("git %s 超时（%s）", strings.Join(args, " "), commitTimeout)
		}
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s 失败：%s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// IsRepo 判断 Root 是否**本身就是**一个 git 工作区顶。
//
// 只看 `.git` 在不在不够（worktree / submodule 会写成文件），所以问 git 自己要
// `--show-toplevel`，并要求它等于 Root。
func (r *Repo) IsRepo() bool {
	top, err := r.git("rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	return samePath(top, r.Root)
}

// samePath 比较两个路径是否指同一处（Windows 上 git 给的是正斜杠，盘符大小写也不定）。
func samePath(a, b string) bool {
	norm := func(p string) string {
		p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
		p = strings.TrimSuffix(p, "/")
		return strings.ToLower(p)
	}
	return norm(a) == norm(b)
}

// HasChanges 报告 rel（相对 vault 根）相对 HEAD 有没有改动，含未跟踪的新文件。
func (r *Repo) HasChanges(rel string) (bool, error) {
	out, err := r.git("status", "--porcelain", "--", rel)
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// Commit 提交**单个文件**，返回 commit SHA。
//
// subject 是第一段，trailer 是最后一段——git 只认最后一段里的 trailer，
// 所以 `Edited-By` 必须独占一段，不能跟正文挤在一起（agent.spec.md §3）。
//
// 只提交 rel 这一个路径：不用 `add -A`，不然会把用户手边未完成的改动一起卷进来，
// 也会把 `.data/` 这类派生文件带进去。
//
// rel 没有改动时返回空 SHA 与 nil——「没变化」不是错误，调用方按「未产生新版本」处理。
func (r *Repo) Commit(rel, subject, trailer string) (string, error) {
	changed, err := r.HasChanges(rel)
	if err != nil {
		return "", err
	}
	if !changed {
		return "", nil
	}
	if _, err := r.git("add", "--", rel); err != nil {
		return "", err
	}
	args := []string{"commit", "-q", "-m", subject}
	if strings.TrimSpace(trailer) != "" {
		args = append(args, "-m", trailer)
	}
	args = append(args, "--", rel)
	if _, err := r.git(args...); err != nil {
		return "", err
	}
	sha, err := r.git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return sha, nil
}

// 让调用方能判断「不是 git 仓库」这类前置条件，而不用解析错误字符串。
var ErrNotRepo = errors.New("vault 不是独立的 git 仓库")
