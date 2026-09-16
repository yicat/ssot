// 会话：当前打开的项目。
//
// ⚠️ 当前状态：**骨架**（上一套方案已作废，见 compose.go 的说明）。
// 保留「切换项目是全有或全无」这一条：加载失败时保持原项目不变，
// 绝不留下一个半加载的项目——那是与方案无关的、写界面就会踩到的坑。
package compose

import (
	"fmt"
	"sync"

	"github.com/ngnl5/ssot/internal/infrastructure/projectfile"
)

// Session 是界面的上下文。它**不是数据**：关掉界面它就没了。
type Session struct {
	// root 是项目根目录（如 "projects"）。发现项目时扫它。
	root string

	mu  sync.Mutex
	dir string
	p   *Project
}

// NewSession 构造会话。dir 为空时取 root 下的第一个项目。
func NewSession(root, dir string) *Session {
	return &Session{root: root, dir: dir}
}

// Root 返回项目根目录。
func (s *Session) Root() string { return s.root }

// Dir 返回当前项目目录（可能尚未加载）。
func (s *Session) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir
}

// Project 返回当前项目，必要时惰性加载。
//
// 未指定目录时取项目列表的第一个；一个都没有时明确报「没有可用项目」，
// 而不是拼一个不存在的路径去撞文件系统——那会让人以为是权限或路径问题。
func (s *Session) Project() (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projectLocked()
}

func (s *Session) projectLocked() (*Project, error) {
	if s.p != nil {
		return s.p, nil
	}
	if s.dir == "" {
		refs, err := projectfile.Discover(s.root)
		if err != nil {
			return nil, fmt.Errorf("发现项目失败：%w", err)
		}
		if len(refs) == 0 {
			return nil, fmt.Errorf("%s 下没有可用项目——一个项目就是一个含 %s 的目录",
				s.root, projectfile.Name)
		}
		s.dir = refs[0].Dir
	}
	p, err := Load(s.dir)
	if err != nil {
		return nil, fmt.Errorf("加载项目 %s：%w", s.dir, err)
	}
	s.p = p
	return p, nil
}

// Open 切换项目。**失败则原项目不变**：先在新目录上完整装配，成功了才换指针。
func (s *Session) Open(dir string) error {
	next, err := Load(dir)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.p
	s.p = next
	s.dir = dir
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// Close 释放当前项目。
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return nil
	}
	err := s.p.Close()
	s.p = nil
	return err
}
