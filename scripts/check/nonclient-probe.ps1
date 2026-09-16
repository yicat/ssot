<#
非客户区探针：确认自绘标题栏在 Windows 上真的生效。

做什么
  对着正在运行的 ssot 窗口做三件事：
    1. 量客户区原点：客户区顶端与窗口顶端相差多少。差 ~31px 说明原生标题栏还在，
       差 ~0 说明客户区铺满整窗、原生那条已经没了
    2. 沿标题栏**横向**扫描，向窗口发 WM_NCHITTEST(0x84)，打印每段的命中码
       命中码：0=HTNOWHERE 1=HTCLIENT(普通内容区) 2=HTCAPTION(可拖) 8=HTMINIMIZE
               9=HTMAXIMIZE 10=HTLEFT 11=HTRIGHT 12=HTTOP 20=HTCLOSE
    3. 在**窗口按钮**的中线**竖向**扫描，量出标题栏 bar 有多高
       （窗口按钮在 bar 里通高；顶上几行 HTTOP 是缩放进来的边框。
        别拿可拖填充区当基准——它被 bar 的内边距内缩过，量出来会偏小）
  输出是**文本**证据：不能看屏幕（或截图读不了）时，用它确认「哪一段 x 是可拖区、
  哪一段是窗口按钮、哪一段是普通内容区、bar 多高」。

适用范围
  - Windows，且应用正以 dev 或 build 起的窗口运行（进程名默认 ssot）
  - 窗口带着 Frameless + Windows.WebView2CompositionHosting（见 docs/specs/shell.spec.md）
  - 横向结论对应窗口顶边往下 -Y 像素那条线（默认 20px，落在那条标题栏里）

什么时候不该用
  - 进程没起来 / 不是 Windows：没有窗口可探，直接退出
  - 窗口刚出现、还没加载完时命中码会全是 1，看着像没生效——等几秒再跑
  - **不要用 WS_CAPTION 判断原生标题栏在不在**：Wails 的 Frameless 是靠 WM_NCCALCSIZE
    把客户区铺满整窗，样式位 WS_CAPTION / WS_THICKFRAME 是**故意保留**的
    （WS_THICKFRAME 还要给缩放用，见 pkg/application/webview_window_windows.go 的
    WM_NCCALCSIZE 注释）。本脚本因此用客户区原点判断，样式位只作参考打印。
  - 它断言的是**命中码**，不是像素：不要拿输出的 x 值当设计稿尺寸用（DPI 缩放会变）。
    y 值同理，但它量出的那一段**高度**就是 bar 高，可用来核对布局意图
  - 别拿它当压力测试反复刷——每条消息都排在目标窗口的 UI 线程上

存盘编码：本文件要存成 **UTF-8 with BOM**。Windows PowerShell 5.1 读无 BOM 的 UTF-8
会把中文注释按 ANSI 解码，引号错位后直接报语法错误（`Unexpected token`）。
#>
param(
    [string]$ProcessName = "ssot",
    [int]$Y = 20,
    [int]$Step = 16
)

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public class NcProbe {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
    [StructLayout(LayoutKind.Sequential)] public struct POINT { public int X; public int Y; }
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT r);
    [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr hWnd, ref POINT p);
    [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
    [DllImport("user32.dll", EntryPoint = "GetWindowLongPtrW")] public static extern IntPtr GetWindowLongPtr(IntPtr hWnd, int nIndex);
}
'@

$proc = Get-Process -Name $ProcessName -ErrorAction SilentlyContinue |
    Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $proc) {
    Write-Error "找不到进程 $ProcessName 的窗口（应用没起来？）"
    exit 1
}

$hwnd = $proc.MainWindowHandle
$rect = New-Object 'NcProbe+RECT'
[void][NcProbe]::GetWindowRect($hwnd, [ref]$rect)
$origin = New-Object 'NcProbe+POINT'
[void][NcProbe]::ClientToScreen($hwnd, [ref]$origin)
$width = $rect.Right - $rect.Left
$height = $rect.Bottom - $rect.Top
$frameTop = $origin.Y - $rect.Top
$frameLeft = $origin.X - $rect.Left

$GWL_STYLE = -16
$style = [int64][NcProbe]::GetWindowLongPtr($hwnd, $GWL_STYLE)

"窗口：$($proc.ProcessName) hwnd=$hwnd 位置=$($rect.Left),$($rect.Top) 尺寸=${width}x${height}"
"标题：[$($proc.MainWindowTitle)]"
"样式：style=0x{0:X8}  WS_CAPTION={1}  WS_THICKFRAME={2}（保留是正常的，见文件头）" -f $style,
    [bool]($style -band 0x00C00000), [bool]($style -band 0x00040000)
