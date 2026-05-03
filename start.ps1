#requires -Version 5.1
<#
.SYNOPSIS
  lingma2api 一键生产部署脚本（Windows / PowerShell）

.DESCRIPTION
  自动完成: 安装前端依赖 -> 构建前端 -> 构建后端二进制（嵌入 frontend-dist） -> 启动服务

.PARAMETER Config
  自定义 config.yaml 路径，默认 ./config.yaml

.PARAMETER SkipFrontend
  跳过前端构建（适用于 frontend-dist 已是最新的场景）

.EXAMPLE
  .\start.ps1
  .\start.ps1 -Config .\config.yaml
  .\start.ps1 -SkipFrontend
#>
param(
    [string]$Config = ".\config.yaml",
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

function Write-Step($msg) { Write-Host "[lingma2api] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[lingma2api] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[lingma2api] $msg" -ForegroundColor Yellow }
function Write-Err($msg)  { Write-Host "[lingma2api] $msg" -ForegroundColor Red }

# 1. 工具链检查
Write-Step "[1/4] 检查工具链..."
$missing = @()
if (-not (Get-Command go -ErrorAction SilentlyContinue))  { $missing += "go" }
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) { $missing += "npm" }
if ($missing.Count -gt 0) {
    Write-Err "缺少工具: $($missing -join ', ')，请先安装"
    exit 1
}
Write-Ok "  go: $((go version) -replace '^go version ', '')"
Write-Ok "  node: $(node --version)"

# 2. 配置检查（仅警告）
if (-not (Test-Path $Config)) {
    Write-Warn "未找到配置文件 $Config（继续，二进制启动时会报错）"
}
$authFile = ".\auth\credentials.json"
if (-not (Test-Path $authFile)) {
    Write-Warn "未找到 $authFile，启动后请通过 OAuth bootstrap 获取（参考 README）"
}

# 3. 构建前端
if (-not $SkipFrontend) {
    Write-Step "[2/4] 构建前端..."
    Push-Location ".\frontend"
    try {
        if (-not (Test-Path "node_modules")) {
            Write-Step "  安装 npm 依赖（首次运行较慢）..."
            npm install
            if ($LASTEXITCODE -ne 0) { throw "npm install 失败" }
        }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }
    } finally {
        Pop-Location
    }
    Write-Ok "  前端构建完成 -> frontend-dist/"
} else {
    Write-Warn "[2/4] 跳过前端构建（-SkipFrontend）"
}

# 4. 构建后端
Write-Step "[3/4] 构建后端二进制..."
$binary = ".\lingma2api.exe"
go build -o $binary .
if ($LASTEXITCODE -ne 0) {
    Write-Err "后端构建失败"
    exit $LASTEXITCODE
}
Write-Ok "  二进制 -> $binary"

# 5. 启动
Write-Step "[4/4] 启动服务..."
Write-Ok "  控制台:   http://127.0.0.1:8080"
Write-Ok "  OpenAI:   http://127.0.0.1:8080/v1"
Write-Ok "  Anthropic: http://127.0.0.1:8080/v1/messages"
Write-Host ""
Write-Host "按 Ctrl+C 停止服务" -ForegroundColor DarkGray
Write-Host ""

& $binary -config $Config
