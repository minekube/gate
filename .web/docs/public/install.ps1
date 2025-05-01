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
$DEFAULT_INSTALL_DIR = if ($IsWindows -or (!$IsLinux -and !$IsMacOS -and [System.Environment]::OSVersion.Platform -eq "Win32NT")) {
    "$env:LOCALAPPDATA\Gate\bin"
} else {
    "$HOME/.gate/bin"
}
$INSTALL_DIR = if ($env:GATE_INSTALL_DIR) { $env:GATE_INSTALL_DIR } else { $DEFAULT_INSTALL_DIR }
$TEMP_DIR = if ($IsWindows -or (!$IsLinux -and !$IsMacOS -and [System.Environment]::OSVersion.Platform -eq "Win32NT")) {
    "$env:TEMP\gate-installer"
} else {
    "/tmp/gate-installer"
}
$REQUIRED_SPACE_MB = 50
$IS_WINDOWS = $IsWindows -or (!$IsLinux -and !$IsMacOS -and [System.Environment]::OSVersion.Platform -eq "Win32NT")

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

# These environment management functions are adapted from Bun's installation script
function Add-WindowsAPIType {
    if (-not $IS_WINDOWS) {
        return $false
    }

    try {
        if (-not ("Win32.NativeMethods" -as [Type])) {
            Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(
    IntPtr hWnd, 
    uint Msg, 
    UIntPtr wParam, 
    string lParam, 
    uint fuFlags, 
    uint uTimeout, 
    out UIntPtr lpdwResult
);
"@
        }
        return $true
    } catch {
        Write-WarningMsg "Could not load Windows API: $_"
        return $false
    }
}

function Broadcast-EnvironmentChange {
    # No-op on non-Windows
    if (-not $IS_WINDOWS) {
        return
    }

    if (Add-WindowsAPIType) {
        try {
            $HWND_BROADCAST = [IntPtr] 0xffff
            $WM_SETTINGCHANGE = 0x1a
            $result = [UIntPtr]::Zero
            
            # Broadcast environment change to all windows
            [Win32.NativeMethods]::SendMessageTimeout(
                $HWND_BROADCAST, 
                $WM_SETTINGCHANGE, 
                [UIntPtr]::Zero, 
                "Environment", 
                2, 
                5000, 
                [ref] $result
            ) | Out-Null
        } catch {
            Write-WarningMsg "Unable to broadcast environment change: $_"
        }
    }

    # Always update current session no matter what
    $env:PATH = "$INSTALL_DIR;$env:PATH"
}

function Update-PathPermanently {
    # Update the current session in any case
    $env:PATH = "$INSTALL_DIR;$env:PATH"
    Write-Info "✅ Added Gate to PATH for current session"

    if ($IS_WINDOWS) {
        # Windows platform - use registry
        try {
            # Check if registry path exists
            if (-not (Test-Path -Path "HKCU:\Environment")) {
                Write-WarningMsg "Windows user environment registry key not found"
                return $false
            }

            # Get current PATH from registry
            $currentPath = (Get-ItemProperty -Path "HKCU:\Environment" -Name "Path" -ErrorAction SilentlyContinue).Path

            # Check if our path is already in there
            if ($currentPath -and ($currentPath -split ';') -contains $INSTALL_DIR) {
                Write-Info "✅ Gate already in PATH"
                # Still broadcast to ensure the current session is updated
                Broadcast-EnvironmentChange
                return $true
            }

            # Add to PATH and update registry
            $newPath = if ($currentPath) { "$INSTALL_DIR;$currentPath" } else { $INSTALL_DIR }
            Set-ItemProperty -Path "HKCU:\Environment" -Name "Path" -Value $newPath
            
            # Broadcast changes
            Broadcast-EnvironmentChange
            Write-Info "✅ Added Gate to PATH permanently"
            return $true
        } catch {
            Write-WarningMsg "Failed to update PATH permanently: $_"
            return $false
        }
    } else {
        # Non-Windows platform - add to shell profile
        try {
            # Determine shell profile file
            $shellProfile = ""
            if ($env:SHELL -like "*bash*") {
                $shellProfile = "$HOME/.bashrc"
            } elseif ($env:SHELL -like "*zsh*") {
                $shellProfile = "$HOME/.zshrc"
            } elseif (Test-Path "$HOME/.profile") {
                $shellProfile = "$HOME/.profile"
            }

            if ($shellProfile -and (Test-Path $shellProfile)) {
                # Check if already in profile
                $profileContent = Get-Content $shellProfile -Raw
                if ($profileContent -match [regex]::Escape("export PATH=`"$INSTALL_DIR`:\$PATH`"")) {
                    Write-Info "✅ Gate already in PATH (via $shellProfile)"
                    return $true
                }

                # Add to profile
                Add-Content -Path $shellProfile -Value "`n# Added by Gate installer`nexport PATH=`"$INSTALL_DIR`:\$PATH`""
                Write-Info "✅ Added Gate to PATH permanently (via $shellProfile)"
                return $true
            } else {
                Write-WarningMsg "Could not find appropriate shell profile to update"
                return $false
            }
        } catch {
            Write-WarningMsg "Failed to update shell profile: $_"
            return $false
        }
    }
}

