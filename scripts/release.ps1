# release.ps1 [-Target C:\path\mdview.exe]
#
# Builds an optimized mdview.exe, closes every running mdview (from any
# location), and copies the new exe to the target path.
#
# The build runs before any window is closed, so a failed test or build leaves
# the running copies alone.
param(
    [string]$Target = "C:\path\mdview.exe"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$src = Join-Path $root "src"
$built = Join-Path $root "mdview.exe"
$targetDir = Split-Path -Parent $Target

Write-Host "[release] Running tests..."
& go -C $src test ./...
if ($LASTEXITCODE -ne 0) { throw "Tests failed" }

Write-Host "[release] Building..."
& go -C $src build -trimpath -ldflags="-s -w -H windowsgui" -o $built .
if ($LASTEXITCODE -ne 0) { throw "Build failed" }

$running = @(Get-Process -Name mdview -ErrorAction SilentlyContinue)
if ($running.Count -gt 0) {
    Write-Host "[release] Closing $($running.Count) mdview process(es)..."
    # Ask each window to close first, so mdview can save its settings.
    foreach ($p in $running) { [void]$p.CloseMainWindow() }
    foreach ($p in $running) {
        if (-not $p.WaitForExit(3000)) {
            Write-Host "[release] Killing $($p.Id) ($($p.Path))"
            Stop-Process -Id $p.Id -Force
            [void]$p.WaitForExit(3000)
        }
    }
}

Write-Host "[release] Copying to $Target..."
New-Item -ItemType Directory -Force $targetDir | Out-Null
Copy-Item $built $Target -Force

$info = Get-Item $Target
Write-Host "[release] Done: $($info.FullName) ($([math]::Round($info.Length / 1MB, 1)) MB, $($info.LastWriteTime))"
