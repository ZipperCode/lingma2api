#requires -Version 5.1
<#
.SYNOPSIS
  lingma2api 开发模式启动脚本（Windows / PowerShell）

.DESCRIPTION
  并行启动:
    - Vite dev server (http://127.0.0.1:3000)  在新窗口
    - Go 后端 (http://127.0.0.1:8080)         在当前窗口
  Vite 已配置 /v1 与 /admin 自动代理到 8080，访问 http://127.0.0.1:3000 即可获得热更新体验。

.PARAMETER Config
  自定义 config.yaml 路径，默认 ./config.yaml

.EXAMPLE
  .\dev.ps1
  .\dev.ps1 -Config .\config.yaml
#>
param(
    [string]$Config = ".\config.yaml"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

function Write-Step($msg) { Write-Host "[lingma2api/dev] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[lingma2api/dev] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[lingma2api/dev] $msg" -ForegroundColor Yellow }
function Write-Err($msg)  { Write-Host "[lingma2api/dev] $msg" -ForegroundColor Red }

# 1. 工具链检查
Write-Step "[1/3] 检查工具链..."
$missing = @()
if (-not (Get-Command go -ErrorAction SilentlyContinue))  { $missing += "go" }
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) { $missing += "npm" }
if ($missing.Count -gt 0) {
    Write-Err "缺少工具: $($missing -join ', ')，请先安装"
    exit 1
}

# 2. 准备前端依赖
$frontendDir = Join-Path $Root "frontend"
if (-not (Test-Path (Join-Path $frontendDir "node_modules"))) {
    Write-Step "[2/3] 首次运行，安装前端依赖..."
    Push-Location $frontendDir
    try {
        npm install
        if ($LASTEXITCODE -ne 0) { throw "npm install 失败" }
    } finally {
        Pop-Location
    }
} else {
    Write-Step "[2/3] 前端依赖已就绪"
}

# 3. 启动 Vite dev server（新窗口）
Write-Step "[3/3] 启动 Vite dev server (新窗口) ..."
$viteProcess = Start-Process -FilePath "powershell.exe" `
    -ArgumentList "-NoExit", "-NoProfile", "-Command", "Set-Location '$frontendDir'; Write-Host 'Vite dev server' -ForegroundColor Green; npm run dev" `
    -PassThru -WindowStyle Normal

if (-not $viteProcess) {
    Write-Err "无法启动 Vite dev server"
    exit 1
}
Write-Ok "  Vite PID: $($viteProcess.Id)"

# 4. 启动 Go 后端（当前窗口）
Start-Sleep -Seconds 1
Write-Host ""
Write-Ok "  前端 (热更新): http://127.0.0.1:3000"
Write-Ok "  后端:          http://127.0.0.1:8080"
Write-Ok "  推荐访问:      http://127.0.0.1:3000  (Vite 自动代理 API)"
Write-Host ""
Write-Host "按 Ctrl+C 停止后端，并尝试关闭 Vite 窗口" -ForegroundColor DarkGray
Write-Host ""

try {
    go run . -config $Config
} finally {
    Write-Step "清理 Vite dev server (PID $($viteProcess.Id)) ..."
    if (-not $viteProcess.HasExited) {
        Stop-Process -Id $viteProcess.Id -Force -ErrorAction SilentlyContinue
        # 子进程清理（npm -> node）
        Get-CimInstance Win32_Process -Filter "ParentProcessId=$($viteProcess.Id)" `
            -ErrorAction SilentlyContinue | ForEach-Object {
                Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
            }
    }
    Write-Ok "已停止"
}
