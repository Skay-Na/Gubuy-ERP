#requires -Version 5.1
<#
.SYNOPSIS
  Gubuy-ERP 一键构建并更新软路由（scripts/build-deploy-router.sh 的 Windows 版）。

.DESCRIPTION
  构建前端/后端 -> docker compose build app -> docker save 导出镜像包 ->
  备份软路由旧镜像包 -> 上传新镜像包 -> 远程 load/重启容器并校验。
  整个流程不会启动 Windows 本机的 app 容器。

  用法（任选其一，在仓库任意位置执行）：
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-deploy-router.ps1
    scripts\build-deploy-router.cmd

  与 macOS 版的差异（Windows 没有 expect / security 钥匙串 / BSD nc）：
    - 免交互 SSH 由 PuTTY 的 plink / pscp 承担（-pw 自动填密码 + -hostkey 预置主机指纹）。
      找不到 PuTTY 时降级为 Windows/Git 自带的 OpenSSH ssh / scp，密码需在终端手动输入。
    - 软路由 SSH 密码存于 scripts\.router-pass.clixml（DPAPI 加密，仅当前用户当前机器可解密）。
    - 端口探测改用 .NET TcpClient，不再依赖 nc。
    - macOS 版的 EXPECT_TIMEOUT 无对应项（plink / ssh 都没有交互超时参数），
      连接挂起由 SSH_CONNECT_TIMEOUT 的前置端口探测兜底。

.PARAMETER RouterPass
  直接提供软路由 SSH 密码（优先级最高，不落盘）。也可用环境变量 ROUTER_PASS。

.PARAMETER ResetPassword
  忽略已保存的密码，重新输入并覆盖 scripts\.router-pass.clixml。

.PARAMETER SshMode
  Auto（默认，优先 PuTTY）/ Putty / OpenSsh。

.PARAMETER RunTests
  1 时执行后端审计测试（需指定非生产库 AUDIT_DB_NAME）。
#>
[CmdletBinding()]
param(
  [string]$RouterLanIp,
  [string]$RouterTailscaleIp,
  [string]$RouterUser,
  [string]$RouterPass,
  [string]$TarPath,
  [string]$LocalBackupDir,
  [string]$RunTests,
  [string]$AuditDbName,
  [string]$PruneDockerCache,
  [string]$RemotePruneDockerImages,
  [string]$BuildCacheMaxAge,
  [string]$SshConnectTimeout,
  [string]$SshMode,
  [switch]$ResetPassword,
  [switch]$SavePassword
)

# 用 Continue 而非 Stop：Stop 模式下原生命令写到 stderr 的正常状态行（如 compose 的
# “Container xxx Running”）在重定向下会被当成 terminating error。原生命令一律用
# $LASTEXITCODE 显式判断成败。
$ErrorActionPreference = "Continue"

try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch { }

# ---------- 路径常量 ----------
$RepoRoot   = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BackendDir = Join-Path $RepoRoot "erp-backend"
$PassFile   = Join-Path $PSScriptRoot ".router-pass.clixml"

# ---------- 输出辅助 ----------
function Write-Step { param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message" -ForegroundColor Cyan
}
function Write-Info { param([string]$Message)
  Write-Host $Message
}
function Write-WarnMsg { param([string]$Message)
  Write-Host ""
  Write-Host "WARN: $Message" -ForegroundColor Yellow
}
function Write-Fail { param([string]$Message)
  Write-Host ""
  Write-Host "ERROR: $Message" -ForegroundColor Red
  exit 1
}

# ---------- 配置：命令行参数 > 环境变量 > 默认值 ----------
function Get-Conf { param([string]$Name, [string]$Default)
  $v = [Environment]::GetEnvironmentVariable($Name)
  if ($v -and $v.Trim() -ne "") { return $v.Trim() }
  return $Default
}
function Set-ConfFromEnv { param([string]$VarName, [string]$EnvName, [string]$Default)
  # 用 Get-Variable/Set-Variable 在脚本作用域里做“参数未提供则回落到环境变量”
  $cur = Get-Variable -Name $VarName -Scope Script -ValueOnly
  if ($cur -and "$cur".Trim() -ne "") { return }
  Set-Variable -Name $VarName -Scope Script -Value (Get-Conf $EnvName $Default)
}
Set-ConfFromEnv "RouterLanIp"             "ROUTER_LAN_IP"             "192.168.5.22"
Set-ConfFromEnv "RouterTailscaleIp"       "ROUTER_TAILSCALE_IP"       "100.83.204.35"
Set-ConfFromEnv "RouterUser"              "ROUTER_USER"               "root"
Set-ConfFromEnv "TarPath"                 "TAR_PATH"                  ""
Set-ConfFromEnv "LocalBackupDir"          "LOCAL_BACKUP_DIR"          (Join-Path $env:USERPROFILE "Desktop")
Set-ConfFromEnv "RunTests"                "RUN_TESTS"                 "0"
Set-ConfFromEnv "AuditDbName"             "AUDIT_DB_NAME"             "erp_audit_test"
Set-ConfFromEnv "PruneDockerCache"        "PRUNE_DOCKER_CACHE"        "1"
Set-ConfFromEnv "RemotePruneDockerImages" "REMOTE_PRUNE_DOCKER_IMAGES" "0"
Set-ConfFromEnv "BuildCacheMaxAge"        "BUILD_CACHE_MAX_AGE"       "168h"
Set-ConfFromEnv "SshConnectTimeout"       "SSH_CONNECT_TIMEOUT"       "3"
Set-ConfFromEnv "SshMode"                 "SSH_MODE"                  "Auto"
# 密码不参与 Trim：首尾空格可能是密码的一部分
if (-not $RouterPass) {
  $envPass = [Environment]::GetEnvironmentVariable("ROUTER_PASS")
  if ($envPass) { $RouterPass = $envPass }
}

