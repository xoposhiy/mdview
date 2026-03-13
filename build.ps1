$ErrorActionPreference = "Stop"
Set-Location "$PSScriptRoot\src"

Write-Host "[mdview] Checking Go..."
$goVersion = & go version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Go not found. Install from https://go.dev/dl/"
    exit 1
}
Write-Host $goVersion

Write-Host "[mdview] Fetching dependencies..."
& go get github.com/jchv/go-webview2 github.com/yuin/goldmark github.com/fsnotify/fsnotify
if ($LASTEXITCODE -ne 0) { Write-Error "go get failed"; exit 1 }

& go mod tidy
if ($LASTEXITCODE -ne 0) { Write-Error "go mod tidy failed"; exit 1 }

Write-Host "[mdview] Running tests..."
& go test ./...
if ($LASTEXITCODE -ne 0) { Write-Error "Tests failed"; exit 1 }

Write-Host "[mdview] Building..."
& go build -trimpath -ldflags="-s -w -H windowsgui" -o "$PSScriptRoot\mdview.exe" .
if ($LASTEXITCODE -ne 0) { Write-Error "Build failed"; exit 1 }

Write-Host ""
Write-Host "Done! mdview.exe is ready."
