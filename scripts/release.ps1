param(
    [string]$Target = 'C:\path\mdview.exe',
    [switch]$CheckOnly
)

$ErrorActionPreference = 'Stop'
$src = Join-Path (Split-Path -Parent $PSScriptRoot) 'src'
$Target = [IO.Path]::GetFullPath($Target)
$targetDir = Split-Path -Parent $Target
$versionFile = "$Target.version"
$token = [guid]::NewGuid().ToString('N')
$work = (New-Item -ItemType Directory -Path (Join-Path ([IO.Path]::GetTempPath()) "mdview-release-$token")).FullName
$built = Join-Path $work 'mdview.exe'
$staged = "$Target.pending-$token"
$stagedVersion = "$versionFile.pending-$token"
$backup = "$Target.previous-$token"
$lock = $null

function Test-Startup {
    $document = Join-Path $work 'startup.md'
    $inputFile = Join-Path $work 'startup.json'
    $stdout = Join-Path $work 'stdout.txt'
    $stderr = Join-Path $work 'stderr.txt'
    [IO.File]::WriteAllText($document, '# Startup check')
    $state = @{ File = $document; ScrollY = 0; Visible = $false } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($inputFile, $state)

    $envNames = @('MDVIEW_CONFIG_DIR', 'APPDATA', 'LOCALAPPDATA', 'TEMP', 'TMP')
    $previous = @{}
    foreach ($name in $envNames) { $previous[$name] = [Environment]::GetEnvironmentVariable($name) }
    try {
        $env:MDVIEW_CONFIG_DIR = Join-Path $work 'config'
        $env:APPDATA = Join-Path $work 'roaming'
        $env:LOCALAPPDATA = Join-Path $work 'local'
        $env:TEMP = $work
        $env:TMP = $work
        $process = Start-Process -FilePath $built -ArgumentList "--resume=$id" -WindowStyle Hidden -PassThru `
            -RedirectStandardInput $inputFile -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    } finally {
        foreach ($name in $envNames) { [Environment]::SetEnvironmentVariable($name, $previous[$name]) }
    }
    try {
        if (-not $process.WaitForExit(20000)) {
            $process.Kill()
            [void]$process.WaitForExit(3000)
            throw 'Startup check timed out.'
        }
        $reply = Get-Content -LiteralPath $stdout -Raw
        if ($process.ExitCode -ne 0 -or -not $reply -or $reply.Trim() -ne 'ready') {
            $details = Get-Content -LiteralPath $stderr -Raw
            throw "Startup check failed (exit $($process.ExitCode)). $details"
        }
    } finally {
        $process.Dispose()
    }
}

try {
    $number = [DateTime]::UtcNow.Ticks
    if (-not $CheckOnly) {
        New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
        $lock = [IO.File]::Open("$Target.publish.lock", 'OpenOrCreate', 'ReadWrite', 'None')
        if ([IO.File]::Exists($versionFile)) {
            $previousID = [IO.File]::ReadAllText($versionFile).Trim()
            if ($previousID -match '^\d{19}$') {
                $number = [Math]::Max($number, ([long]$previousID + 1))
            }
        }
    }
    $id = $number.ToString('D19', [Globalization.CultureInfo]::InvariantCulture)

    Write-Host '[release] Running Go tests...'
    & go -C $src test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }

    Write-Host '[release] Building...'
    & go -C $src build -trimpath "-ldflags=-s -w -H windowsgui -X main.buildID=$id" -o $built .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }

    Write-Host '[release] Checking startup...'
    Test-Startup
    if ($CheckOnly) { return }

    Copy-Item -LiteralPath $built -Destination $staged
    [IO.File]::WriteAllText($stagedVersion, $id)

    # Windows lets a running executable keep its file open under a new name.
    # Move on the target volume so the ready executable comes into place at once.
    if ([IO.File]::Exists($Target)) { [IO.File]::Move($Target, $backup) }
    $placed = $false
    try {
        [IO.File]::Move($staged, $Target)
        $placed = $true
        if ([IO.File]::Exists($versionFile)) {
            [IO.File]::Replace($stagedVersion, $versionFile, [NullString]::Value)
        } else {
            [IO.File]::Move($stagedVersion, $versionFile)
        }
    } catch {
        if ($placed) { [IO.File]::Move($Target, $staged) }
        if ([IO.File]::Exists($backup)) { [IO.File]::Move($backup, $Target) }
        throw
    }

    # Earlier copies may still be in use while their windows switch over.
    $prefix = [IO.Path]::GetFileName($Target) + '.previous-'
    Get-ChildItem -LiteralPath $targetDir -File | Where-Object {
        $_.Name.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) -and
        $_.Name.Substring($prefix.Length) -match '^[0-9a-f]{32}$'
    } | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }
    Write-Host "[release] Published $id to $Target. Open windows can now update."
} finally {
    if ($lock) { $lock.Dispose() }
    if (-not $CheckOnly) {
        Remove-Item -LiteralPath $staged,$stagedVersion -Force -ErrorAction SilentlyContinue
    }
    $scratch = Get-Item -LiteralPath $work -ErrorAction SilentlyContinue
    if ($scratch -and $scratch.FullName -eq $work -and -not ($scratch.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        Remove-Item -LiteralPath $scratch.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
}