$AppImage    = "erpwonchon-app:latest"
$AppPlatform = "linux/arm64"   # 软路由是 ARM64；推错架构会导致容器 exec format error 崩溃循环
$RemoteDir   = "/mnt/opt/docker/install"
$RemoteTar   = "erpwonchon-app.tar"
$script:PlinkExe   = $null
$script:PscpExe    = $null
$script:SshExe     = $null
$script:ScpExe     = $null
$script:RouterIp   = $null
$script:RouterPassPlain = $null
$script:HostKeys   = @()

# ---------- 工具定位 ----------
function Resolve-Tool {
  param([string[]]$Names, [string[]]$ExtraCandidates = @())
  foreach ($n in $Names) {
    $cmd = Get-Command $n -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($cmd) { return $cmd.Source }
  }
  foreach ($c in $ExtraCandidates) {
    if ($c -and (Test-Path $c)) { return $c }
  }
  return $null
}
function Get-GoExe {
  $candidates = @()
  if ($env:DEV_GO_EXE) { $candidates += $env:DEV_GO_EXE }
  else { $candidates += (Resolve-Tool @("go", "go.exe")) }
  $candidates += "$env:ProgramFiles\Go\bin\go.exe"
  $candidates += (Get-ChildItem "$env:USERPROFILE\.workbuddy\binaries\go\versions\*\go\bin\go.exe" -ErrorAction SilentlyContinue |
    Sort-Object FullName -Descending | ForEach-Object { $_.FullName })
  $candidates += "$env:USERPROFILE\scoop\apps\go\current\bin\go.exe"
  $candidates += "$env:LOCALAPPDATA\Programs\Go\bin\go.exe"
  foreach ($c in $candidates) {
    if ($c -and (Test-Path $c)) { return $c }
  }
  return $null
}
function Require-Command { param([string]$Name)
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) { Write-Fail "未找到必需命令：$Name" }
}
# npm 不一定在 PowerShell 的 PATH 里（常见于 nvm / 多版本 Node），这里按已知安装位置兜底查找。
# 返回 @{ Exe = 可执行文件; Prefix = 前置参数 }，调用：& $npm.Exe ($npm.Prefix + @('run','build'))
function Get-NpmInvoke {
  $cmd = Get-Command npm.cmd -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($cmd) { return @{ Exe = $cmd.Source; Prefix = @() } }
  $cmd = Get-Command npm -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($cmd -and $cmd.Source -like "*.cmd") { return @{ Exe = $cmd.Source; Prefix = @() } }

  $node = $null
  foreach ($c in @("$env:ProgramFiles\nodejs\node.exe", "$env:LOCALAPPDATA\hermes\node\node.exe")) {
    if (-not $node -and $c -and (Test-Path $c)) { $node = $c }
  }
  if (-not $node) {
    $node = Get-ChildItem "$env:USERPROFILE\.workbuddy\binaries\node\versions\*\node.exe" -ErrorAction SilentlyContinue |
      Sort-Object FullName -Descending | Select-Object -First 1 -ExpandProperty FullName
  }
  if ($node) {
    $dir = Split-Path -Parent $node
    $npmCmd = Join-Path $dir "npm.cmd"
    if (Test-Path $npmCmd) { return @{ Exe = $npmCmd; Prefix = @() } }
    $cli = Join-Path $dir "node_modules\npm\bin\npm-cli.js"
    if (Test-Path $cli) { return @{ Exe = $node; Prefix = @($cli) } }
  }
  return $null
}
function Format-FileSize { param([double]$Bytes)
  $units = @('B', 'KB', 'MB', 'GB', 'TB')
  $i = 0
  while ($Bytes -ge 1024 -and $i -lt 4) { $Bytes = $Bytes / 1024; $i++ }
  if ($i -eq 0) { return "$([int]$Bytes) $($units[$i])" }
  return ("{0:N1} {1}" -f $Bytes, $units[$i])
}