"客户区原点：相对窗口左上角 +$frameLeft,+$frameTop"

if ($frameTop -gt 4) {
    "  → 客户区上方还压着约 ${frameTop}px：原生标题栏可能还在（Frameless 没生效？）"
}
else {
    "  → 客户区从窗口顶部开始：原生标题栏已去掉，顶上就是自绘的那条"
}

$names = @{
    0 = "HTNOWHERE"; 1 = "HTCLIENT"; 2 = "HTCAPTION"; 3 = "HTSYSMENU"; 8 = "HTMINIMIZE"
    9 = "HTMAXIMIZE"; 10 = "HTLEFT"; 11 = "HTRIGHT"; 12 = "HTTOP"; 20 = "HTCLOSE"
}

function Get-RunLength {
    param([array]$Samples, [string]$AxisLabel, [int]$Offset)
    $out = @()
    $start = 0
    for ($i = 1; $i -le $Samples.Count; $i++) {
        if ($i -eq $Samples.Count -or $Samples[$i].Code -ne $Samples[$start].Code) {
            $name = $names[$Samples[$start].Code]
            if (-not $name) { $name = "未知($($Samples[$start].Code))" }
            $out += ("  {0} {1,5} - {2,5} : {3}" -f $AxisLabel, ($Samples[$start].Pos - $Offset),
                ($Samples[$i - 1].Pos - $Offset), $name)
            $start = $i
        }
    }
    return $out
}

# ── 横向扫描 ────────────────────────────────────────────────
$scanY = $rect.Top + $Y
$hits = @()
for ($x = $rect.Left + 2; $x -lt $rect.Right - 2; $x += $Step) {
    # WM_NCHITTEST 的 lParam 是【屏幕坐标】，低位 x、高位 y
    $lp = (($scanY -band 0xFFFF) -shl 16) -bor ($x -band 0xFFFF)
    $code = [int][NcProbe]::SendMessage($hwnd, 0x84, [IntPtr]::Zero, [IntPtr][int64]$lp)
    $hits += [pscustomobject]@{ Pos = $x; Code = $code }
}

"`n横向扫描：窗口顶边往下 $Y px，步长 $Step px（x 为距窗口左边的像素）"
Get-RunLength -Samples $hits -AxisLabel 'x' -Offset $rect.Left

# ── 竖向扫描：量标题栏 bar 的高度 ────────────────────────────
# 扫**窗口按钮**（HTMINIMIZE/HTMAXIMIZE/HTCLOSE）：它们在 bar 里是通高的
# （`items-stretch`），所以那一段的高度就是 bar 的高度。
# 别扫可拖填充区——那是我标的元素，被 bar 的内边距内缩过，量出来会偏小（踩过）。
$target = $null
$start = 0
for ($i = 1; $i -le $hits.Count; $i++) {
    if ($i -eq $hits.Count -or $hits[$i].Code -ne $hits[$start].Code) {
        if ($hits[$start].Code -in 8, 9, 20) {
            $w = $hits[$i - 1].Pos - $hits[$start].Pos
            if (-not $target -or $w -gt $target.Width) {
                $target = [pscustomobject]@{
                    Center = $hits[$start].Pos + [int]($w / 2)
                    Width  = $w
                    What   = $names[$hits[$start].Code]
                }
            }
        }
        $start = $i
    }
}

if ($target) {
    $vhits = @()
    for ($y = 1; $y -le 80; $y += 2) {
        $lp = ((($rect.Top + $y) -band 0xFFFF) -shl 16) -bor ($target.Center -band 0xFFFF)
        $code = [int][NcProbe]::SendMessage($hwnd, 0x84, [IntPtr]::Zero, [IntPtr][int64]$lp)
        $vhits += [pscustomobject]@{ Pos = $rect.Top + $y; Code = $code }
    }
    "`n竖向扫描：在 $($target.What) 的中线（距左边 $($target.Center - $rect.Left) px）往下扫"
    "（y 为距窗口顶边的像素；窗口按钮在 bar 里通高，所以它那一段就是 bar 的高）"
    Get-RunLength -Samples $vhits -AxisLabel 'y' -Offset $rect.Top
}
else {
    "`n竖向扫描：横向没找到窗口按钮（HTMINIMIZE/HTMAXIMIZE/HTCLOSE），跳过"
}
