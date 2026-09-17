<#
.SYNOPSIS
  起一个**会话级**的 DSH，把 ssot 的能力层作为 MCP 服务器挂上去。

.DESCRIPTION
  做什么：设好 SSOT_MCP_BIN / SSOT_MCP_VAULT 两个环境变量，再用 `--patch` 叠加
  仓库里的 .dsh\mcp.patch.yml，然后启动 DSH。只有**这一次会话**会多出
  mcp__ssot__* 这套工具；你平时写代码用的 DSH 一个字节都不动。

  为什么不写进 ~/.dsh/profiles/web/cordis.patch.yml：那份是**全局**的，
  所有会话（包括写代码的）都会多出这八个工具、白吃 token，还可能被误调。
  详见 docs/specs/dsh.spec.md §1。

.适用范围
  本机装了 DSH Desktop（Windows 上 DSH 没有独立 node.exe，是拿 Electron 当 node
  跑 harness）。要「让 agent 操作 vault」时用它。

.什么时候不该用
  - 别用它跑日常写代码的会话——那些会话不需要 vault 工具。
  - 桌面 GUI 里**没法**用这个 overlay：桌面版起 harness 时写死只叠加它自己的
    dsh-desktop.patch.yml，没有给用户加 patch 的位置。想看 GUI 就用这个脚本起的
    那个地址（脚本会把 URL 打出来）。

.EXAMPLE
  pwsh -File scripts\dsh\dsh-ssot.ps1                    # 用 projects\demo 这个示例 vault
  pwsh -File scripts\dsh\dsh-ssot.ps1 -Vault D:\myvault  # 指定自己的 vault
  pwsh -File scripts\dsh\dsh-ssot.ps1 -DumpConfig        # 只打印组装结果、不启动（自检）
#>
[CmdletBinding()]
param(
  # 要打开的 vault 根目录；默认用仓库里的示例 vault projects\demo
  [string]$Vault = "",
  # harness 监听端口（默认刻意避开桌面版那个）
  [int]$Port = 9300,
  # DSH Desktop 安装目录
  [string]$DshInstall = "",
  # 只打印组装后的配置树然后退出（零副作用，用来确认 overlay 生效）
  [switch]$DumpConfig
)

$ErrorActionPreference = "Stop"

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$overlay = Join-Path $repo ".dsh\mcp.patch.yml"
if (-not (Test-Path $overlay)) { throw "找不到 overlay：$overlay" }

if ([string]::IsNullOrWhiteSpace($Vault)) { $Vault = Join-Path $repo "projects\demo" }
if (-not (Test-Path $Vault)) { throw "vault 目录不存在：$Vault（用 -Vault 指定）" }
$Vault = (Resolve-Path $Vault).Path

if ([string]::IsNullOrWhiteSpace($DshInstall)) {
  $DshInstall = Join-Path $env:LOCALAPPDATA "Programs\DSH Desktop"
}
$dshExe = Join-Path $DshInstall "DSH Desktop.exe"
$nodeEntry = Join-Path $DshInstall "resources\harness-node-entry.mjs"
$dshBin = Join-Path $DshInstall "resources\app\node_modules\@deepseek-ai\dsh\lib\bin.js"
foreach ($p in @($dshExe, $nodeEntry, $dshBin)) {
  if (-not (Test-Path $p)) { throw "没找到 $p（DSH Desktop 装在哪？用 -DshInstall 指定）" }
}

# ssot 二进制：MCP 是**长驻进程**，DSH 起它、一直用它，中间还会重连，
# 所以必须是编译好的可执行文件（`go run` 那种一次性进程会立刻退出）。
$bin = Join-Path $repo "bin\ssot-cli.exe"
$srcNewer = Get-ChildItem (Join-Path $repo "cmd"), (Join-Path $repo "internal") -Recurse -Include *.go -ErrorAction SilentlyContinue |
  Sort-Object LastWriteTime -Descending | Select-Object -First 1
$needBuild = (-not (Test-Path $bin)) -or ($srcNewer -and $srcNewer.LastWriteTime -gt (Get-Item $bin).LastWriteTime)
if ($needBuild) {
  Write-Host "编译 ssot …" -ForegroundColor DarkGray
  # GOCACHE 必须落在工作区内：否则受限沙箱下 go 会被拒绝（见 AGENTS.md）
  $env:GOCACHE = Join-Path $repo ".gocache"
  Push-Location $repo
  try {
    & go build -o $bin ./cmd/ssot
    if ($LASTEXITCODE -ne 0) { throw "go build 失败（exit $LASTEXITCODE）" }
  } finally { Pop-Location }
}

$env:SSOT_MCP_BIN = $bin
$env:SSOT_MCP_VAULT = $Vault
# Windows 上拿 Electron 当 node 用：不设它，子进程会当成 Electron 应用启动，
# 参数位置错位后只会报「--profile <name> is required」。
$env:ELECTRON_RUN_AS_NODE = "1"

$args = @(
  "--expose-internals", $nodeEntry, $dshBin,
  "web",
  "--patch", $overlay
)
# ⚠️ 启动器自己的选项（--patch / --dump-config）必须排在 --host / --port **之前**：
# 那两个不是启动器的选项，`passThroughOptions` 一遇到它们就把后面全部当成 app 参数透传，
# 于是 --dump-config 会被 web app 当成自己的未知选项而报错。
# 而且 dump 模式**不收任何 app 参数**（多一个 --host 就报 "config dumps take no app arguments"）。
if ($DumpConfig) {
  $args += "--dump-config"
} else {
  $args += @("--host", "127.0.0.1", "--port", "$Port")
}

Write-Host "vault    : $Vault"
Write-Host "ssot     : $bin"
Write-Host "overlay  : $overlay"
Write-Host "端口     : $Port$(if ($DumpConfig) { '（只 dump 配置，不启动）' })"
Write-Host ""

& $dshExe @args
exit $LASTEXITCODE