# ---------- 镜像包路径 ----------
function Resolve-TarPath { param([string]$Path)
  if ([string]::IsNullOrWhiteSpace($Path)) { $Path = $RemoteTar }
  if (-not [System.IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $RepoRoot $Path }
  $parent = Split-Path -Parent $Path
  if (-not (Test-Path $parent)) { Write-Fail "镜像包所在目录不存在：$parent" }
  $item = (Resolve-Path $parent).Path
  return (Join-Path $item (Split-Path -Leaf $Path))
}
$LocalTarFileEnv = (Get-Conf "LOCAL_TAR_FILE" "")
$requestedTar    = if ($TarPath) { $TarPath } elseif ($LocalTarFileEnv) { $LocalTarFileEnv } else { $RemoteTar }
$requestedAlt    = if ($LocalTarFileEnv) { $LocalTarFileEnv } else { $requestedTar }
$TarPath       = Resolve-TarPath $requestedTar
$AltTarPath    = Resolve-TarPath $requestedAlt
if ($TarPath -ne $AltTarPath) {
  Write-Fail "TAR_PATH 与 LOCAL_TAR_FILE 指向不同文件（$TarPath / $AltTarPath）。请让它们保持一致，或只设置其中一个。"
}
$env:TAR_PATH       = $TarPath
$env:LOCAL_TAR_FILE = $TarPath

# ---------- 端口探测（替代 nc -z -G） ----------
# 不能只看 TcpClient.Connected：在某些网络（代理 / Tailscale / Winsock LSP）下
# 对不可达地址也会立刻“连上”。真正可信的判据是读到 SSH 版本横幅 "SSH-2.0-…"。
function Test-SshPort { param([string]$Ip, [int]$TimeoutSec)
  $client = New-Object System.Net.Sockets.TcpClient
  try {
    $async = $client.BeginConnect($Ip, 22, $null, $null)
    if (-not $async.AsyncWaitHandle.WaitOne($TimeoutSec * 1000)) { return $false }
    $client.EndConnect($async) | Out-Null
    if (-not $client.Connected) { return $false }
    $client.ReceiveTimeout = $TimeoutSec * 1000
    $buf = New-Object byte[] 64
    $read = $client.GetStream().Read($buf, 0, 64)
    $banner = [System.Text.Encoding]::ASCII.GetString($buf, 0, $read)
    return ($banner -match '^\s*SSH-')
  } catch {
    return $false
  } finally {
    $client.Close()
  }
}
function Select-RouterHost {
  Write-Info "尝试软路由局域网地址 $RouterLanIp…"
  if (Test-SshPort $RouterLanIp ([int]$SshConnectTimeout)) {
    Write-Info "使用局域网地址：$RouterLanIp"
    return $RouterLanIp
  }
  Write-Info "局域网 SSH 不可达，尝试 Tailscale 地址 $RouterTailscaleIp…"
  if (Test-SshPort $RouterTailscaleIp ([int]$SshConnectTimeout)) {
    Write-Info "使用 Tailscale 地址：$RouterTailscaleIp"
    return $RouterTailscaleIp
  }
  Write-Fail "局域网地址 $RouterLanIp 与 Tailscale 地址 $RouterTailscaleIp 的 SSH 均不可达。"
}

# ---------- 密码：DPAPI 加密存档（替代 macOS 钥匙串） ----------
function Get-RouterPassword {
  if ($RouterPass) {
    # -RouterPass 配合 -SavePassword：把密码用 DPAPI 加密存档，之后免交互
    if ($SavePassword) {
      try {
        $sec = ConvertTo-SecureString $RouterPass -AsPlainText -Force
        $sec | ConvertFrom-SecureString | Set-Content -Path $PassFile -Encoding ASCII
        Write-Info "SSH 密码已用 DPAPI 加密存档到 scripts\.router-pass.clixml"
      } catch {
        Write-WarnMsg "密码存档写入失败（$PassFile）：$($_.Exception.Message)"
      }
    }
    return $RouterPass
  }
  if (-not $script:PlinkExe) {
    # OpenSSH 模式下密码由 ssh / scp 自己在终端提示，没必要在这里录入（也无处可用）
    return $null
  }
  if (-not $ResetPassword -and (Test-Path $PassFile)) {
    try {
      $sec = (Get-Content $PassFile -Raw | ConvertTo-SecureString -ErrorAction Stop)
      return ([System.Net.NetworkCredential]::new('', $sec).Password)
    } catch {
      Write-WarnMsg "已保存的密码无法解密（$PassFile），将重新询问。"
    }
  }
  $sec = Read-Host "请输入软路由 $RouterUser 的 SSH 密码（将用 DPAPI 加密保存到 scripts\.router-pass.clixml）" -AsSecureString
  $plain = ([System.Net.NetworkCredential]::new('', $sec).Password)
  if ([string]::IsNullOrEmpty($plain)) { Write-Fail "软路由 SSH 密码为空。" }
  try {
    # DPAPI 输出是十六进制文本，用 ASCII 存即可（避免 Set-Content -Encoding UTF8 写入 BOM）
    $sec | ConvertFrom-SecureString | Set-Content -Path $PassFile -Encoding ASCII
  } catch {
    Write-WarnMsg "密码存档写入失败（$PassFile），本次仅在内存中使用。$($_.Exception.Message)"
  }
  return $plain
}

# ---------- SSH 传输层：PuTTY（免交互） / OpenSSH（需手输密码） ----------
function Initialize-SshTransport {
  $mode = $SshMode.Trim().ToLowerInvariant()
  if ($mode -eq "auto" -or $mode -eq "putty") {
    $script:PlinkExe = Resolve-Tool @("plink", "plink.exe") @(
      "$env:ProgramFiles\PuTTY\plink.exe",
      "${env:ProgramFiles(x86)}\PuTTY\plink.exe",
      "$env:LOCALAPPDATA\Programs\PuTTY\plink.exe"
    )
    $script:PscpExe = Resolve-Tool @("pscp", "pscp.exe") @(
      "$env:ProgramFiles\PuTTY\pscp.exe",
      "${env:ProgramFiles(x86)}\PuTTY\pscp.exe",
      "$env:LOCALAPPDATA\Programs\PuTTY\pscp.exe"
    )
  }
  if ($script:PlinkExe -and $script:PscpExe) {
    Write-Info "SSH 传输层：PuTTY plink / pscp（免交互）"
    return
  }
  if ($mode -eq "putty") {
    Write-Fail "SSH_MODE=Putty 但未找到 plink.exe / pscp.exe。请安装 PuTTY（https://www.chiark.greenend.org.uk/~sgtatham/putty/），或改用 -SshMode OpenSsh。"
  }
  $script:SshExe = Resolve-Tool @("ssh", "ssh.exe") @("$env:SystemRoot\System32\OpenSSH\ssh.exe", "C:\Program Files\Git\usr\bin\ssh.exe")
  $script:ScpExe = Resolve-Tool @("scp", "scp.exe") @("$env:SystemRoot\System32\OpenSSH\scp.exe", "C:\Program Files\Git\usr\bin\scp.exe")
  if (-not $script:SshExe -or -not $script:ScpExe) {
    Write-Fail "未找到 plink/pscp，也未找到 ssh/scp。请安装 PuTTY，或启用 Windows 可选功能“OpenSSH 客户端”。"
  }
  Write-WarnMsg "未找到 PuTTY，改用 OpenSSH ssh / scp：密码需要在终端手动输入（备份、上传、部署可能各提示一次）。"
}

# PuTTY 是否支持 -pwfile（比 -pw 安全，密码不出现在命令行里）
function Test-PlinkSupportsPwFile {
  if (-not $script:PlinkExe) { return $false }
  $out = & $script:PlinkExe -pwfile 2>&1 | Out-String
  return ($out -notmatch 'unknown option')
}

# 用 ssh-keyscan 取主机公钥，算 MD5 冒号十六进制指纹（plink -hostkey 全版本通用格式）
function Get-SshKeyscanCandidates {
  $candidates = New-Object System.Collections.Generic.List[string]
  $cmd = Get-Command ssh-keyscan -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($cmd) { $candidates.Add($cmd.Source) }
  # Git for Windows 自带的 OpenSSH 通常比 Windows 内置版新（内置版可能不支持新 KEX）
  $git = Get-Command git.exe -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($git) {
    $dir = Split-Path -Parent $git.Source
    foreach ($up in @((Split-Path -Parent $dir), (Split-Path -Parent (Split-Path -Parent $dir)))) {
      if ($up) { $candidates.Add((Join-Path $up "usr\bin\ssh-keyscan.exe")) }
    }
  }
  $candidates.Add("C:\Program Files\Git\usr\bin\ssh-keyscan.exe")
  $candidates.Add("$env:SystemRoot\System32\OpenSSH\ssh-keyscan.exe")
  $candidates.Add("$env:SystemRoot\Sysnative\OpenSSH\ssh-keyscan.exe")   # 32 位 PowerShell 访问真实 System32
  return ($candidates | Where-Object { $_ -and (Test-Path $_) } | Select-Object -Unique)
}
function Initialize-HostKeys {
  $candidates = Get-SshKeyscanCandidates
  if ($candidates.Count -eq 0) {
    Write-WarnMsg "未找到 ssh-keyscan，无法自动预置主机指纹，稍后将改用交互式确认主机密钥。"
    return
  }
  foreach ($keyscan in $candidates) {
    $lines = & $keyscan -p 22 -T ([int]$SshConnectTimeout + 5) -t rsa,ed25519,ecdsa $script:RouterIp 2>$null
    foreach ($line in $lines) {
      if ("$line" -match '^\s*#') { continue }
      $fields = -split "$line"
      if ($fields.Count -lt 3) { continue }
      try {
        $blob = [Convert]::FromBase64String($fields[2])
        $md5  = [System.Security.Cryptography.MD5]::Create()
        $hash = $md5.ComputeHash($blob)
        $script:HostKeys += (($hash | ForEach-Object { $_.ToString('x2') }) -join ':')
      } catch { }
    }
    if ($script:HostKeys.Count -gt 0) { break }
  }
  $script:HostKeys = @($script:HostKeys | Select-Object -Unique)
  if ($script:HostKeys.Count -eq 0) {
    Write-WarnMsg "ssh-keyscan 未取到 $($script:RouterIp) 的主机公钥，稍后将改用交互式确认主机密钥。"
  }
}

# 拿不到指纹时的兜底：去掉 -batch 连一次，让用户在终端确认并缓存主机密钥，
# 之后所有调用仍走 -batch（PuTTY 会把密钥写进注册表缓存）。
function Invoke-PuttyHostKeyWarmup {
  $argsList = New-Object System.Collections.Generic.List[string]
  $argsList.Add("-ssh")
  $argsList.Add("-P")
  $argsList.Add("22")
  if ($script:UsePwFile) { $argsList.Add("-pwfile"); $argsList.Add($script:PwFilePath) }
  else { $argsList.Add("-pw"); $argsList.Add($script:RouterPassPlain) }
  $argsList.Add("$RouterUser@$($script:RouterIp)")
  $argsList.Add("exit")
  # 不重定向输出：主机密钥确认提示必须让用户看到并回答
  & $script:PlinkExe $argsList.ToArray()
  return $LASTEXITCODE
}

# 统一的 plink / pscp 调用。-NoSshFlag 用于 pscp（pscp 不接受 -ssh 协议选项）。
function Invoke-Putty {
  param([string]$Exe, [string[]]$Tail, [switch]$NoSshFlag, [switch]$Capture)
  $argsList = New-Object System.Collections.Generic.List[string]
  $argsList.Add("-batch")
  if (-not $NoSshFlag) { $argsList.Add("-ssh") }
  $argsList.Add("-P")
  $argsList.Add("22")
  foreach ($fp in $script:HostKeys) { $argsList.Add("-hostkey"); $argsList.Add($fp) }
  if ($script:UsePwFile) {
    $argsList.Add("-pwfile")
    $argsList.Add($script:PwFilePath)
  } else {
    $argsList.Add("-pw")
    $argsList.Add($script:RouterPassPlain)
  }
  foreach ($t in $Tail) { $argsList.Add($t) }
  if ($Capture) { return (& $Exe $argsList.ToArray() 2>$null) }
  # 关键：输出必须重定向到宿主控制台。若直接 `& $Exe ...`，pscp 的进度行会
  # 混入本函数的返回值，`$code = Invoke-Putty ...` 拿到的是「进度行数组+退出码」，
  # `if ($code -ne 0)` 对非空数组恒为真——pscp 成功也会被误判为失败。
  & $Exe $argsList.ToArray() | Out-Host
  return $LASTEXITCODE
}

function Invoke-OpenSshCopy {
  param([string]$Source, [string]$Destination)
  # 输出走宿主控制台，避免混入返回值（同 Invoke-Putty 的说明）
  & $script:ScpExe -o "ConnectTimeout=$SshConnectTimeout" -o StrictHostKeyChecking=accept-new $Source $Destination | Out-Host
  return $LASTEXITCODE
}

# 远端执行一段 shell 脚本（PuTTY 走 plink，OpenSSH 走 ssh）
  # 注意：PS 5.1 把参数传给原生程序时会剥掉/错乱脚本里内嵌的双引号，
  # 远端 ash 会报 "unterminated quoted string"。因此统一改为 base64 传输：
  # 本地 UTF-8 编码，远端解码后交给 sh（软路由无 base64 命令，用 openssl）。
  # 必须用 `openssl enc -d -base64 -A`：软路由 OpenSSL 3.0 的 `openssl base64 -d`
  # 对长单行输入只解出第一个块（实测 5248 字符只解出 94 字节）。
  function Invoke-RemoteScript {
    param([string]$Script, [switch]$Capture)
    $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Script))
    $wrapped = "echo $b64 | openssl enc -d -base64 -A | sh"
  if ($script:PlinkExe) {
    return (Invoke-Putty -Exe $script:PlinkExe -Tail @("$RouterUser@$($script:RouterIp)", $wrapped) -Capture:$Capture)
  }
  if ($Capture) {
    return (& $script:SshExe -o "ConnectTimeout=$SshConnectTimeout" -o StrictHostKeyChecking=accept-new "$RouterUser@$($script:RouterIp)" $wrapped 2>$null)
  }
  # 输出走宿主控制台，避免混入返回值（同 Invoke-Putty 的说明）
  & $script:SshExe -o "ConnectTimeout=$SshConnectTimeout" -o StrictHostKeyChecking=accept-new "$RouterUser@$($script:RouterIp)" $wrapped | Out-Host
  return $LASTEXITCODE
}

