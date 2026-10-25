#requires -Version 5.1
<#
.SYNOPSIS
  Gubuy-ERP Windows 开发环境脚本（scripts/dev.sh 的 Windows 版）。

.DESCRIPTION
  管理本地开发栈的完整生命周期：
    - MySQL 运行在 Docker（停止脚本不会删除容器与数据卷）
    - Go API 在宿主机以 Air 热重载运行
    - Vue/Vite 开发服务器在宿主机运行

  用法（任选其一）：
    powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 <command>
    scripts\dev.cmd <command>

  生命周期命令：
    start       后台启动 MySQL、Go/Air、Vite（默认）
    status      查看开发服务和 MySQL 状态
    stop        停止 Go/Air 和 Vite，保留 MySQL 容器及数据卷
    restart     重启 Go/Air 和 Vite
    foreground  在当前终端前台运行，Ctrl+C 停止前后端
    logs        持续查看运行日志（Ctrl+C 退出）
#>
[CmdletBinding()]
param(
  [Parameter(Position = 0)][string]$Command = "start",
  [switch]$RunMode
)

# ---------- 环境变量规范化 ----------
# 沙箱/某些代理工具可能同时注入 HTTPS_PROXY 与 https_proxy。.NET 的 Start-Process
# 用大小写不敏感的环境字典启动子进程，遇到同名不同大小写的变量会抛出
# “已添加了具有相同键的项”。这里把代理变量统一为单一大写键。
function Repair-ProxyEnvDup {
  foreach ($name in @('HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY', 'ALL_PROXY', 'FTP_PROXY')) {
    $val = [Environment]::GetEnvironmentVariable($name)  # 大小写不敏感读取
    if (-not $val) { continue }
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
    [Environment]::SetEnvironmentVariable($name.ToLower(), $null, 'Process')
    [Environment]::SetEnvironmentVariable($name, $val, 'Process')
  }
}
Repair-ProxyEnvDup

# 用 Continue 而非 Stop：Stop 模式下原生命令写到 stderr 的正常状态行（如 compose 的
# “Container xxx Running”）在重定向下会被当成 terminating error。原生命令一律用
# $LASTEXITCODE 显式判断成败。
$ErrorActionPreference = "Continue"

# ---------- 路径常量 ----------
$RepoRoot   = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BackendDir = Join-Path $RepoRoot "erp-backend"
$StateDir   = Join-Path $RepoRoot "logs\dev-stack"
$PidFile    = Join-Path $StateDir "runner.pid"
$LogFile    = Join-Path $StateDir "dev.log"
$AirOutLog  = Join-Path $StateDir "air.out.log"
$AirErrLog  = Join-Path $StateDir "air.err.log"
$ViteOutLog = Join-Path $StateDir "vite.out.log"
$ViteErrLog = Join-Path $StateDir "vite.err.log"
$AgentOutLog = Join-Path $StateDir "agent.out.log"
$AgentErrLog = Join-Path $StateDir "agent.err.log"
$script:LockFile = Join-Path $StateDir "control.lock"
$script:LockStream = $null

# ---------- 输出辅助 ----------
function Write-Step { param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message" -ForegroundColor Cyan
}
function Write-Info { param([string]$Message)
  Write-Host $Message
}
function Write-Fail { param([string]$Message)
  Write-Host ""
  Write-Host "错误：$Message" -ForegroundColor Red
  exit 1
}
function Write-TailLog { param([int]$Lines = 60)
  if (Test-Path $LogFile) {
    Get-Content $LogFile -Tail $Lines | ForEach-Object { Write-Host $_ }
  }
}

# ---------- .env 加载 ----------
$script:DotEnv = @{}
function Import-DotEnv {
  $envFile = Join-Path $RepoRoot ".env"
  if (-not (Test-Path $envFile)) { return }
  foreach ($line in Get-Content $envFile) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0 -or $trimmed.StartsWith("#")) { continue }
    $idx = $trimmed.IndexOf("=")
    if ($idx -lt 1) { continue }
    $key = $trimmed.Substring(0, $idx).Trim()
    $val = $trimmed.Substring($idx + 1).Trim()
    if ($val.Length -ge 2 -and (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'")))) {
      $val = $val.Substring(1, $val.Length - 2)
    }
    $script:DotEnv[$key] = $val
    # 导出到当前进程环境，子进程（Go/Vite/Compose）继承
    Set-Item -Path "env:$key" -Value $val
  }
}
function Get-DotEnv { param([string]$Key, [string]$Default = "")
  if ($script:DotEnv.ContainsKey($Key) -and $script:DotEnv[$Key] -ne "") { return $script:DotEnv[$Key] }
  $existing = [Environment]::GetEnvironmentVariable($Key)
  if ($existing) { return $existing }
  return $Default
}

