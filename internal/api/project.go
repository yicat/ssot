// 项目接口：让界面能列出并切换项目。
//
// ⚠️ 当前状态：**骨架**。上一套方案已整体作废（见 internal/compose/compose.go），
// 这里只留下与方案无关的那一件：有哪些项目、当前是哪个。
// 新方案的服务方法按设计逐个加到这里，而不是先铺一堆空壳。
package api

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/infrastructure/projectfile"
)

// ProjectRef 是一个可选的项目（面向界面）。
type ProjectRef struct {
	Dir  string `json:"dir"`
	Name string `json:"name"`
	// Path 是完整路径。两个项目目录同名是允许的，
	// 因此界面上必须显示完整路径才能区分。
	Path        string `json:"path"`
	Description string `json:"description"`
	// Current 标记它是不是当前打开的那个。
	Current bool `json:"current"`
}

// SessionState 是当前会话（面向界面）。
type SessionState struct {
	Dir         string `json:"dir"`
	Name        string `json:"name"`
	ProjectDir  string `json:"projectDir"`
	Description string `json:"description"`
}

// ProjectService 暴露项目与会话。
type ProjectService struct {
	session *compose.Session
}

// NewProjectService 构造服务。
func NewProjectService(s *compose.Session) *ProjectService { return &ProjectService{session: s} }

// Projects 列出项目根目录下的全部项目，按目录名排序。
func (s *ProjectService) Projects() ([]ProjectRef, error) {
	refs, err := projectfile.Discover(s.session.Root())
	if err != nil {
		return nil, err
	}
	cur := s.session.Dir()
	out := make([]ProjectRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, ProjectRef{
			Dir: r.Dir, Name: r.Display(), Path: r.Path,
			Description: r.File.Description, Current: r.Dir == cur,
		})
	}
	return out, nil
}

// Open 切换项目。**失败时会话不变**，因此界面不会塌成空白。
func (s *ProjectService) Open(dir string) (SessionState, error) {
	if dir == "" {
		return SessionState{}, fmt.Errorf("必须指明项目目录")
	}
	if err := s.session.Open(dir); err != nil {
		return SessionState{}, err
	}
	return s.Current()
}

// Current 返回当前会话。
func (s *ProjectService) Current() (SessionState, error) {
	p, err := s.session.Project()
	if err != nil {
		return SessionState{}, err
	}
	return SessionState{
		Dir: p.Dir, Name: p.Name(), ProjectDir: p.Dir, Description: p.Description(),
	}, nil
}