function Test-CommandAvailability {
    param (
        [string]$Command,
        [string]$FullPath
    )
    
    # First try directly via PATH
    try {
        $null = Get-Command $Command -ErrorAction Stop
        return $true
    } catch {
        # Try with refreshed PATH if on Windows
        if ($IS_WINDOWS) {
            try {
                $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "User") + ";" + 
                            [System.Environment]::GetEnvironmentVariable("PATH", "Machine")
                
                $null = Get-Command $Command -ErrorAction Stop
                return $true
            } catch {
                # Fall through to direct execution test
            }
        }
        
        # Try running directly as last resort
        try {
            if (Test-Path $FullPath) {
                & $FullPath "--version" | Out-Null
                if ($LASTEXITCODE -eq 0 -or $LASTEXITCODE -eq $null) {
                    return $true
                }
            }
            return $false
        } catch {
            return $false
        }
    }
}

function Detect-Arch {
    # Try primary detection method
    try {
        $arch = [Environment]::ProcessorArchitecture.ToString()
        switch ($arch) {
            "X86" { return "386" }
            "AMD64" { return "amd64" }
            "ARM64" { return "arm64" }
            default { throw "Unrecognized architecture: $arch" }
        }
    }
    catch {
        # Fallback detection method
        try {
            $is64Bit = [System.Environment]::Is64BitOperatingSystem
            
            if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") {
                return "arm64"
            }
            elseif ($is64Bit) {
                return "amd64"
            }
            else {
                return "386"
            }
        }
        catch {
            Write-ErrorMsg "Failed to detect system architecture: $_"
        }
    }
}

function Detect-OS {
    if ($IS_WINDOWS) {
        return "windows"
    } elseif ($IsLinux -or (Get-Command uname -ErrorAction SilentlyContinue)) {
        return "linux"
    } elseif ($IsMacOS) {
        return "darwin"
    } else {
        # Fallback detection
        $os = [System.Runtime.InteropServices.RuntimeInformation]::OSDescription
        if ($os -like "*Windows*") {
            return "windows"
        } elseif ($os -like "*Linux*") {
            return "linux"
        } elseif ($os -like "*Darwin*") {
            return "darwin"
        } else {
            Write-ErrorMsg "Unsupported OS: $os"
        }
    }
}

function Check-DiskSpace {
    param([string]$Path)

    try {
        if ($IS_WINDOWS) {
            # Windows method
            $drive = (Get-Item $Path).PSDrive.Name
            $freeSpace = (Get-PSDrive -Name $drive).Free / 1MB

            if ($freeSpace -lt $REQUIRED_SPACE_MB) {
                Write-ErrorMsg "Insufficient disk space. Required: $REQUIRED_SPACE_MB MB, Available: $([math]::Round($freeSpace,2)) MB"
            }
        } else {
            # Linux/macOS method - try df command
            try {
                $dfOutput = Invoke-Expression "df -m '$Path' | tail -1"
                $freeSpace = [int]($dfOutput -split '\s+')[3]
                
                if ($freeSpace -lt $REQUIRED_SPACE_MB) {
                    Write-ErrorMsg "Insufficient disk space. Required: $REQUIRED_SPACE_MB MB, Available: $freeSpace MB"
                }
            } catch {
                Write-WarningMsg "Could not check disk space: $_"
                # Continue anyway
            }
        }
    } catch {
        Write-WarningMsg "Error checking disk space: $_"
        # Continue anyway
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
        $errorDetails = $_.Exception.Message
        $statusCode = if ($_.Exception.Response) { $_.Exception.Response.StatusCode.value__ } else { "Unknown" }
        
        Write-ErrorMsg "Failed to download from $Url
Error details: $errorDetails
Status code: $statusCode
Please check your internet connection and try again.
If the problem persists, the server might be temporarily unavailable or the URL might be incorrect."
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
        $actual_checksum = $hash.Hash.ToLower()
        
        # Extract just the hash part if it contains a path or other prefix
        if ($expected_checksum -match '([a-fA-F0-9]{64})') {
            $expected_checksum = $matches[1].ToLower()
        } else {
            $expected_checksum = $expected_checksum.ToLower()
        }
    }
    catch {
        Write-ErrorMsg "Failed to compute checksum for $filename"
    }

    if ($expected_checksum -ne $actual_checksum) {
        Write-ErrorMsg "Checksum verification failed for $filename.`nExpected: $expected_checksum`nActual:   $actual_checksum"
    }

    Write-Info "✅ Checksum verified for $filename"
}

