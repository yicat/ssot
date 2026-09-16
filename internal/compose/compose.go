// Package compose 是组合根：唯一允许同时依赖各层的包。
//
// ⚠️ 当前状态：**骨架**。上一套方案（六部件 + 断言库 + 核验流程）已整体作废，
// 业务代码与规格集已清空，新的设计待定。这里只保留「项目能被发现与打开」
// 这一件与方案无关的事——界面要有东西可选，不然连壳都跑不起来。
//
// 依赖方向（对新写的代码同样适用）：api → application → domain ← infrastructure。
package compose

import (
	"fmt"
	"path/filepath"

	"github.com/ngnl5/ssot/internal/infrastructure/projectfile"
)

// Project 是一个已打开的项目。
//
// 现在它只有目录与 project.yml 里的名字/描述；新方案要用什么结构，
// 等设计定下来再长——不要为了「看起来完整」先塞一批字段进去。
type Project struct {
	// Dir 是项目目录。
	Dir string
	// File 是 project.yml 的内容。
	File projectfile.File
}

// Name 返回项目名；缺名时退回目录名。
func (p *Project) Name() string {
	if p.File.Project != "" {
		return p.File.Project
	}
	return filepath.Base(p.Dir)
}

// Description 返回项目描述。
func (p *Project) Description() string { return p.File.Description }

// Close 释放资源（当前没有需要释放的，留着是为了让调用方不必关心这一点）。
func (p *Project) Close() error { return nil }

// Load 打开一个项目目录。
func Load(dir string) (*Project, error) {
	f, err := projectfile.Load(filepath.Join(dir, projectfile.Name))
	if err != nil {
		return nil, fmt.Errorf("加载项目定义：%w", err)
	}
	return &Project{Dir: dir, File: f}, nil
}
