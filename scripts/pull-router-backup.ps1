$ErrorActionPreference = 'Stop'
$dest = 'D:\工作\ERP备份'
$log = Join-Path $dest 'pull-log.txt'
$sshOpts = @('-o','BatchMode=yes','-o','ConnectTimeout=10')
try {
    $latest = (& ssh @sshOpts root@192.168.5.22 "ls -1t /mnt/opt/docker/backups/erp_db_*.sql.gz 2>/dev/null | head -1") | Select-Object -First 1
    if (-not $latest) { throw "router has no backup file" }
    $name = (Split-Path $latest -Leaf).Trim()
    $local = Join-Path $dest $name
    if (Test-Path $local) {
        Add-Content $log "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') SKIP already have $name"
        exit 0
    }
    & scp @sshOpts "root@192.168.5.22:${latest}" "$dest\" 2>$null
    if ($LASTEXITCODE -ne 0) { throw "scp failed with exit code $LASTEXITCODE" }
    $sz = (Get-Item $local).Length
    Add-Content $log "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') OK $name size=$sz"
} catch {
    Add-Content $log "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') FAIL $($_.Exception.Message)"
    exit 1
}
Get-ChildItem $dest -Filter 'erp_db_*.sql.gz' |
    Where-Object { $_.LastWriteTime -lt (Get-Date).AddDays(-30) } |
    Remove-Item -Force