function Show-Success {
    param(
        [string]$Version,
        [string]$InstallPath,
        [bool]$PermanentPathSet,
        [string]$BinaryName
    )
    
    Write-Info "✨ Successfully installed Gate $Version!"
    Write-Host "📍 Location: $InstallPath" -ForegroundColor $YELLOW
    Write-Host ""
    
    # Test if gate binary is available in PATH immediately
    $gateWorks = Test-CommandAvailability -Command $BinaryName -FullPath $InstallPath
    
    if ($gateWorks) {
        Write-Info "🚀 Gate is ready! Run '$BinaryName' to start the proxy"
        Write-Host "   Type $BinaryName to start the server now" -ForegroundColor $GREEN
    } else {
        if ($IS_WINDOWS) {
            Write-WarningMsg "Gate is installed but not immediately available in this terminal session."
            Write-Host "   Please try opening a new terminal window and running: $BinaryName" -ForegroundColor $GREEN
        } else {
            # For Linux/macOS
            Write-WarningMsg "Gate is installed but you need to reload your shell configuration."
            Write-Host "   Run: source ~/.bashrc   (or your shell's equivalent)" -ForegroundColor $GREEN
            Write-Host "   Or open a new terminal window" -ForegroundColor $GREEN
        }
        
        Write-Host ""
        Write-Host "   For now, you can run it using the full path:" -ForegroundColor $YELLOW
        Write-Host "   $InstallPath" -ForegroundColor $GREEN
    }
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
        $releaseInfo = Invoke-RestMethod -Uri $apiUrl -Method Get -Headers @{ "User-Agent" = "Minekube-Gate-Installer/PowerShell" }
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
    $DownloadedBinaryName = $BinaryName
    
    # Determine final executable name based on OS
    $ExecutableName = if ($OS -eq "windows") { "gate.exe" } else { "gate" }
    
    if ($OS -eq "windows") {
        $DownloadedBinaryName += ".exe"
    }

    $DOWNLOAD_URL = "https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/v$VERSION/$DownloadedBinaryName"
    $CHECKSUMS_URL = "https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/v$VERSION/checksums.txt"

    Write-Info "⚡ Downloading Gate $VERSION for $OS-$ARCH..."
    Write-Host "📥 From: $DOWNLOAD_URL" -ForegroundColor $YELLOW

    # Download binary
    $binaryPath = Join-Path $TEMP_DIR $DownloadedBinaryName
    Download-File -Url $DOWNLOAD_URL -Destination $binaryPath

    # Download checksums.txt
    $checksumsPath = Join-Path $TEMP_DIR "checksums.txt"
    Download-File -Url $CHECKSUMS_URL -Destination $checksumsPath

    # Verify checksum
    Verify-Checksum -File $binaryPath -ChecksumFile $checksumsPath

    # Move binary to installation path with correct name
    $finalPath = Join-Path $INSTALL_DIR $ExecutableName
    Move-Item -Path $binaryPath -Destination $finalPath -Force
    Write-Host "✔️ Moved $DownloadedBinaryName to $finalPath" -ForegroundColor $GREEN

    # Make the file executable if on Linux/macOS
    if ($OS -ne "windows") {
        try {
            # Try to make executable using chmod if available
            $chmodResult = Invoke-Expression "chmod +x '$finalPath'" 2>&1
            Write-Host "✔️ Made $ExecutableName executable" -ForegroundColor $GREEN
        } catch {
            Write-WarningMsg "Unable to set executable permission. You may need to run: chmod +x '$finalPath'"
        }
    }

    # Update PATH for current session and try to make it permanent
    $pathUpdated = Update-PathPermanently
    
    # Create command alias for the current session as additional failsafe
    Set-Alias -Name $ExecutableName.Replace(".exe", "") -Value $finalPath -Scope Global
    
    # Show success message
    Show-Success -Version $VERSION -InstallPath $finalPath -PermanentPathSet $pathUpdated -BinaryName $ExecutableName.Replace(".exe", "")
}

# Execute installation
Install-Gate
