package vaultapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFilePagingAndLimits(t *testing.T) {
	root := newVault(t)
	write(t, root, "raw/大文件.txt", strings.Join([]string{"一", "二", "三", "四", "五"}, "\n")+"\n")
	svc := New(root)

	all, err := svc.ReadFile("raw/大文件.txt", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if all.TotalLines != 5 || all.FromLine != 1 || all.ToLine != 5 || all.Truncated {
		t.Errorf("整读的区间不对：%+v", all)
	}
	if all.Text != "一\n二\n三\n四\n五" {
		t.Errorf("文本不对：%q", all.Text)
	}

	// 分页：从第 3 行取 2 行
	part, err := svc.ReadFile("raw/大文件.txt", 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if part.FromLine != 3 || part.ToLine != 4 || !part.Truncated || part.Text != "三\n四" {
		t.Errorf("分页不对：%+v", part)
	}
	// 起点超出文件长度：不是错误，给个空段
	past, err := svc.ReadFile("raw/大文件.txt", 99, 10)
	if err != nil {
		t.Errorf("起点超出不该报错：%v", err)
	}
	if past.FromLine != 0 || past.Text != "" {
		t.Errorf("超出范围该给空段：%+v", past)
	}
	// 上限夹紧：要 99999 行也只能给到上限
	huge, err := svc.ReadFile("raw/大文件.txt", 1, 99999)
	if err != nil {
		t.Fatal(err)
	}
	if huge.ToLine > readFileMaxLines {
		t.Errorf("该被上限夹住：%+v", huge)
	}
}

// TestReadFileConfinesToVault 是这个工具的安全边界：**不许越出 vault**。
//
// 不做「先拼接再清洗」：那会把 `../../x` 悄悄变成合法路径，等于静默放行。
func TestReadFileConfinesToVault(t *testing.T) {
	root := newVault(t)
	svc := New(root)
	for _, bad := range []string{"", "  ", "/etc/passwd", `C:\Windows\win.ini`, "../外面.txt", "raw/../../外面.txt", "..\\外面.txt"} {
		if _, err := svc.ReadFile(bad, 1, 10); err == nil {
			t.Errorf("%q 该被拒绝", bad)
		}
	}
	// 目录不是文件
	if _, err := svc.ReadFile("docs", 1, 10); err == nil {
		t.Error("目录该被拒绝")
	}
	// 不存在的文件要报清楚
	if _, err := svc.ReadFile("docs/没有这篇.md", 1, 10); err == nil {
		t.Error("不存在的文件该报错")
	}
}

func TestReadFileRejectsBinaryAndBig(t *testing.T) {
	root := newVault(t)
	// 非法 UTF-8：明确说「不是文本」，而不是给出乱码
	if err := os.MkdirAll(filepath.Join(root, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "raw", "二进制.bin"), []byte{0xff, 0xfe, 0x00, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(root)
	if _, err := svc.ReadFile("raw/二进制.bin", 1, 10); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("非文本该明确报出来：%v", err)
	}
	// 超大文件：这里不该硬读进来
	big := filepath.Join(root, "raw", "巨大.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := svc.ReadFile("raw/巨大.bin", 1, 10); err == nil || !strings.Contains(err.Error(), "太大") {
		t.Errorf("超大文件该被挡：%v", err)
	}
}