# ---------- 本地 Docker 缓存清理 ----------
function Invoke-DockerCacheCleanup {
  switch ($PruneDockerCache) {   # 大小写不敏感：TRUE / NO 等写法同样有效
    "0" { Write-Step "跳过 Docker 缓存清理（PRUNE_DOCKER_CACHE=$PruneDockerCache）"; return }
    "1" {
      Write-Step "回收 ${BuildCacheMaxAge} 前的未使用 BuildKit 缓存及悬空镜像"
      & docker builder prune --force --filter "until=$BuildCacheMaxAge"
      & docker image prune --force --filter "until=$BuildCacheMaxAge"
      return
    }
    "false" { Write-Step "跳过 Docker 缓存清理（PRUNE_DOCKER_CACHE=$PruneDockerCache）"; return }
    "no" { Write-Step "跳过 Docker 缓存清理（PRUNE_DOCKER_CACHE=$PruneDockerCache）"; return }
    "true" {
      Write-Step "回收 ${BuildCacheMaxAge} 前的未使用 BuildKit 缓存及悬空镜像"
      & docker builder prune --force --filter "until=$BuildCacheMaxAge"
      & docker image prune --force --filter "until=$BuildCacheMaxAge"
      return
    }
    "yes" {
      Write-Step "回收 ${BuildCacheMaxAge} 前的未使用 BuildKit 缓存及悬空镜像"
      & docker builder prune --force --filter "until=$BuildCacheMaxAge"
      & docker image prune --force --filter "until=$BuildCacheMaxAge"
      return
    }
    "all" {
      Write-Step "回收全部未使用 BuildKit 缓存及悬空镜像"
      & docker builder prune --force
      & docker image prune --force
      return
    }
    default { Write-Fail "PRUNE_DOCKER_CACHE 仅支持 0、1 或 all（当前：$PruneDockerCache）" }
  }
}

