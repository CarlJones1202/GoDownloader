# AG-Godownload Repository Initialization Script
# This script installs necessary dependencies and sets up the project.

$ErrorActionPreference = "Stop"

function Write-Step($message) {
    Write-Host "`n[STEP] $message" -ForegroundColor Cyan
}

function Write-Success($message) {
    Write-Host "[SUCCESS] $message" -ForegroundColor Green
}

function Write-Warning-Host($message) {
    Write-Host "[WARNING] $message" -ForegroundColor Yellow
}

# 1. Check for Admin Privileges
Write-Step "Checking for administrative privileges..."
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Warning-Host "This script may need administrative privileges to install packages via winget."
    Write-Warning-Host "If it fails, please run PowerShell as Administrator."
} else {
    Write-Success "Running as Administrator."
}

# 2. Check for winget
Write-Step "Checking for winget..."
if (-not (Get-Command "winget" -ErrorAction SilentlyContinue)) {
    Write-Error "winget is not installed. Please install 'App Installer' from the Microsoft Store."
}
Write-Success "winget found."

# 3. Install FFmpeg
Write-Step "Checking for FFmpeg..."
if (-not (Get-Command "ffmpeg" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing FFmpeg via winget..."
    winget install --id Gyan.FFmpeg --silent --accept-package-agreements --accept-source-agreements
    # Refresh path
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "ffmpeg" -ErrorAction SilentlyContinue) {
    Write-Success "FFmpeg is ready."
} else {
    Write-Warning-Host "FFmpeg was installed but 'ffmpeg' command is not in the current session path."
    Write-Warning-Host "Restart your terminal after this script completes."
}

# 4. Install yt-dlp
Write-Step "Checking for yt-dlp..."
if (-not (Get-Command "yt-dlp" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing yt-dlp via winget..."
    winget install --id yt-dlp.yt-dlp --silent --accept-package-agreements --accept-source-agreements --source winget
    # Refresh path
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "yt-dlp" -ErrorAction SilentlyContinue) {
    Write-Success "yt-dlp is ready."
} else {
    Write-Warning-Host "yt-dlp was installed but 'yt-dlp' command is not in the current session path."
}

# 5. Install Go
Write-Step "Checking for Go..."
if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Go via winget..."
    winget install --id GoLang.Go --silent --accept-package-agreements --accept-source-agreements
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "go" -ErrorAction SilentlyContinue) {
    Write-Success "Go is ready ($(go version))."
} else {
    Write-Warning-Host "Go was installed but 'go' command is not in the current session path."
}

# 5. Install Node.js
Write-Step "Checking for Node.js..."
if (-not (Get-Command "node" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Node.js via winget..."
    winget install --id OpenJS.NodeJS.LTS --silent --accept-package-agreements --accept-source-agreements
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "node" -ErrorAction SilentlyContinue) {
    Write-Success "Node.js is ready ($(node -v))."
} else {
    Write-Warning-Host "Node.js was installed but 'node' command is not in the current session path."
}

# 6. Initialize Backend
Write-Step "Initializing backend (Go)..."
go mod download
go mod tidy
Write-Success "Backend initialized."

# 7. Initialize Frontend
if (Test-Path "frontend") {
    Write-Step "Initializing frontend (Node.js)..."
    Push-Location frontend
    npm install
    Pop-Location
    Write-Success "Frontend initialized."
}

# 8. Create necessary directories
Write-Step "Creating directories..."
if (-not (Test-Path "uploads")) {
    New-Item -ItemType Directory -Path "uploads" -Force | Out-Null
    Write-Success "Created 'uploads' directory."
} else {
    Write-Host "'uploads' directory already exists."
}

if (-not (Test-Path "bin")) {
    New-Item -ItemType Directory -Path "bin" -Force | Out-Null
    Write-Success "Created 'bin' directory."
} else {
    Write-Host "'bin' directory already exists."
}

# 9. Reminders
Write-Step "Post-initialization reminders:"
if (-not (Test-Path "wireguard.conf")) {
    Write-Warning-Host "Reminder: You may need a 'wireguard.conf' in the root directory if you plan to use VPN-based scraping."
    Write-Warning-Host "See WIREGUARD_SETUP.md for details."
}

Write-Host "`nInitialization Complete!" -ForegroundColor Green
Write-Host "If some commands were not found, please restart your terminal session."