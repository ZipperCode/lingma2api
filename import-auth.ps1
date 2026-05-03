#requires -Version 5.1
<#
.SYNOPSIS
  从本机已登录的 Lingma 程序提取认证信息，导入到项目的 auth/credentials.json

.DESCRIPTION
  包装 cmd/lingma-import-cache 工具：
    1. 检查 Lingma 缓存目录是否存在（默认 ~/.lingma）
    2. 调用 go run ./cmd/lingma-import-cache 读取缓存并派生凭据
    3. 写出到项目的 auth/credentials.json（默认）

  仅作一次性迁移使用。运行态服务不会再读取 ~/.lingma。

.PARAMETER LingmaDir
  Lingma 安装目录，默认 ~/.lingma（或 $env:USERPROFILE\.lingma）

.PARAMETER Output
  输出凭据文件路径，默认 ./auth/credentials.json

.PARAMETER Force
  目标文件已存在时直接覆盖（不提示）

.EXAMPLE
  .\import-auth.ps1
  .\import-auth.ps1 -Output .\auth\credentials.json -Force
  .\import-auth.ps1 -LingmaDir D:\custom\.lingma
#>
param(
    [string]$LingmaDir = (Join-Path $env:USERPROFILE ".lingma"),
    [string]$Output    = ".\auth\credentials.json",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

function Write-Step($msg) { Write-Host "[lingma2api/import] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[lingma2api/import] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[lingma2api/import] $msg" -ForegroundColor Yellow }
function Write-Err($msg)  { Write-Host "[lingma2api/import] $msg" -ForegroundColor Red }

# 1. 工具链 + 源目录检查
Write-Step "[1/3] 检查环境..."
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Err "未找到 go，请先安装"
    exit 1
}
if (-not (Test-Path $LingmaDir)) {
    Write-Err "Lingma 目录不存在: $LingmaDir"
    Write-Err "请确认本机已安装并登录过 Lingma 客户端"
    exit 1
}
$cacheDir = Join-Path $LingmaDir "cache"
if (-not (Test-Path $cacheDir)) {
    Write-Warn "未找到 $cacheDir，导入可能失败（请先在 Lingma 客户端完成一次登录）"
}
Write-Ok "  Lingma 目录: $LingmaDir"

# 2. 输出目标处理
$outputAbs = [System.IO.Path]::GetFullPath((Join-Path $Root $Output))
$outputDir = Split-Path -Parent $outputAbs
if (-not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
    Write-Ok "  创建输出目录: $outputDir"
}

if ((Test-Path $outputAbs) -and -not $Force) {
    Write-Warn "  目标文件已存在: $outputAbs"
    $reply = Read-Host "覆盖? [y/N]"
    if ($reply -notmatch '^[yY]') {
        Write-Warn "已取消（使用 -Force 跳过本提示）"
        exit 0
    }
}

# 3. 调用 lingma-import-cache
Write-Step "[2/3] 从 Lingma 缓存提取并派生凭据..."
& go run ./cmd/lingma-import-cache --lingma-dir "$LingmaDir" --output "$outputAbs"
if ($LASTEXITCODE -ne 0) {
    Write-Err "导入失败（exit $LASTEXITCODE）"
    exit $LASTEXITCODE
}

# 4. 校验
Write-Step "[3/3] 校验输出..."
if (-not (Test-Path $outputAbs)) {
    Write-Err "导入命令成功但目标文件未生成: $outputAbs"
    exit 1
}
$bytes = (Get-Item $outputAbs).Length
Write-Ok "  写入: $outputAbs ($bytes bytes)"

# 5. 提醒同步 config.yaml
$cfgPath = Join-Path $Root "config.yaml"
if (Test-Path $cfgPath) {
    $cfg = Get-Content $cfgPath -Raw
    if ($cfg -notmatch [regex]::Escape($outputAbs) -and $cfg -notmatch 'auth_file:\s*"\./auth/credentials\.json"') {
        Write-Warn "提示: config.yaml 的 credential.auth_file 可能未指向 $outputAbs"
        Write-Warn "  打开 config.yaml 检查 credential.auth_file"
    }
}

Write-Ok "完成喵～现在可以运行 .\start.ps1 启动服务"