# ---------- 部署到软路由 ----------
function Deploy-ToRouter {
  if (-not (Test-Path $LocalBackupDir)) {
    New-Item -ItemType Directory -Force -Path $LocalBackupDir | Out-Null
  }

  Write-Step "5/6 备份旧镜像并上传新镜像包"
  Write-Info "备份目录：$LocalBackupDir"

  # pscp 对远端通配符支持有限，这里先列出再逐个下载（等价于 scp 'dir/*.tar'）
  $remoteNames = @()
  $listing = Invoke-RemoteScript -Capture "cd $RemoteDir && ls -1 *.tar 2>/dev/null || true"
  foreach ($line in $listing) {
    $n = "$line".Trim()
    if ($n -and $n -notmatch '^\s*#') { $remoteNames += $n }
  }
  if ($remoteNames.Count -eq 0) {
    Write-Info "软路由上未发现旧镜像包（$RemoteDir/*.tar），跳过备份。"
  }
  foreach ($n in $remoteNames) {
    $src = "$RouterUser@$($script:RouterIp):$RemoteDir/$n"
    Write-Info "备份：$src"
    if ($script:PscpExe) {
      $code = Invoke-Putty -Exe $script:PscpExe -NoSshFlag -Tail @($src, "$LocalBackupDir\")
    } else {
      $code = Invoke-OpenSshCopy -Source $src -Destination "$LocalBackupDir\"
    }
    if ($code -ne 0) { Write-Fail "软路由旧镜像备份失败（$n），更新已停止。" }
  }

  Write-Info "上传位置：$RemoteDir/$RemoteTar"
  if ($script:PscpExe) {
    $code = Invoke-Putty -Exe $script:PscpExe -NoSshFlag -Tail @($TarPath, "$RouterUser@$($script:RouterIp):$RemoteDir/$RemoteTar")
  } else {
    $code = Invoke-OpenSshCopy -Source $TarPath -Destination "$RouterUser@$($script:RouterIp):$RemoteDir/$RemoteTar"
  }
  if ($code -ne 0) { Write-Fail "镜像上传失败，软路由容器未更新。" }

  $remoteUpdate = @'
set -eu
cd /mnt/opt/docker/install
if ! target_image=$(docker inspect --format "{{.Config.Image}}" gubuy-erp-app 2>/dev/null); then
  echo "未找到软路由现有 ERP app 容器，无法继承其镜像名称" >&2
  exit 1
fi
case "${target_image:-}" in
  ""|sha256:*|*@sha256:*) echo "无法识别现有 app 镜像名：${target_image:-<空值>}" >&2; exit 1 ;;
esac
docker load -i erpwonchon-app.tar
new_image_id=$(docker image inspect --format "{{.Id}}" erpwonchon-app:latest)
# 架构兜底校验：镜像架构必须与主机一致，否则容器会 exec format error 崩溃循环
host_arch=$(uname -m)
case "$host_arch" in
  aarch64) host_arch=arm64 ;;
  x86_64) host_arch=amd64 ;;
