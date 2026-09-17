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
	// DSHHome 是 harness 的配置根（profiles / sessions / .credentials.yaml 都在这儿）。
	//
	// ⚠️ **必须显式钉住**：默认值取决于环境变量 DSH_HOME，而它存不存在又取决于
	// 「App 是被谁启动的」——从 DSH 会话里启动会继承桌面版那套 home，直接双击启动则落到
	// `~/.dsh`。不钉住就会出现「同一个 profile 一会儿在一会儿不在」
	// （本机实测：`~/.dsh/profiles` 与 `%APPDATA%\dsh-desktop\harness\profiles` 是两个根）。
	DSHHome string `json:"dshHome"`
	// Profile 是起后端用的 profile 名（用我们建的 `ssot-agent`：它关掉了绕过能力层的工具）。
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
	// DefaultProfile 是**我们自己的** profile：它比通用的 acp 多一件事——
	// 关掉了 pwsh / bash / fs / str-replace-editor，agent 只能通过能力层碰 vault。
	DefaultProfile = "ssot-agent"
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
	if got.Agent.DSHHome == "" {
		got.Agent.DSHHome = def.Agent.DSHHome
		notes = append(notes, "agent.dshHome 为空，用默认值："+def.Agent.DSHHome)
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
			DSHHome:    DefaultDSHHome(),
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

// ProfileDir 是 profile 目录（在钉住的 DSHHome 下：`<dshHome>/profiles/<名字>`）。
func (a Agent) ProfileDir() string {
	if a.DSHHome == "" {
		return ""
	}
	return filepath.Join(a.DSHHome, "profiles", a.Profile)
}

// DefaultDSHHome 猜 harness 的配置根。
//
// 优先桌面版自己的那个（`%APPDATA%\\dsh-desktop\\harness`，本机装了桌面版就有它），
// 否则退回 DSH 的标准位置 `~/.dsh`。两台机器上这两套是**并行**的：桌面版一套、CLI 一套。
func DefaultDSHHome() string {
	if appData := os.Getenv("APPDATA"); appData != "" && runtime.GOOS == "windows" {
		desktop := filepath.Join(appData, "dsh-desktop", "harness")
		if _, err := os.Stat(desktop); err == nil {
			return desktop
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dsh")
}

// Args 是起 ACP 后端要拼的命令行。
//
// 形状是照桌面版自己的启动代码抄的（dsh.spec.md 里记着出处）：
// Electron 当 node 用，所以这三个参数一个都不能少。
func (a Agent) Args() []string {
	return []string{"--expose-internals", a.NodeEntry(), a.DSHBin(), "--profile", a.Profile}
}

// Env 是子进程要多带的环境变量。
//
//   - 不设 ELECTRON_RUN_AS_NODE 会被当成 Electron 应用启动，参数错位后只报
//     「--profile <name> is required」（那个报错完全看不出真实原因）；
//   - 显式给 DSH_HOME：不钉住的话，profile / 会话 / 凭据落在哪取决于 App 是谁启动的
//     （见 DSHHome 的注释）。
func (a Agent) Env() []string {
	env := []string{"ELECTRON_RUN_AS_NODE=1"}
	if a.DSHHome != "" {
		env = append(env, "DSH_HOME="+a.DSHHome)
	}
	return env
}

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
			Hint: fmt.Sprintf("需要在 %s 下建 profile：package.json 的 bundles 写 @deepseek-ai/dsh-base 与 @deepseek-ai/dsh-acp-app（见 docs/specs/dsh.spec.md）", a.ProfileDir()),
		},
	}
	// 光有 profile 目录不够：**必须堵住绕过能力层的工具**，
	// 否则 agent 能直接改 vault 文件、直接 commit，「门在能力层」就白设了。
	patchPath := filepath.Join(a.ProfileDir(), "cordis.patch.yml")
	guarded, missing := a.guardsBypassTools()
	items = append(items, Check{
		Name: "profile 关掉了绕过能力层的工具", Path: patchPath, OK: guarded,
		Hint: fmt.Sprintf("cordis.patch.yml 里要禁用 %s（禁用后 agent 只能走 mcp__ssot__*）", strings.Join(missing, "、")),
	})
	items = append(items, Check{
		Name: "ssot CLI（MCP 服务器）", Path: a.CLIBin, OK: exists(a.CLIBin),
		Hint: "跑 wails3 task build:cli 生成 bin\\ssot-cli.exe（App 起 MCP 服务器用的就是它）",
	})
	return items
}

// bypassToolIDs 是**必须禁用**的那几个工具行。
//
// 留着它们，agent 就能绕过能力层：pwsh/bash 能跑任意命令（包括 git commit），
// fs / str-replace-editor 能直接读写文件（包括改 front matter 里的 status）。
// 见 docs/specs/dsh.spec.md「后端工具集必须收紧」。
// ⚠️ `tool-fs-search`（glob/grep）**不在**这份清单里：它只有发现能力、写不了东西，
// 是纯只读。第一版把它一起关了，结果 agent 连找文件都不会——那是砍过头（见 dsh.spec.md）。
var bypassToolIDs = []string{"tool-pwsh", "tool-bash", "tool-fs", "tool-str-replace-editor"}

// guardsBypassTools 检查 profile 的 patch 有没有把那些工具关掉。
//
// 这是**配置检查**（读文件看有没有 `disabled: true`），不是安全边界——边界是运行时那行标志本身。
// 已知误判：patch 用 `!!js` 表达式动态决定时这里看不出来。
func (a Agent) guardsBypassTools() (bool, []string) {
	b, err := os.ReadFile(filepath.Join(a.ProfileDir(), "cordis.patch.yml"))
	if err != nil {
		return false, bypassToolIDs
	}
	lines := strings.Split(string(b), "\n")
	var missing []string
	for _, id := range bypassToolIDs {
		if !disabledIn(lines, id) {
			missing = append(missing, id)
		}
	}
	return len(missing) == 0, missing
}

// disabledIn 看某一行 id 后面（同一条目内）有没有 `disabled: true`。
//
// ⚠️ 窗口要给够：dump 出来的条目中间会夹 `__dshPluginOwner` 这类块，
// `disabled:` 可能落在 id 后面**第 7 行**；窗口只留 4 行会误判成「启用」
// （我就这么误读过一次：明明关掉了，却以为没关上）。
func disabledIn(lines []string, id string) bool {
	for i, line := range lines {
		if !strings.Contains(line, "id:") || !strings.Contains(line, id) {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+12; j++ {
			t := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t, "-") && strings.Contains(t, "id:") {
				break // 到下一条了
			}
			if strings.Contains(t, "disabled:") {
				// 只认字面 true：`!!js` 表达式要看运行时才知道，配置检查不该假装看得懂。
				return strings.Contains(t, "true")
			}
		}
	}
	return false
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
