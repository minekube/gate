# Install.ps1 - PowerShell installation script for Gate

# Exit script on errors
$ErrorActionPreference = "Stop"

# Colors for terminal output
$RED = "Red"
$GREEN = "Green"
$YELLOW = "Yellow"
$NC = "White" # No Color

# Constants
$REPO_OWNER = "minekube"
$REPO_NAME = "gate"
$DEFAULT_INSTALL_DIR = "$env:LOCALAPPDATA\Gate\bin"
$INSTALL_DIR = if ($env:GATE_INSTALL_DIR) { $env:GATE_INSTALL_DIR } else { $DEFAULT_INSTALL_DIR }
$TEMP_DIR = "$env:TEMP\gate-installer"
$REQUIRED_SPACE_MB = 50

# Functions

function Write-Info {
    param([string]$Message)
    Write-Host $Message -ForegroundColor $GREEN
}

function Write-WarningMsg {
    param([string]$Message)
    Write-Host $Message -ForegroundColor $YELLOW
}

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host $Message -ForegroundColor $RED
    exit 1
}

function Detect-Arch {
    $arch = [Environment]::ProcessorArchitecture.ToString()
    switch ($arch) {
        "X86" { return "386" }
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default {
            Write-ErrorMsg "Unsupported architecture: $arch"
        }
    }
}

function Detect-OS {
    $os = [System.Runtime.InteropServices.RuntimeInformation]::OSDescription
    if ($os -like "*Windows*") {
        return "windows"
    } else {
        Write-ErrorMsg "Unsupported OS: $os"
    }
}

function Check-DiskSpace {
    param([string]$Path)

    # Get free space in MB
    $drive = (Get-Item $Path).PSDrive.Name
    $freeSpace = (Get-PSDrive -Name $drive).Free / 1MB

    if ($freeSpace -lt $REQUIRED_SPACE_MB) {
        Write-ErrorMsg "Insufficient disk space. Required: $REQUIRED_SPACE_MB MB, Available: $([math]::Round($freeSpace,2)) MB"
    }
}

function Download-File {
    param(
        [string]$Url,
        [string]$Destination
    )

    try {
        Invoke-WebRequest -Uri $Url -OutFile $Destination -ErrorAction Stop
    }
    catch {
        Write-ErrorMsg "Failed to download from $Url"
    }
}

function Verify-Checksum {
    param(
        [string]$File,
        [string]$ChecksumFile
    )

    $filename = [System.IO.Path]::GetFileName($File)
    $expected_checksum = Select-String -Path $ChecksumFile -Pattern $filename | ForEach-Object { ($_ -split '\s+')[0] }

    if (-not $expected_checksum) {
        Write-ErrorMsg "Checksum for $filename not found in $ChecksumFile"
    }

    try {
        $hash = Get-FileHash -Path $File -Algorithm SHA256
        $actual_checksum = $hash.Hash
    }
    catch {
        Write-ErrorMsg "Failed to compute checksum for $filename"
    }

    if ($expected_checksum -ne $actual_checksum) {
        Write-ErrorMsg "Checksum verification failed for $filename.`nExpected: $expected_checksum`nActual:   $actual_checksum"
    }

    Write-Info "✅ Checksum verified for $filename"
}

function Update-PATHSession {
    $env:PATH = "$INSTALL_DIR;$env:PATH"
}

function Add-PATHTemporarily {
    Write-Info "✨ Successfully installed Gate $VERSION!"
    Write-Host "📍 Location: $INSTALL_PATH" -ForegroundColor $YELLOW
    Write-Host ""

    Write-WarningMsg "To use Gate, run this command now:"
    Write-Host "  \$env:PATH = `"$INSTALL_DIR;`$env:PATH`"" -ForegroundColor $GREEN
    Write-Host ""

    Write-WarningMsg "Or add it permanently by running the following commands:"
    Write-Host "  [Environment]::SetEnvironmentVariable('PATH', `"$INSTALL_DIR;`$env:PATH`", 'User')" -ForegroundColor $GREEN
    Write-Host "  Reload your PowerShell session or restart your terminal." -ForegroundColor $GREEN
    Write-Host ""

    Write-Info "🚀 Run gate to start the proxy"
}

# Main Installation Function

function Install-Gate {
    Write-Info "✨ Installing Gate..."

    # Create installation and temp directories
    New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
    New-Item -ItemType Directory -Path $TEMP_DIR -Force | Out-Null

    # Check disk space
    Check-DiskSpace -Path $INSTALL_DIR

    # Detect OS and architecture
    $OS = Detect-OS
    $ARCH = Detect-Arch

    # Fetch the latest version from GitHub API
    Write-Info "📡 Fetching the latest release information..."
    try {
        $apiUrl = "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest"
        $releaseInfo = Invoke-RestMethod -Uri $apiUrl -Method Get -Headers @{ "User-Agent" = "PowerShell" }
        $VERSION = $releaseInfo.tag_name -replace '^v', ''
    }
    catch {
        Write-ErrorMsg "Failed to fetch release information from GitHub."
    }

    if (-not $VERSION) {
        Write-ErrorMsg "Failed to detect the latest version."
    }

    # Set download URLs
    $BinaryName = "gate_${VERSION}_${OS}_${ARCH}"
    if ($OS -eq "windows") {
        $BinaryName += ".exe"
    }

    $DOWNLOAD_URL = "https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/v$VERSION/$BinaryName"
    $CHECKSUMS_URL = "https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/v$VERSION/checksums.txt"

    Write-Info "⚡ Downloading Gate $VERSION for $OS-$ARCH..."
    Write-Host "📥 From: $DOWNLOAD_URL" -ForegroundColor $YELLOW

    # Download binary
    $binaryPath = Join-Path $TEMP_DIR $BinaryName
    Download-File -Url $DOWNLOAD_URL -Destination $binaryPath

    # Download checksums.txt
    $checksumsPath = Join-Path $TEMP_DIR "checksums.txt"
    Download-File -Url $CHECKSUMS_URL -Destination $checksumsPath

    # Verify checksum
    Verify-Checksum -File $binaryPath -ChecksumFile $checksumsPath

    # Move binary to installation path
    $finalPath = Join-Path $INSTALL_DIR "gate.exe"
    Move-Item -Path $binaryPath -Destination $finalPath -Force
    Write-Host "✔️ Moved gate.exe to $finalPath" -ForegroundColor $GREEN

    # Update PATH for current session
    Update-PATHSession

    # Final messages and PATH instructions
    Add-PATHTemporarily
}

# Execute installation
Install-Gate