esac
img_arch=$(docker image inspect --format "{{.Architecture}}" erpwonchon-app:latest)
if [ "$img_arch" != "$host_arch" ]; then
  echo "镜像架构与主机不匹配：镜像=$img_arch 主机=$host_arch，拒绝更新" >&2
  exit 1
fi
docker image tag erpwonchon-app:latest "$target_image"
# 服务端密钥只保存在软路由上，不进入镜像包。
ai_secret_file=.erp-ai-config-secret
jwt_secret_file=.erp-jwt-secret
override_file=.erp-runtime-secrets.compose.yml
current_env=$(docker inspect --format "{{range .Config.Env}}{{println .}}{{end}}" gubuy-erp-app)
current_ai_secret=$(printf "%s\n" "$current_env" | sed -n "s/^AI_CONFIG_SECRET=//p" | tail -n 1)
current_jwt_secret=$(printf "%s\n" "$current_env" | sed -n "s/^JWT_SECRET=//p" | tail -n 1)
umask 077
if [ -s "$ai_secret_file" ]; then
  ai_secret=$(cat "$ai_secret_file")
else
  ai_secret=$current_ai_secret
  if [ "${#ai_secret}" -lt 24 ] ||
    [ "$ai_secret" = please_change_this_secret ] ||
    [ "$ai_secret" = erp_wonchon_default_secret_key_2026_please_change ]; then
    ai_secret=$(LC_ALL=C tr -dc "A-Za-z0-9" < /dev/urandom | head -c 48)
  fi
  printf "%s" "$ai_secret" > "$ai_secret_file"
fi
if [ "${#ai_secret}" -lt 24 ]; then
  echo "AI 加密密钥文件无效：$ai_secret_file" >&2
  exit 1
fi
chmod 600 "$ai_secret_file"

if [ -s "$jwt_secret_file" ]; then
  jwt_secret=$(cat "$jwt_secret_file")
else
  jwt_secret=$current_jwt_secret
  if [ "${#jwt_secret}" -lt 32 ] ||
    [ "$jwt_secret" = please_change_this_secret ] ||
    [ "$jwt_secret" = erp_wonchon_default_secret_key_2026_please_change ]; then
    jwt_secret=$(LC_ALL=C tr -dc "A-Za-z0-9" < /dev/urandom | head -c 64)
  fi
  printf "%s" "$jwt_secret" > "$jwt_secret_file"
fi
if [ "${#jwt_secret}" -lt 32 ] ||
  [ "$jwt_secret" = please_change_this_secret ] ||
  [ "$jwt_secret" = erp_wonchon_default_secret_key_2026_please_change ]; then
  echo "JWT 密钥文件无效：$jwt_secret_file（需至少 32 字节）" >&2
  exit 1
fi
chmod 600 "$jwt_secret_file"

printf "services:\n  app:\n    environment:\n      AI_CONFIG_SECRET: %s\n      JWT_SECRET: %s\n" \
  "$ai_secret" "$jwt_secret" > "$override_file"
chmod 600 "$override_file"
set -- -f docker-compose.yml
set -- "$@" -f "$override_file"
if [ -f .erp-app-dns.compose.yml ]; then set -- "$@" -f .erp-app-dns.compose.yml; fi
if command -v docker-compose >/dev/null 2>&1; then
  docker-compose "$@" up -d --no-build --no-deps --force-recreate app
