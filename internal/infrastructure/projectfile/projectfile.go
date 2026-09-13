// Package projectfile 读取与发现项目定义（project.yml）。
//
// 见 docs/specs/workspace.spec.md：项目列表只列出**含 project.yml 的目录**——
// 一个空目录或半成品目录不是项目。这条规则的落地处就是本包。
package projectfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Name 是项目定义的文件名。
const Name = "project.yml"

// File 是 project.yml 的内容。
type File struct {
	Project          string `yaml:"project"`
	Description      string `yaml:"description"`
	MetamodelVersion int    `yaml:"metamodelVersion"`
}

// Ref 是一个被发现的项目。
type Ref struct {
	// Dir 是项目目录（相对或绝对，取决于调用方给的 root）
	Dir string
	// Path 是 project.yml 的完整路径
	Path string
	File File
}

// Display 返回项目名；缺名时退回目录名，保证界面上永远有个可读的标识。
func (r Ref) Display() string {
	if r.File.Project != "" {
		return r.File.Project
	}
	return filepath.Base(r.Dir)
}

// Load 读取项目定义。
//
// 缺 `project` 名时拒绝：没有名字的项目无法在界面上被区分，
// 而「用目录名凑一个」会让 project.yml 里的名字与界面显示悄悄不一致。
func Load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("%s 解析失败：%w", path, err)
	}
	if f.Project == "" {
		return File{}, fmt.Errorf("%s 缺少 project 名", path)
	}
	return f, nil
}

// Discover 在 root 下发现全部项目（root/*/project.yml），按目录名排序。
//
// root 本身不存在时返回空列表而不是错误：第一次使用时还没有任何项目，
// 那是「一个都没有」，不是「出错了」——两者在界面上必须能区分。
func Discover(root string) ([]Ref, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Ref
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		path := filepath.Join(dir, Name)
		if _, err := os.Stat(path); err != nil {
			continue // 不是项目目录，静默跳过
		}
		f, err := Load(path)
		if err != nil {
			// 有 project.yml 但读不了：这是配置错误，必须报出来，
			// 不能当成「不是项目」悄悄跳过。
			return nil, err
		}
		out = append(out, Ref{Dir: dir, Path: path, File: f})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}
