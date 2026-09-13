// 会话：当前打开的项目与当前选中的场景。
//
// 见 docs/specs/workspace.spec.md。界面此前把「当前项目」藏在启动参数里，
// 换项目要重启进程。会话把它变成可切换的状态，并且**切换必须是全有或全无**：
// 加载失败时保持原项目不变，绝不留下一个半加载的项目——
// 半加载的项目会让界面显示上一个项目的数据，而标题写着新项目。
package compose

import (
	"fmt"
	"sync"

	"github.com/ngnl5/ssot/internal/infrastructure/projectfile"
)

// Session 是界面的上下文。
//
// 它**不是数据**：关掉界面它就没了。因此它不落库、不参与断言身份。
type Session struct {
	// root 是项目根目录（如 "projects"）。发现项目时扫它。
	root string

	mu       sync.Mutex
	dir      string
	p        *Project
	scenario string
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

// Scenario 返回当前场景名；空串表示未选中或该项目没有场景。
func (s *Session) Scenario() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scenario
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
			return nil, fmt.Errorf("%s 下没有可用项目——一个项目就是一个含 project.yml 的目录", s.root)
		}
		s.dir = refs[0].Dir
	}
	p, err := Load(s.dir, true)
	if err != nil {
		return nil, fmt.Errorf("加载项目 %s：%w", s.dir, err)
	}
	s.p = p
	if s.scenario == "" && len(p.Scenarios) > 0 {
		s.scenario = p.Scenarios[0].Name
	}
	return p, nil
}

// Open 切换项目。
//
// **失败则原项目不变。** 先在新目录上完整装配，成功了才换指针——
// 中途失败时旧项目仍可用，界面不会塌成空白。
func (s *Session) Open(dir string) error {
	next, err := Load(dir, true)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.p
	s.p = next
	s.dir = dir
	// 场景跟着项目走：留着一个属于旧项目的场景名，会让所有场景级视图取错数。
	s.scenario = ""
	if len(next.Scenarios) > 0 {
		s.scenario = next.Scenarios[0].Name
	}
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// SelectScenario 选中一个场景。名称必须在该项目的场景列表中。
func (s *Session) SelectScenario(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.projectLocked()
	if err != nil {
		return err
	}
	if _, ok := p.Scenario(name); !ok {
		names := make([]string, 0, len(p.Scenarios))
		for _, sc := range p.Scenarios {
			names = append(names, sc.Name)
		}
		return fmt.Errorf("项目 %s 中没有场景 %q（可用：%v）", p.Name(), name, names)
	}
	s.scenario = name
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