else
  docker compose "$@" up -d --no-build --no-deps --force-recreate app
fi
running_image_id=$(docker inspect --format "{{.Image}}" gubuy-erp-app)
if [ "$running_image_id" != "$new_image_id" ]; then
  echo "容器仍使用旧镜像：$running_image_id；期望：$new_image_id" >&2
  exit 1
fi
status=unknown
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  status=$(docker inspect --format "{{.State.Status}}" gubuy-erp-app)
  [ "$status" = running ] && break
  sleep 2
done
if [ "$status" != running ]; then
  echo "容器未进入 running 状态（当前：$status），最近日志如下：" >&2
  docker logs --tail 80 gubuy-erp-app >&2 || true
  exit 1
fi
container_env=$(docker inspect --format "{{range .Config.Env}}{{println .}}{{end}}" gubuy-erp-app)
if ! printf "%s\n" "$container_env" | grep -Fqx "AI_CONFIG_SECRET=$ai_secret"; then
  echo "容器未收到 AI_CONFIG_SECRET" >&2
  exit 1
fi
if ! printf "%s\n" "$container_env" | grep -Fqx "JWT_SECRET=$jwt_secret"; then
  echo "容器未收到 JWT_SECRET" >&2
  exit 1
fi
echo "镜像校验通过：$new_image_id"
echo "AI_CONFIG_SECRET 与 JWT_SECRET 已持久保存并注入容器"
if [ "__REMOTE_PRUNE__" = 1 ]; then docker image prune --force --filter dangling=true; fi
'@
  $remoteUpdate = $remoteUpdate -replace '__REMOTE_PRUNE__', $RemotePruneDockerImages

  Write-Step "6/6 更新软路由 app 并校验运行状态"
  $code = Invoke-RemoteScript -Script $remoteUpdate
  if ($code -ne 0) { Write-Fail "软路由容器更新失败，请查看上方 SSH/Docker 错误。" }

  $code = Invoke-RemoteScript -Script "docker ps --filter name=gubuy-erp-app --format '{{.Names}} {{.Image}} {{.Status}}'"
  if ($code -ne 0) { Write-Fail "容器更新完成，但无法读取软路由容器状态。" }
}

