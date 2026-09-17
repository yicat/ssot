package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	s := &Store{path: filepath.Join(t.TempDir(), "nope", "settings.json")}
	got, notes, err := s.Load()
	if err != nil {
		t.Fatalf("文件不存在不该是错（第一次跑就是这样）：%v", err)
	}
	if got.Agent.Profile != DefaultProfile || got.Agent.Actor != DefaultActor {
		t.Errorf("该给默认值：%+v", got)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "还没有设置文件") {
		t.Errorf("该有一条「用的是默认值」的说明：%v", notes)
	}
}

func TestBrokenFileIsAnErrorNotSilentDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// 坏 JSON：必须明确报错。静默用默认值最坏——人以为自己配的东西生效了。
	if err := os.WriteFile(path, []byte("{ 这不是 json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := (&Store{path: path}).Load()
	if err == nil {
		t.Fatal("坏文件必须报错，不许静默吞掉")
	}
	if !strings.Contains(err.Error(), "不是合法 JSON") || !strings.Contains(err.Error(), path) {
		t.Errorf("错误信息要说清是什么问题、哪个文件：%v", err)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "嵌套", "settings.json")
	s := &Store{path: path}
	want := Defaults()
	want.Agent.Profile = "acp-2"
	want.Agent.CLIBin = `C:\somewhere\ssot-cli.exe`
	want.Appearance.Theme = "dark"
	if err := s.Save(want); err != nil {
		t.Fatalf("保存该自动建目录：%v", err)
	}
	got, notes, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("字段齐全时不该有补默认值的说明：%v", notes)
	}
	if got.Agent.Profile != "acp-2" || got.Appearance.Theme != "dark" || got.Agent.CLIBin != want.Agent.CLIBin {
		t.Errorf("读回来的不一致：%+v", got)
	}
}

func TestPartialFileFillsDefaultsAndSaysSo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// 只写了半截（手工编辑常见）：缺的补默认值，并且**说清补了什么**。
	if err := os.WriteFile(path, []byte(`{"agent":{"profile":"acp"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, notes, err := (&Store{path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent.Profile != "acp" {
		t.Errorf("已有的值不该被覆盖：%+v", got.Agent)
	}
	if got.Agent.CLIBin == "" || got.Agent.Actor != DefaultActor {
		t.Errorf("缺的该补上：%+v", got.Agent)
	}
	if len(notes) < 3 {
		t.Errorf("该逐条说明补了什么：%v", notes)
	}
}

func TestCheckReportsEachMissingPiece(t *testing.T) {
	s := Defaults()
	s.Agent.DSHInstall = filepath.Join(t.TempDir(), "不存在的 DSH")
	s.Agent.CLIBin = filepath.Join(t.TempDir(), "也不存在", "ssot-cli.exe")
	// ⚠️ 得换一个**不存在**的 profile 名：默认那个 `acp` 在本机是真存在的（建过），
	// 用默认名测「缺失」会假红——第一版就是这样被测试自己抓出来的。
	s.Agent.Profile = "不存在的-profile"

	items := s.Check()
	if len(items) < 5 {
		t.Fatalf("该逐条检查后端（现在 %d 条）", len(items))
	}
	for _, it := range items {
		if it.OK {
			t.Errorf("这些路径都不存在，不该报 OK：%+v", it)
		}
		if it.Hint == "" {
			t.Errorf("每条都要给「怎么补」：%+v", it)
		}
	}
	if s.AllOK() {
		t.Error("缺东西时 AllOK 该是 false")
	}
}

func TestAgentArgsAndEnv(t *testing.T) {
	a := Agent{DSHInstall: `C:\d`, Profile: "ssot-agent", DSHHome: `C:\dshhome`}
	args := strings.Join(a.Args(), " ")
	// 这三个参数一个都不能少（Electron 当 node 用；见 dsh.spec.md）。
	for _, want := range []string{"--expose-internals", "harness-node-entry.mjs", "bin.js", "--profile ssot-agent"} {
		if !strings.Contains(args, want) {
			t.Errorf("命令行缺 %q：%s", want, args)
		}
	}
	env := strings.Join(a.Env(), " ")
	if !strings.Contains(env, "ELECTRON_RUN_AS_NODE=1") {
		t.Errorf("环境变量缺 ELECTRON_RUN_AS_NODE（少了它会报 --profile is required）：%v", a.Env())
	}
	// DSH_HOME 必须显式钉住：不钉住，profile/会话/凭据落在哪取决于 App 是谁启动的。
	if !strings.Contains(env, "DSH_HOME=") {
		t.Errorf("环境变量缺 DSH_HOME：%v", a.Env())
	}
}

// TestGuardsBypassTools 验「profile 到底堵没堵住」这个检查。
//
// 样例照**真的 dump 形状**写：条目中间会夹 `__dshPluginOwner` 块，`disabled:` 落在 id 后面第 7 行——
// 我第一版窗口只留 4 行，于是把「已经关掉」读成「没关」，白折腾了一轮。这条就是那个 bug 的回归。
func TestGuardsBypassTools(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "profiles", "ssot-agent")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	patch := `# 说明
- id: tool-pwsh
  disabled: true
- id: tool-bash
  disabled: true
- id: tool-fs
  name: '@deepseek-ai/dsh-tool-fs'
  __dshPluginOwner:
    packageName: '@deepseek-ai/dsh-base'
    version: 0.1.2-rc.1
  disabled: true
- id: tool-fs-search
  disabled: true
- id: tool-str-replace-editor
  disabled: true
- id: tool-skill
  disabled: false
`
	if err := os.WriteFile(filepath.Join(profile, "cordis.patch.yml"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Agent{DSHHome: dir, Profile: "ssot-agent"}
	ok, missing := a.guardsBypassTools()
	if !ok {
		t.Errorf("这份 patch 已经全关掉了，不该报缺：%v", missing)
	}

	// 少关一个 → 必须报出来（假绿最危险：以为堵住了其实没堵）
	weak := strings.Replace(patch, "- id: tool-fs-search\n  disabled: true\n", "", 1)
	if err := os.WriteFile(filepath.Join(profile, "cordis.patch.yml"), []byte(weak), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, missing = a.guardsBypassTools()
	if ok || len(missing) != 1 || missing[0] != "tool-fs-search" {
		t.Errorf("少关一个该被报出来：ok=%v missing=%v", ok, missing)
	}

	// `!!js` 条件式不算「关掉了」：运行时才知道，配置检查不该假装看得懂。
	cond := strings.Replace(patch, "- id: tool-fs-search\n  disabled: true\n", "- id: tool-fs-search\n  disabled: !!js process.env.NOPE\n", 1)
	if err := os.WriteFile(filepath.Join(profile, "cordis.patch.yml"), []byte(cond), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.guardsBypassTools(); ok {
		t.Error("条件式禁用不该被当成「已关掉」")
	}

	// 没有 patch 文件 → 报缺全部（别静默当成没问题）。
	if err := os.Remove(filepath.Join(profile, "cordis.patch.yml")); err != nil {
		t.Fatal(err)
	}
	if ok, missing := a.guardsBypassTools(); ok || len(missing) != len(bypassToolIDs) {
		t.Errorf("没有 patch 文件该报缺全部：ok=%v missing=%v", ok, missing)
	}
}
