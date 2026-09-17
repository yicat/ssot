// Package appconfig 读写 App 自己的设置。
//
// 只管我们自己的三层（docs/specs/settings.spec.md）：**项目 / Agent 后端 / 外观**。
// 模型与 API key **不在这里**——那是会话级（`configOptions`）与后端自己的事；
// 复制一层一定会跟它打架。
//
// 存哪：用户级一份（`os.UserConfigDir()/ssot/settings.json`），理由见 spec §3——
// 里面有本机路径（DSH 装在哪、CLI 二进制在哪），换机器不该带着走，也不该写进 vault。
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Settings 是全部设置。
type Settings struct {
	ProjectsRoot string     `json:"projectsRoot"`
	Agent        Agent      `json:"agent"`
	Appearance   Appearance `json:"appearance"`
}

// Agent 是后端相关的设置。
type Agent struct {
	// DSHInstall 是 DSH Desktop 的安装目录（下面有 DSH Desktop.exe 与 resources/）。
	DSHInstall string `json:"dshInstall"`
	// Profile 是起后端用的 profile 名（我们建的那个叫 acp）。
	Profile string `json:"profile"`
	// CLIBin 是 ssot CLI 的路径——**MCP 服务器就是它**（`<CLIBin> mcp --root <vault>`）。
	CLIBin string `json:"cliBin"`
	// Actor 是 agent 写入时记在 git trailer 里的名字（`agent:<名字>`）。
	Actor string `json:"actor"`
}

// Appearance 是外观（主题；字号口径在 typography.spec.md 里定死，不给随便拧）。
type Appearance struct {
	Theme string `json:"theme"` // system | light | dark
}

// 默认值（写下来免得靠猜）。
const (
	DefaultProfile = "acp"
	DefaultActor   = "agent:dsh"
	DefaultTheme   = "system"
)

// Store 是设置文件的读写入口。
type Store struct{ path string }

// Open 打开（不创建）用户级设置文件。目录不存在不算错——第一次跑就是没有。
func Open() (*Store, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "settings.json")}, nil
}

// Dir 返回设置目录（用户级）。
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("拿不到用户配置目录：%w", err)
	}
	return filepath.Join(base, "ssot"), nil
}

// Path 是设置文件的完整路径。
func (s *Store) Path() string { return s.path }

// Load 读设置。
//
// 三种情况分得很清（spec §3：**不许静默用默认值**）：
//   - 文件不存在 → 返回默认值 + 一条说明（第一次跑就是这样，不是错）
//   - 文件读不动/不是合法 JSON → **报错**，让上层明确告诉人「配置读不了，现在用的是默认值」
//   - 字段缺 → 用默认值补（不算错，但会返回补了什么）
func (s *Store) Load() (Settings, []string, error) {
	def := Defaults()
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return def, []string{"还没有设置文件（用的是默认值）：" + s.path}, nil
	}
	if err != nil {
		return def, nil, fmt.Errorf("设置文件读不了（%s）：%w", s.path, err)
	}
	var got Settings
	if err := json.Unmarshal(b, &got); err != nil {
		return def, nil, fmt.Errorf("设置文件不是合法 JSON（%s）：%w", s.path, err)
	}
	var notes []string
	if got.ProjectsRoot == "" {
		got.ProjectsRoot = def.ProjectsRoot
		notes = append(notes, "projectsRoot 为空，用默认值："+def.ProjectsRoot)
	}
	if got.Agent.DSHInstall == "" {
		got.Agent.DSHInstall = def.Agent.DSHInstall
		notes = append(notes, "agent.dshInstall 为空，用默认值："+def.Agent.DSHInstall)
	}
	if got.Agent.Profile == "" {
		got.Agent.Profile = def.Agent.Profile
		notes = append(notes, "agent.profile 为空，用默认值："+def.Agent.Profile)
	}
	if got.Agent.CLIBin == "" {
		got.Agent.CLIBin = def.Agent.CLIBin
		notes = append(notes, "agent.cliBin 为空，用默认值："+def.Agent.CLIBin)
	}
	if got.Agent.Actor == "" {
		got.Agent.Actor = def.Agent.Actor
		notes = append(notes, "agent.actor 为空，用默认值："+def.Agent.Actor)
	}
	if got.Appearance.Theme == "" {
		got.Appearance.Theme = def.Appearance.Theme
	}
	return got, notes, nil
}

// Save 写设置（先建目录；权限 0o700/0o600——里面是本机路径，不是秘密但也没必要放开）。
func (s *Store) Save(v Settings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(b, '\n'), 0o600)
}