# ---------- 工具定位 ----------
function Get-GoExe {
  if ($script:GoExe) { return $script:GoExe }
  $candidates = @()
  if ($env:DEV_GO_EXE) { $candidates += $env:DEV_GO_EXE }
  $candidates += "$env:ProgramFiles\Go\bin\go.exe"
  $candidates += Get-ChildItem "$env:USERPROFILE\.workbuddy\binaries\go\versions\*\go\bin\go.exe" -ErrorAction SilentlyContinue |
    Sort-Object FullName -Descending | ForEach-Object { $_.FullName }
  $candidates += "$env:USERPROFILE\scoop\apps\go\current\bin\go.exe"
  $candidates += "$env:LOCALAPPDATA\Programs\Go\bin\go.exe"
  foreach ($c in $candidates) {
    if ($c -and (Test-Path $c)) { $script:GoExe = $c; return $c }
  }
  return $null
}
function Get-NodeExe {
  if ($script:NodeExe) { return $script:NodeExe }
  $candidates = @()
  if ($env:DEV_NODE_EXE) { $candidates += $env:DEV_NODE_EXE }
  $candidates += "$env:ProgramFiles\nodejs\node.exe"
  $candidates += Get-ChildItem "$env:USERPROFILE\.workbuddy\binaries\node\versions\*\node.exe" -ErrorAction SilentlyContinue |
    Sort-Object FullName -Descending | ForEach-Object { $_.FullName }
  $candidates += "$env:LOCALAPPDATA\hermes\node\node.exe"
  foreach ($c in $candidates) {
    if ($c -and (Test-Path $c)) { $script:NodeExe = $c; return $c }
  }
  return $null
}

