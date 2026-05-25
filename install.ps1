# PrismCLI Installer for Windows

$InstallDir = Join-Path $HOME ".prism"
$ExePath = Join-Path $InstallDir "prism.exe"
$Url = "https://github.com/brnalemusic/PrismCLI/releases/latest/download/prism.exe"

# Create installation directory
if (-not (Test-Path $InstallDir)) {
    Write-Host "Creating installation directory: $InstallDir" -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

# Download the binary
Write-Host "Downloading PrismCLI from $Url..." -ForegroundColor Cyan
try {
    Invoke-WebRequest -Uri $Url -OutFile $ExePath -UseBasicParsing
} catch {
    Write-Error "Failed to download PrismCLI. Please check your internet connection or the repository status."
    exit 1
}

# Add to PATH if not already present
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    Write-Host "Adding $InstallDir to User PATH..." -ForegroundColor Cyan
    $NewPath = "$UserPath;$InstallDir"
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    $env:Path = "$env:Path;$InstallDir" # Update current session
    Write-Host "PATH updated. You may need to restart your terminal." -ForegroundColor Yellow
}

Write-Host "`nPrismCLI installed successfully!" -ForegroundColor Green
Write-Host "Try running: prism --help"