// Defaults 是默认设置。
//
// 默认值都指向**本机的常见位置**，并且 CLI 默认取「App 自己那个目录下的 ssot-cli.exe」——
// App 与 CLI 是一起构建的，摆在一起是最省事的默认。
func Defaults() Settings {
	install := ""
	if local := os.Getenv("LOCALAPPDATA"); local != "" && runtime.GOOS == "windows" {
		install = filepath.Join(local, "Programs", "DSH Desktop")
	} else if home, err := os.UserHomeDir(); err == nil {
		install = filepath.Join(home, "Applications", "DSH Desktop")
	}
	return Settings{
		ProjectsRoot: "projects",
		Agent: Agent{
			DSHInstall: install,
			Profile:    DefaultProfile,
			CLIBin:     DefaultCLIPath(),
			Actor:      DefaultActor,
		},
		Appearance: Appearance{Theme: DefaultTheme},
	}
}

// DefaultCLIPath 猜 CLI 的位置：App 二进制同目录下的 ssot-cli(.exe)。
func DefaultCLIPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "ssot-cli"
	}
	name := "ssot-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name)
}

// 后端要用的三个文件（都能从安装目录推出来）。
func (a Agent) DSHExe() string { return filepath.Join(a.DSHInstall, "DSH Desktop.exe") }
func (a Agent) NodeEntry() string {
	return filepath.Join(a.DSHInstall, "resources", "harness-node-entry.mjs")
}

func (a Agent) DSHBin() string {
	return filepath.Join(a.DSHInstall, "resources", "app", "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
}

// ProfileDir 是 profile 目录（本机是 `~/.dsh/profiles/<名字>`）。
func (a Agent) ProfileDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dsh", "profiles", a.Profile)
}

// Args 是起 ACP 后端要拼的命令行。
//
// 形状是照桌面版自己的启动代码抄的（dsh.spec.md 里记着出处）：
// Electron 当 node 用，所以这三个参数一个都不能少。
func (a Agent) Args() []string {
	return []string{"--expose-internals", a.NodeEntry(), a.DSHBin(), "--profile", a.Profile}
}

// Env 是子进程要多带的环境变量。不设 ELECTRON_RUN_AS_NODE 会被当成 Electron 应用启动，
// 参数错位后只报「--profile <name> is required」（那个报错完全看不出真实原因）。
func (a Agent) Env() []string { return []string{"ELECTRON_RUN_AS_NODE=1"} }

// Check 逐条检查后端能不能用（spec §4：配置页要能报「缺什么」）。
//
// 只报告，**不自动修**：装 DSH、建 profile 都是人的动作，静默改用户 home 下的东西更不行。
type Check struct {
	Name string `json:"name"`
	Path string `json:"path"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint"`
}

// Check 返回逐条检查结果。
func (s Settings) Check() []Check {
	exists := func(p string) bool {
		if strings.TrimSpace(p) == "" {
			return false
		}
		_, err := os.Stat(p)
		return err == nil
	}
	a := s.Agent
	items := []Check{
		{Name: "DSH 可执行文件", Path: a.DSHExe(), OK: exists(a.DSHExe()),
			Hint: "DSH Desktop 装在哪？改「Agent 后端 → DSH 安装目录」"},
		{Name: "harness 入口", Path: a.NodeEntry(), OK: exists(a.NodeEntry()),
			Hint: "DSH 安装不完整（缺 resources\\harness-node-entry.mjs）"},
		{Name: "dsh 入口", Path: a.DSHBin(), OK: exists(a.DSHBin()),
			Hint: "DSH 安装不完整（缺 dsh/lib/bin.js）"},
		{Name: "profile 目录", Path: a.ProfileDir(), OK: exists(a.ProfileDir()),
			Hint: fmt.Sprintf("需要 %s/profile/package.json：bundles 写 @deepseek-ai/dsh-base 与 @deepseek-ai/dsh-acp-app（见 docs/specs/dsh.spec.md）", a.Profile),
		},
		{Name: "ssot CLI（MCP 服务器）", Path: a.CLIBin, OK: exists(a.CLIBin),
			Hint: "跑 wails3 task build:cli 生成 bin\\ssot-cli.exe（App 起 MCP 服务器用的就是它）"},
	}
	return items
}

// AllOK 报告后端是否齐了。
func (s Settings) AllOK() bool {
	for _, c := range s.Check() {
		if !c.OK {
			return false
		}
	}
	return true
}
