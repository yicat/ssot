package vaultapp

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FileSlice 是一次只读文件读取的结果。
type FileSlice struct {
	// Path 是相对 vault 根的路径（永远 / 分隔）。
	Path string
	// Text 是取到的那一段（不含行号前缀——行号给 FromLine/ToLine）。
	Text string
	// FromLine / ToLine 是这次取到的行号区间（1 起，闭区间）。
	FromLine, ToLine int
	// TotalLines 是这个文件一共多少行，便于调用方决定要不要继续翻。
	TotalLines int
	// Truncated 表示还有没取到的部分。
	Truncated bool
}

// 读取上限：单次最多这些行、这些字节。
//
// 有上限是因为读文件是给模型看的：一次塞进几百 KB 会把上下文吃光，
// 也会让「到底读了什么」说不清。要更多就分页读。
const (
	readFileMaxLines = 2000
	readFileMaxBytes = 1 << 20 // 1 MiB
)

// ReadFile 只读地读 vault 里的一个文本文件（支持按行分页）。
//
// 为什么能力层要给这个：agent 需要读原文、JSON 导出、`project.yml`、表的说明文件……
// 这些不都是「文档」。但我们**不给它文件系统工具**——那样它就能直接改 vault、直接 commit，
// 把「写入回落 draft + 留痕」「agent 不能发布」两条规则绕过去。
// 所以：**读放开，写只走能力层**。
//
// 边界（都是硬性的，不做「尽力而为」）：
//   - 路径必须是 vault 内的相对路径：绝对路径与任何 `..` 一律拒绝（不做拼接后再清洗，
//     因为清洗会把 `../x` 悄悄变成合法路径，那是「静默放行」）；
//   - 只读文本：不是合法 UTF-8 就明确报错（图片等二进制不在这里读）；
//   - 单次读取有行数与字节上限，超了返回 Truncated，让调用方分页。
func (s *Service) ReadFile(rel string, fromLine, maxLines int) (FileSlice, error) {
	clean, err := s.confine(rel)
	if err != nil {
		return FileSlice{}, err
	}
	if maxLines <= 0 {
		maxLines = 400
	}
	if maxLines > readFileMaxLines {
		maxLines = readFileMaxLines
	}
	if fromLine <= 0 {
		fromLine = 1
	}

	full := filepath.Join(s.Root(), filepath.FromSlash(clean))
	fi, err := os.Stat(full)
	if err != nil {
		return FileSlice{}, fmt.Errorf("读不了 %s：%w", clean, err)
	}
	if fi.IsDir() {
		return FileSlice{}, fmt.Errorf("%s 是目录，不是文件", clean)
	}
	if fi.Size() > readFileMaxBytes*8 {
		return FileSlice{}, fmt.Errorf("%s 太大（%d 字节）——这里只读文本片段，大文件请用数据表或检索", clean, fi.Size())
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return FileSlice{}, err
	}
	if int64(len(b)) > readFileMaxBytes {
		b = b[:readFileMaxBytes]
	}
	if !utf8.Valid(b) {
		return FileSlice{}, fmt.Errorf("%s 不是 UTF-8 文本（二进制文件这里不读）", clean)
	}

	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	total := len(lines)
	// Split 会在结尾多给一个空元素（文件以换行结尾时），计数时去掉它更好理解。
	if total > 0 && lines[total-1] == "" {
		total--
	}
	if fromLine > total {
		return FileSlice{Path: clean, FromLine: 0, ToLine: 0, TotalLines: total}, nil
	}
	end := fromLine + maxLines - 1
	if end > total {
		end = total
	}
	slice := lines[fromLine-1 : end]
	return FileSlice{
		Path:       clean,
		Text:       strings.Join(slice, "\n"),
		FromLine:   fromLine,
		ToLine:     end,
		TotalLines: total,
		Truncated:  end < total,
	}, nil
}

// confine 把调用方给的路径收敛成「vault 内的相对路径」，越界一律报错。
func (s *Service) confine(rel string) (string, error) {
	t := strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if t == "" {
		return "", fmt.Errorf("必须给文件路径")
	}
	if path.IsAbs(t) || (len(t) > 1 && t[1] == ':') {
		return "", fmt.Errorf("只接受 vault 内的相对路径，收到绝对路径 %q", rel)
	}
	for _, seg := range strings.Split(t, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径里不许有 ..（收到 %q）——那会跑到 vault 外面去", rel)
		}
	}
	clean := path.Clean(strings.TrimPrefix(t, "./"))
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("路径不合法：%q", rel)
	}
	return clean, nil
}