# ---------- 进程/PID 管理 ----------
function Test-RunnerAlive { param([int]$ProcId)
  if ($ProcId -le 0) { return $false }
  $proc = Get-Process -Id $ProcId -ErrorAction SilentlyContinue
  if (-not $proc) { return $false }
  # 运行器是通过 powershell -EncodedCommand 启动的 dev.ps1 实例
  return ($proc.ProcessName -match "^powershell$|^pwsh$")
}
function Read-RunnerPid {
  if (-not (Test-Path $PidFile)) { return 0 }
  $raw = (Get-Content $PidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
  $parsed = 0
  if ([int]::TryParse($raw, [ref]$parsed)) { return $parsed }
  return 0
}
function Write-RunnerPid {
  New-Item -ItemType Directory -Force -Path $StateDir | Out-Null
  $tmp = "$PidFile.$PID.tmp"
  Set-Content -Path $tmp -Value $PID
  Move-Item -Path $tmp -Destination $PidFile -Force
}
function Remove-RunnerPidIfOwned {
  if (-not (Test-Path $PidFile)) { return }
  $raw = (Get-Content $PidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
  if ("$raw" -eq "$PID") { Remove-Item $PidFile -Force -ErrorAction SilentlyContinue }
}
function Stop-ProcessTree { param([int]$ProcId)
  if ($ProcId -le 0) { return }
  if (-not (Get-Process -Id $ProcId -ErrorAction SilentlyContinue)) { return }
  & taskkill /PID $ProcId /T /F | Out-Null
}

# ---------- 控制锁（文件独占，进程死亡自动释放） ----------
function Enter-ControlLock {
  New-Item -ItemType Directory -Force -Path $StateDir | Out-Null
  $attempt = 0
  while ($true) {
    try {
      # 仅独占持有句柄即视为加锁；进程退出/被杀时操作系统自动释放，无需清理陈旧锁
      $script:LockStream = [System.IO.File]::Open($script:LockFile, 'OpenOrCreate', 'ReadWrite', 'None')
      return
    } catch {
      $attempt++
      if ($attempt -gt 100) {
        Write-Fail "另一个 dev.ps1 生命周期命令仍在执行；如确认没有命令运行，请删除 $($script:LockFile) 后重试。"
      }
      Start-Sleep -Milliseconds 100
    }
  }
}
function Exit-ControlLock {
  # 只释放句柄，不删除锁文件：删除可能与刚拿到锁的下一个进程产生竞态。
  # 锁的语义由“独占句柄”决定，残留的锁文件不会阻塞后续加锁。
  if ($script:LockStream) {
    try { $script:LockStream.Close() } catch {}
    $script:LockStream = $null
  }
}

# ---------- HTTP 探测 ----------
function Test-Url { param([string]$Url)
  & curl.exe --noproxy '*' -s -f -o NUL --connect-timeout 1 --max-time 2 $Url 2>$null | Out-Null
  return ($LASTEXITCODE -eq 0)
}

# ---------- 端口检查 ----------
function Test-PortBusy { param([int]$Port)
  try {
    $conns = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    if ($conns -and @($conns).Count -gt 0) { return $true }
  } catch {
    $hit = netstat -ano | Select-String "[:.]$Port\s.*LISTENING"
    if ($hit) { return $true }
  }
  return $false
}
function Assert-PortFree { param([int]$Port, [string]$Service)
  if ($Port -lt 1 -or $Port -gt 65535) {
    Write-Fail "$Service 端口配置无效：$Port。端口范围必须是 1-65535。"
  }
  if (Test-PortBusy $Port) {
    Write-Host ""
    Write-Host "端口 $Port（$Service）已被占用：" -ForegroundColor Yellow
    netstat -ano | Select-String "[:.]$Port\s.*LISTENING" | ForEach-Object { Write-Host $_.Line }
    Write-Fail "请停止占用该端口的进程，或在 .env 中修改 DEV_API_PORT / DEV_VITE_PORT 后重试。"
  }
}

# ---------- Docker 引擎 ----------
function Get-DockerDesktopExe {
  $candidates = @(
    "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe",
    "$env:LOCALAPPDATA\Docker\Docker Desktop.exe"
  )
  foreach ($c in $candidates) { if (Test-Path $c) { return $c } }
  return $null
}
function Test-DockerEngine {
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { return $false }
  & docker info *> $null
  return ($LASTEXITCODE -eq 0)
}
function Ensure-DockerEngine {
  if (Test-DockerEngine) {
    Write-Step "Docker 引擎已就绪"
    return
  }
  $exe = Get-DockerDesktopExe
  if (-not $exe) {
    Write-Fail "Docker CLI 已安装，但 Docker 引擎不可用，且未找到 Docker Desktop。请先启动 Docker Desktop 或 Docker 服务，再重试。"
  }
  Write-Step "Docker 引擎未启动，正在打开 Docker Desktop"
  Start-Process -FilePath $exe | Out-Null

  $timeout = [int](Get-DotEnv "DEV_DOCKER_STARTUP_TIMEOUT" "180")
  $elapsed = 0
  while ($elapsed -lt $timeout) {
    Start-Sleep -Seconds 2
    $elapsed += 2
    if (Test-DockerEngine) {
      Write-Step "Docker 引擎已就绪（等待 $elapsed 秒）"
      return
    }
    if ($elapsed % 20 -eq 0) {
      Write-Info "等待 Docker Desktop 启动（$elapsed/$timeout 秒）"
    }
  }
  Write-Fail "等待 Docker Desktop $timeout 秒后仍无法连接。请检查 Docker Desktop 状态，或设置 DEV_DOCKER_STARTUP_TIMEOUT 后重试。"
}

# ---------- 运行栈（run / foreground 共用） ----------
function Start-DevStack {
  # 前置工具检查
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Write-Fail "未找到 Docker CLI" }
  $goExe = Get-GoExe
  if (-not $goExe) { Write-Fail "未找到 Go。请安装 Go 1.26，或通过 DEV_GO_EXE 指定 go.exe 完整路径。" }
  $nodeExe = Get-NodeExe
  if (-not $nodeExe) { Write-Fail "未找到 Node.js。请安装 Node 22，或通过 DEV_NODE_EXE 指定 node.exe 完整路径。" }
  $curlExe = "$env:SystemRoot\System32\curl.exe"
  if (-not (Test-Path $curlExe)) { Write-Fail "未找到 curl.exe，无法检查本地服务是否启动成功" }

  if (-not (Test-Path (Join-Path $RepoRoot ".env"))) {
    Write-Fail "未找到项目根目录 .env。请先复制 .env.example 为 .env 并设置至少 32 位的 JWT_SECRET。"
  }
  if (-not (Test-Path (Join-Path $RepoRoot "node_modules\vite\bin\vite.js"))) {
    Write-Fail "前端依赖尚未安装，请先在项目根目录运行 npm install。"
  }
  if (-not (Test-Path (Join-Path $BackendDir ".air.windows.toml"))) {
    Write-Fail "未找到 $BackendDir\.air.windows.toml，无法启动 Go 热重载。"
  }

  # 导出 .env 并读取端口配置
  Import-DotEnv
  $devApiPort   = [int](Get-DotEnv "DEV_API_PORT" "8081")
  $devVitePort  = [int](Get-DotEnv "DEV_VITE_PORT" "5173")
  $mysqlPort    = [int](Get-DotEnv "MYSQL_HOST_PORT" "3307")
  $jwtSecret    = (Get-DotEnv "JWT_SECRET" "")

  if ($jwtSecret.Length -lt 32) { Write-Fail ".env 中的 JWT_SECRET 不能为空且至少需要 32 字节。" }
  if ($devApiPort -eq $devVitePort) { Write-Fail "DEV_API_PORT 和 DEV_VITE_PORT 不能使用同一个端口。" }

  # 端口检查放在 Docker 启动之前，避免端口冲突留下半启动的栈
  Assert-PortFree $devApiPort "Go API"
  Assert-PortFree $devVitePort "Vite"

  Ensure-DockerEngine

  # 导出后端/前端所需环境变量（子进程继承）
  $env:DB_HOST = "127.0.0.1"
  $env:DB_PORT = "$mysqlPort"
  $env:DB_USER = (Get-DotEnv "DB_USER" "root")
  $env:DB_PASS = (Get-DotEnv "DB_PASS" (Get-DotEnv "MYSQL_ROOT_PASSWORD" "rootpassword"))
  $env:DB_NAME = (Get-DotEnv "DB_NAME" "erp_db")
  $env:BACKEND_LISTEN_ADDR = ":$devApiPort"
  $env:VITE_API_PROXY_TARGET = "http://127.0.0.1:$devApiPort"
  $env:VITE_API_BASE_URL = ""

  # 把 Go 与 GOPATH\bin 加入 PATH（供 go generate / air 使用）
  $goBinDir = Split-Path -Parent $goExe
  $gopathBin = (& $goExe env GOPATH)
  if ($gopathBin) { $gopathBin = Join-Path $gopathBin "bin" }
  $env:PATH = "$goBinDir;$gopathBin;$env:PATH"

  Push-Location $RepoRoot
  try {
    & docker compose config --quiet
    if ($LASTEXITCODE -ne 0) { Write-Fail "Docker Compose 配置校验失败，请检查 .env。" }

    # 停止本地打包版 app，避免与开发 API 同时执行后台任务
    $runningServices = & docker compose ps --status running --services 2>$null
    if ($runningServices -contains "app") {
      Write-Step "停止本地打包版 app，避免与开发 API 同时执行后台任务"
      & docker compose stop app | Out-Null
    }

    Write-Step "启动并等待本地 MySQL（端口 $mysqlPort）"
    & docker compose up -d --wait db
    if ($LASTEXITCODE -ne 0) { Write-Fail "MySQL 容器启动失败，请执行 docker compose logs db 查看。" }
    Write-Info "本机开发数据源：127.0.0.1:$mysqlPort/$($env:DB_NAME)"
  } finally {
    Pop-Location
  }

  # SQL 客户端（系统设置页备份/恢复用）——Windows 上仅提示，不阻断
  $hasClient = (Get-Command mariadb-dump -ErrorAction SilentlyContinue) -or (Get-Command mysqldump -ErrorAction SilentlyContinue)
  if (-not $hasClient) {
    Write-Host "提示：未检测到 mariadb-dump/mysqldump，系统设置页的 SQL 备份/恢复将不可用（不影响开发）。" -ForegroundColor Yellow
  }

  # Air 热重载工具
  $airExe = Join-Path $gopathBin "air.exe"
  if (-not (Test-Path $airExe)) {
    Write-Step "首次启动需要安装 Go 热重载工具 Air"
    & $goExe install github.com/air-verse/air@v1.67.4
    if ($LASTEXITCODE -ne 0) { Write-Fail "Air 安装失败，请检查 Go 环境与网络后重试。" }
  }
  if (-not (Test-Path $airExe)) { Write-Fail "Air 安装后未找到：$airExe" }

  # Swagger docs
  if (-not (Test-Path (Join-Path $BackendDir "docs\docs.go"))) {
    Write-Step "首次启动生成 Swagger 文档"
    Push-Location $BackendDir
    try { & $goExe generate . } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { Write-Fail "Swagger 文档生成失败（go generate .）。" }
  }

  Write-Step "启动 Go API 热重载（http://127.0.0.1:$devApiPort）"
  $startArgs = @{ FilePath = $airExe; ArgumentList = "-c", ".air.windows.toml"; WorkingDirectory = $BackendDir; PassThru = $true }
  if ($RunMode) {
    $script:AirProc = Start-Process @startArgs -WindowStyle Hidden -RedirectStandardOutput $AirOutLog -RedirectStandardError $AirErrLog
  } else {
    $script:AirProc = Start-Process @startArgs -NoNewWindow
  }
  if (-not $script:AirProc) { Write-Fail "Air 进程启动失败，请查看日志：$AirErrLog" }

  Write-Step "启动 Vue/Vite（http://localhost:$devVitePort）"
  $viteEntry = Join-Path $RepoRoot "node_modules\vite\bin\vite.js"
  $startArgs = @{ FilePath = $nodeExe; ArgumentList = "`"$viteEntry`"", "--port", "$devVitePort", "--strictPort"; WorkingDirectory = $RepoRoot; PassThru = $true }
  if ($RunMode) {
    $script:ViteProc = Start-Process @startArgs -WindowStyle Hidden -RedirectStandardOutput $ViteOutLog -RedirectStandardError $ViteErrLog
  } else {
    $script:ViteProc = Start-Process @startArgs -NoNewWindow
  }
  if (-not $script:ViteProc) { Write-Fail "Vite 进程启动失败，请查看日志：$ViteErrLog" }

  # pi 核心 Agent 服务（agent-service，默认 8082）
  $devAgentPort = [int](Get-DotEnv "DEV_AGENT_PORT" "8082")
  $env:AGENT_SERVICE_PORT = "$devAgentPort"
  $env:VITE_AGENT_PROXY_TARGET = "http://127.0.0.1:$devAgentPort"
  # 本机开发 Go 监听 8081（BACKEND_LISTEN_ADDR），显式告知 agent-service 回调地址
  $env:ERP_API_URL = "http://127.0.0.1:8081"
  $AgentDir = Join-Path $RepoRoot "agent-service"
  if (-not (Test-Path (Join-Path $AgentDir "node_modules"))) {
    Write-Step "首次启动需要安装 agent-service 依赖"
    Push-Location $AgentDir
    try { & $nodeExe (Join-Path $RepoRoot "node_modules\npm\bin\npm-cli.js") install --no-audit --no-fund | Out-Null } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { Write-Fail "agent-service 依赖安装失败，请检查网络后重试。" }
  }
  Write-Step "启动 Agent 服务（pi 核心，http://127.0.0.1:$devAgentPort）"
  $agentEntry = Join-Path $AgentDir "node_modules\tsx\dist\cli.mjs"
  $startArgs = @{ FilePath = $nodeExe; ArgumentList = "`"$agentEntry`"", "src/server.ts"; WorkingDirectory = $AgentDir; PassThru = $true }
  if ($RunMode) {
    $script:AgentProc = Start-Process @startArgs -WindowStyle Hidden -RedirectStandardOutput $AgentOutLog -RedirectStandardError $AgentErrLog
  } else {
    $script:AgentProc = Start-Process @startArgs -NoNewWindow
  }
  if (-not $script:AgentProc) { Write-Fail "Agent 服务进程启动失败，请查看日志：$AgentErrLog" }

  # 就绪探测
  Wait-ForHttp -Service "Go API" -Url "http://127.0.0.1:$devApiPort/api/admin/status" -Proc $script:AirProc
  Wait-ForHttp -Service "Vite" -Url "http://localhost:$devVitePort/" -Proc $script:ViteProc
  Wait-ForHttp -Service "Agent 服务" -Url "http://127.0.0.1:$devAgentPort/api/ai/agent/brief" -Proc $script:AgentProc
  if (-not (Test-Url "http://localhost:$devVitePort/api/admin/status")) {
    Write-Fail "Vite 已启动，但 API 代理请求失败，请检查 vite.config.js 中的代理配置。"
  }

  Write-Step "开发环境已启动；修改 Vue 会即时热更新，修改 Go 会自动重编译并重启。"
  if ((Get-DotEnv "DEV_OPEN_BROWSER" "1") -eq "1") {
    Open-DevPages -DevVitePort $devVitePort
  }
}

function Wait-ForHttp { param([string]$Service, [string]$Url, [System.Diagnostics.Process]$Proc)
  $timeout = [int](Get-DotEnv "DEV_STARTUP_TIMEOUT" "180")
  $elapsed = 0
  Write-Step "等待 $Service 就绪"
  while (-not (Test-Url $Url)) {
    if ($Proc.HasExited) {
      Write-Fail "$Service 异常退出（状态码 $($Proc.ExitCode)）。"
    }
    if ($elapsed -ge $timeout) {
      Write-Fail "$Service 在 $timeout 秒内未就绪（$Url）。可设置 DEV_STARTUP_TIMEOUT 延长等待时间。"
    }
    Start-Sleep -Seconds 1
    $elapsed++
  }
}

function Open-DevPages { param([int]$DevVitePort)
  $adminUrl = "http://localhost:$DevVitePort/admin"
  $staffUrl = "http://localhost:$DevVitePort/terminal"
  try {
    Start-Process $adminUrl | Out-Null
    Start-Process $staffUrl | Out-Null
    Write-Step "已在默认浏览器打开管理员端和员工端"
  } catch {
    Write-Host "提示：请在浏览器中打开：`n  管理员端：$adminUrl`n  员工端：$staffUrl" -ForegroundColor Yellow
  }
}

# ---------- run / foreground 模式：启动并守护子进程 ----------
function Invoke-Runner {
  New-Item -ItemType Directory -Force -Path $StateDir | Out-Null
  if (-not $RunMode) {
    # foreground：确保没有后台运行器
    Enter-ControlLock
    $existing = Read-RunnerPid
    if (Test-RunnerAlive $existing) {
      Exit-ControlLock
      Write-Fail "开发服务已由 PID $existing 管理；请先运行 .\scripts\dev.ps1 stop。"
    }
    Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
    Exit-ControlLock
  }
  Write-RunnerPid

  $script:AirProc = $null
  $script:ViteProc = $null
  $script:AgentProc = $null
  $exitCode = 0
  try {
    Start-DevStack
    if ($RunMode) {
      Write-Info "后台运行器已就绪（PID $PID）；使用 .\scripts\dev.ps1 stop 停止 Go/Air 与 Vite，MySQL 容器和数据卷会保留。"
    } else {
      Write-Info "按 Ctrl+C 停止前后端开发进程。MySQL 容器和数据卷会保留。"
    }

    # 守护循环：任一子进程退出则终止整个栈
    while ($true) {
      if ($script:AirProc.HasExited) {
        throw "Air 已退出（状态码 $($script:AirProc.ExitCode)）。"
      }
      if ($script:ViteProc.HasExited) {
        throw "Vite 已退出（状态码 $($script:ViteProc.ExitCode)）。"
      }
      if ($script:AgentProc -and $script:AgentProc.HasExited) {
        throw "Agent 服务已退出（状态码 $($script:AgentProc.ExitCode)）。"
      }
      Start-Sleep -Seconds 1
    }
  } catch {
    $msg = $_.Exception.Message
    Write-Host ""
    Write-Host "错误：$msg" -ForegroundColor Red
    if (-not $RunMode) {
      # 后台模式下输出已重定向到 dev.log 本身，不能再 tail 进去（会自嵌套膨胀）
      if (Test-Path $LogFile) {
        Write-Host "最近日志（$LogFile）：" -ForegroundColor Yellow
        Write-TailLog 40
      }
    }
    $exitCode = 1
  } finally {
    foreach ($p in @($script:ViteProc, $script:AirProc, $script:AgentProc)) {
      if ($p -and -not $p.HasExited) { Stop-ProcessTree $p.Id }
    }
    Remove-RunnerPidIfOwned
  }
  exit $exitCode
}

# ---------- start ----------
function Start-Stack {
  Import-DotEnv
  $devApiPort  = [int](Get-DotEnv "DEV_API_PORT" "8081")
  $devVitePort = [int](Get-DotEnv "DEV_VITE_PORT" "5173")

  Enter-ControlLock
  $runnerPid = Read-RunnerPid
  if (Test-RunnerAlive $runnerPid) {
    Exit-ControlLock
    Write-Info "开发进程已在运行（PID $runnerPid），检查服务状态……"
    $ok = Wait-ManagedStart -RunnerPid $runnerPid -DevApiPort $devApiPort -DevVitePort $devVitePort
    exit ($(if ($ok) { 0 } else { 1 }))
  }
  Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Force -Path $StateDir | Out-Null

  Write-Step "后台启动开发环境"
  # 通过 EncodedCommand 启动隐藏运行器，其输出全部追加到 dev.log
  $inner = "& '$PSCommandPath' -RunMode *>> '$LogFile'"
  $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($inner))
  $proc = Start-Process -FilePath "powershell.exe" `
    -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", $encoded `
    -WindowStyle Hidden -PassThru

  # 等待运行器写入 PID 文件
  $attempts = 0
  while ($attempts -lt 50) {
    $written = Read-RunnerPid
    if ($written -gt 0 -and (Test-RunnerAlive $written)) { break }
    if ($proc.HasExited) { break }
    Start-Sleep -Milliseconds 100
    $attempts++
  }
  Exit-ControlLock

  $runnerPid = Read-RunnerPid
  if (-not (Test-RunnerAlive $runnerPid)) {
    Write-Host "开发环境未能启动，最近日志如下：" -ForegroundColor Red
    Write-TailLog 60
    exit 1
  }
  $ok = Wait-ManagedStart -RunnerPid $runnerPid -DevApiPort $devApiPort -DevVitePort $devVitePort
  exit ($(if ($ok) { 0 } else { 1 }))
}

function Wait-ManagedStart { param([int]$RunnerPid, [int]$DevApiPort, [int]$DevVitePort)
  $timeout = [int](Get-DotEnv "DEV_TOTAL_STARTUP_TIMEOUT" "600")
  $elapsed = 0
  $apiUrl = "http://127.0.0.1:$DevApiPort/api/admin/status"
  $proxyUrl = "http://localhost:$DevVitePort/api/admin/status"
  while ($elapsed -le $timeout) {
    if ((Test-Url $apiUrl) -and (Test-Url $proxyUrl)) {
      Write-Step "开发环境已就绪："
      Write-Info "  管理员端：http://localhost:$DevVitePort/admin"
      Write-Info "  员工端：http://localhost:$DevVitePort/terminal"
      Write-Info "  Go API：$apiUrl"
      Write-Info "日志：$LogFile"
      return $true
    }
    if (-not (Test-RunnerAlive $RunnerPid)) {
      Write-Host "开发服务启动失败，最近日志如下：" -ForegroundColor Red
      Write-TailLog 60
      return $false
    }
    if ($elapsed -gt 0 -and $elapsed % 10 -eq 0) {
      Write-Info "开发环境仍在启动（$elapsed 秒），进度日志：$LogFile"
    }
    Start-Sleep -Seconds 1
    $elapsed++
  }
  Write-Host "开发服务在 $timeout 秒内未就绪，最近日志如下：" -ForegroundColor Red
  Write-TailLog 60
  Stop-Stack | Out-Null
  return $false
}

# ---------- status ----------
function Show-Status {
  Import-DotEnv
  $devApiPort  = [int](Get-DotEnv "DEV_API_PORT" "8081")
  $devVitePort = [int](Get-DotEnv "DEV_VITE_PORT" "5173")
  $mysqlPort   = [int](Get-DotEnv "MYSQL_HOST_PORT" "3307")
  $apiUrl   = "http://127.0.0.1:$devApiPort/api/admin/status"
  $viteUrl  = "http://localhost:$devVitePort/"
  $proxyUrl = "http://localhost:$devVitePort/api/admin/status"
  $notReady = $false

  if ((Get-Command docker -ErrorAction SilentlyContinue) -and (Test-DockerEngine)) {
    Write-Info "Docker 引擎：就绪"
  } else {
    Write-Info "Docker 引擎：未就绪（运行 start 会自动打开 Docker Desktop）"
    $notReady = $true
  }

  $runnerPid = Read-RunnerPid
  if (Test-RunnerAlive $runnerPid) {
    Write-Info "开发进程：运行中（PID $runnerPid）"
  } else {
    if ($runnerPid -gt 0) {
      Write-Info "开发进程：未运行（清理过期 PID 记录）"
      Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
    } else {
      Write-Info "开发进程：未运行"
    }
    $notReady = $true
  }

  if (Test-Url $apiUrl)   { Write-Info "Go API：就绪 $apiUrl" }   else { Write-Info "Go API：未就绪 $apiUrl";   $notReady = $true }
  if (Test-Url $viteUrl)  { Write-Info "Vite：就绪 $viteUrl" }    else { Write-Info "Vite：未就绪 $viteUrl";    $notReady = $true }
  if (Test-Url $proxyUrl) { Write-Info "API 代理：就绪 $proxyUrl" } else { Write-Info "API 代理：未就绪 $proxyUrl"; $notReady = $true }

  $dbRunning = $false
  if ((Get-Command docker -ErrorAction SilentlyContinue)) {
    $services = & docker compose ps --status running --services 2>$null
    $dbRunning = ($services -contains "db")
  }
  if ($dbRunning) {
    Write-Info "MySQL：运行中（127.0.0.1:$mysqlPort）"
  } else {
    Write-Info "MySQL：未运行（预期端口 127.0.0.1:$mysqlPort）"
    $notReady = $true
  }
  Write-Info "日志：$LogFile"
  exit ($(if ($notReady) { 1 } else { 0 }))
}

# ---------- stop ----------
function Stop-Stack {
  Enter-ControlLock
  try {
    $runnerPid = Read-RunnerPid
    if ($runnerPid -le 0) {
      Write-Info "开发服务未运行；MySQL 容器及数据卷未更改。"
      return $true
    }
    if (-not (Test-RunnerAlive $runnerPid)) {
      Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
      Write-Info "已清理过期 PID 记录；未找到受管理的开发服务。"
      return $true
    }
    Write-Step "停止开发服务（PID $runnerPid）"
    Stop-ProcessTree $runnerPid
    $attempt = 0
    while ((Test-RunnerAlive $runnerPid) -and $attempt -lt 50) {
      Start-Sleep -Milliseconds 100
      $attempt++
    }
    if (Test-RunnerAlive $runnerPid) {
      Write-Host "开发进程未能停止，请查看日志：$LogFile" -ForegroundColor Red
      return $false
    }
    Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
    Write-Info "Go/Air 与 Vite 已停止；MySQL 容器及数据卷保留。"
    return $true
  } finally {
    Exit-ControlLock
  }
}

# ---------- 命令分发 ----------
switch ($Command.ToLower()) {
  "start"      { if ($RunMode) { Invoke-Runner } else { Start-Stack } }
  "status"     { Show-Status }
  "stop"       { exit ($(if (Stop-Stack) { 0 } else { 1 })) }
  "restart"    { Stop-Stack | Out-Null; Start-Stack }
  "foreground" { Invoke-Runner }
  "logs" {
    if (Test-Path $LogFile) { Get-Content $LogFile -Tail 80 -Wait } else { Write-Fail "暂无日志：$LogFile" }
  }
  "-h" { & $PSScriptRoot\dev.ps1 help }
  "--help" { & $PSScriptRoot\dev.ps1 help }
  "help" {
    Write-Host @"
用法：.\scripts\dev.ps1 [start|status|stop|restart|foreground|logs]
      或：scripts\dev.cmd [command]

  start       在后台启动 MySQL、Go/Air 和 Vite（默认）
  status      查看开发服务和 MySQL 状态
  stop        停止 Go/Air 和 Vite，保留 MySQL 容器及数据卷
  restart     重启 Go/Air 和 Vite
  foreground  在当前终端前台运行；按 Ctrl+C 停止前后端
  logs        持续查看运行日志（Ctrl+C 退出）

start 会在需要时自动打开 Docker Desktop 并等待引擎就绪。
日志：logs\dev-stack\dev.log（另有 air/vite 独立输出日志）
"@
  }
  default {
    & $PSScriptRoot\dev.ps1 help
    Write-Host ""
    Write-Host "错误：未知命令：$Command" -ForegroundColor Red
    exit 1
  }
}