# ================= 主流程 =================
Push-Location $RepoRoot
try {
  foreach ($cmdName in @('docker', 'tar')) { Require-Command $cmdName }
  $goExe = Get-GoExe
  if (-not $goExe) { Write-Fail "未找到 Go。请安装 Go 1.26，或通过 DEV_GO_EXE 指定 go.exe 完整路径。" }
  $npmInvoke = Get-NpmInvoke
  if (-not $npmInvoke) { Write-Fail "未找到 npm。请安装 Node.js 22，或把 npm.cmd 所在目录加入 PATH。" }
  Initialize-SshTransport

  if (@('0','1','false','no','true','yes','all') -notcontains $PruneDockerCache.ToLowerInvariant()) {
    Write-Fail "PRUNE_DOCKER_CACHE 仅支持 0、1 或 all（当前：$PruneDockerCache）"
  }
  if (@('0','1') -notcontains $RemotePruneDockerImages) {
    Write-Fail "REMOTE_PRUNE_DOCKER_IMAGES 仅支持 0 或 1（当前：$RemotePruneDockerImages）"
  }

  Write-Host ""
  Write-Host "=================================================="
  Write-Host "开始一键构建并更新软路由"
  Write-Host "镜像包：$TarPath"
  Write-Host "=================================================="

  Write-Step "1/6 部署前检查：确认软路由 SSH 可达"
  $script:RouterIp = Select-RouterHost
  $connType = if ($script:RouterIp -eq $RouterLanIp) { "局域网" } else { "Tailscale" }
  Write-Info "部署目标：$($script:RouterIp)（$connType）"

  $script:RouterPassPlain = Get-RouterPassword
  if ($script:PlinkExe -and [string]::IsNullOrEmpty($script:RouterPassPlain)) { Write-Fail "软路由 SSH 密码为空。" }

  # PuTTY 优先用 -pwfile（密码不进入命令行），不支持时退回 -pw
  $script:UsePwFile = $false
  $script:PwFilePath = $null
  if ($script:PlinkExe) {
    Initialize-HostKeys
    if (Test-PlinkSupportsPwFile) {
      $script:PwFilePath = Join-Path ([IO.Path]::GetTempPath()) "erp-router-pass-$PID.txt"
      # 必须写 UTF-8 无 BOM：PS 5.1 的 Set-Content -Encoding UTF8 会写 BOM，
      # 而 plink 会把 BOM 当成密码的一部分，导致密码正确也登录失败。
      [System.IO.File]::WriteAllText($script:PwFilePath, $script:RouterPassPlain, (New-Object System.Text.UTF8Encoding($false)))
      $script:UsePwFile = $true
    }
    if ($script:HostKeys.Count -eq 0) {
      Write-WarnMsg "现在连一次软路由以确认主机密钥（出现 Store key in cache? 时输入 y）；之后全程免交互。"
      $warm = Invoke-PuttyHostKeyWarmup
      if ($warm -ne 0) {
        Write-Fail "无法与软路由建立 SSH 连接（退出码 $warm）。请检查主机密钥是否正确、密码是否正确，或手动执行一次：plink -ssh $RouterUser@$($script:RouterIp)"
      }
    }
  }

  Write-Info "校验 Docker Compose 配置"
  # compose 入口随环境而变：部分 Docker Desktop 版本把 compose 放在 resources\cli-plugins，
  # 且该目录不进 docker CLI 的插件搜索路径，此时 `docker compose` 会报 unknown command。
  # 独立 docker-compose 始终可用，优先用它；两者都没有才视为环境缺失。
  if (Get-Command docker-compose -ErrorAction SilentlyContinue) {
    & docker-compose config -q
  } elseif (Get-Command docker -ErrorAction SilentlyContinue) {
    & docker compose config --quiet
  } else {
    Write-Fail "未找到 docker-compose 或 docker，无法校验 Compose 配置。"
  }
  if ($LASTEXITCODE -ne 0) {
    Write-Fail "Docker Compose 配置校验失败。请先修复上方错误；首次部署需在项目根目录 .env 中设置 JWT_SECRET（私有随机值，至少 32 字节）。更换密钥会使现有登录会话失效。"
  }

  Write-Step "2/6 构建校验：前端与后端"
  Write-Info "前端：npm run build"
  & $npmInvoke.Exe ($npmInvoke.Prefix + @('run', 'build'))
  if ($LASTEXITCODE -ne 0) { Write-Fail "前端构建失败。" }

  Write-Info "后端：go build ."
  Push-Location $BackendDir
  try {
    # 不使用 ./...：erp-backend/scripts 下是多个独立维护工具，不能作为一个 Go package 一起构建。
    & $goExe build .
    if ($LASTEXITCODE -ne 0) { Write-Fail "后端构建失败。" }
  } finally {
    Pop-Location
  }

  if ($RunTests -eq "1") {
    if ([string]::IsNullOrEmpty($AuditDbName) -or $AuditDbName -eq "erp_db") {
      Write-Fail "RunTests=1 时必须指定非生产审计库（默认：erp_audit_test），禁止对 erp_db 跑测试"
    }
    Write-Step "可选审计测试：DB_NAME=$AuditDbName go test ./controllers"
    Push-Location $BackendDir
    try {
      $env:DB_NAME = $AuditDbName
      & $goExe test ./controllers
      if ($LASTEXITCODE -ne 0) { Write-Fail "后端测试失败。" }
    } finally {
      Pop-Location
    }
  } else {
    Write-Step "跳过后端测试（默认不触碰数据库；如需执行：scripts\build-deploy-router.cmd -RunTests 1）"
  }

  Write-Step "3/6 构建 Docker 镜像：app（目标架构 $AppPlatform）"
  # 注：本机 docker compose build 不支持 --platform，改用 buildx；
  # --load 把构建结果载入本地镜像存储（默认 buildx 结果只存构建缓存）。
  & docker buildx build --platform $AppPlatform --load -t $AppImage .
  if ($LASTEXITCODE -ne 0) {
    Write-WarnMsg "常规构建失败，尝试 --pull=false（适合网络较差但本地已有基础镜像的情况）..."
    & docker buildx build --platform $AppPlatform --pull=false --load -t $AppImage .
    if ($LASTEXITCODE -ne 0) { Write-Fail "Docker 镜像构建失败。" }
  }
  # 构建后立刻校验镜像架构，防止把 amd64 镜像推上 arm64 软路由
  $imgArch = (& docker image inspect $AppImage --format "{{.Os}}/{{.Architecture}}" 2>$null)
  if ($LASTEXITCODE -ne 0 -or "$imgArch" -ne $AppPlatform) {
    Write-Fail "镜像架构不匹配：期望 $AppPlatform，实际 $imgArch。"
  }
  Write-Info "镜像架构校验通过：$imgArch"

  Write-Step "4/6 导出镜像包并清理本地构建缓存"
  & docker save $AppImage -o "$TarPath"
  if ($LASTEXITCODE -ne 0) { Write-Fail "镜像导出失败：$TarPath" }
  # 显式用系统自带 bsdtar：若 PATH 里是 Git/MSYS 的 GNU tar，会把 "C:\..."
  # 解析成「远程主机:路径」而直接失败。找不到系统 tar 时才退回 PATH。
  $tarExe = Join-Path $env:SystemRoot "System32\tar.exe"
  if (-not (Test-Path $tarExe)) { $tarExe = "tar" }
  $entries = & $tarExe -tf "$TarPath" 2>$null
  # Docker 29+（containerd 镜像存储）导出 OCI 布局（index.json），
  # 同时兼容旧 docker-archive 格式（manifest.json）。
  $validArchive = ($LASTEXITCODE -eq 0) -and
    (($entries -contains "manifest.json") -or ($entries -contains "index.json"))
  if (-not $validArchive) {
    Write-Fail "镜像包无效或不完整：$TarPath"
  }
  $size = (Get-Item $TarPath).Length
  Write-Info ("镜像包：{0}（{1}）" -f $TarPath, (Format-FileSize $size))

  Invoke-DockerCacheCleanup

  if (-not (Test-Path $TarPath)) { Write-Fail "未生成镜像包：$TarPath" }

  Deploy-ToRouter

  Write-Host ""
  Write-Host "=================================================="
  Write-Host "新镜像已构建并完成软路由部署；Windows 本机 app 容器未启动。"
  Write-Host "镜像包：$TarPath"
  Write-Host "=================================================="
} finally {
  Pop-Location
  if ($script:PwFilePath -and (Test-Path $script:PwFilePath)) {
    Remove-Item $script:PwFilePath -Force -ErrorAction SilentlyContinue
  }
}
