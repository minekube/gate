---
title: 'Install Gate from Binaries - Direct Download Setup'
description: 'Download and install Gate Minecraft proxy binaries for Linux, Windows, and macOS. Quick setup with direct executable files.'
---

# Downloading Prebuilt Binaries

_The installation of Gate is ultra easy, NO Java needed!
Gate is only a single executable file ready to run the proxy._

## Quick Install

::: code-group

```sh [Linux/macOS]
curl -fsSL https://gate.minekube.com/install | bash

✨ Installing Gate...
⚡ Downloading Gate 0.42.2 for darwin-arm64...
✅ Checksum verified for gate_0.42.2_darwin_arm64
✨ Successfully installed Gate 0.42.2!
📍 Location: /Users/robin/.local/bin/gate
🚀 Run gate to start the proxy
```

```powershell [Windows]
powershell -c "irm https://gate.minekube.com/install.ps1 | iex"
```

:::

## Manual Download

If you prefer to download and install Gate manually:

1. Visit the <VPButton text="Releases" href="https://github.com/minekube/gate/releases/latest"/> page
2. Download the appropriate binary for your system
3. Verify the SHA256 checksum (recommended)
4. Place the binary in your preferred location

**Make Gate Executable** (Linux/macOS only)

```sh
chmod +x gate*
```

::: tip No Root Access?
You can still install Gate to your user directory:

```sh
mkdir -p ~/.local/bin
mv gate* ~/.local/bin/gate
# Add to PATH:
export PATH="$HOME/.local/bin:$PATH"
```

:::

## Installation Locations

- **Linux/macOS**: `~/.local/bin/gate`
- **Windows**: `%LOCALAPPDATA%\Gate\bin\gate.exe`

Both locations are in user space and don't require administrative privileges.

## Uninstalling Gate

To uninstall Gate, simply remove the binary:

::: code-group

```sh [Linux/macOS]
rm ~/.local/bin/gate
```

```powershell [Windows]
Remove-Item "$env:LOCALAPPDATA\Gate\bin\gate.exe"
```

:::

## Troubleshooting

If you encounter any issues:

1. **PATH not set**:

   - The scripts will provide commands to add Gate to your PATH
   - Follow the on-screen instructions after installation

2. **Checksum Verification Failed**:

   - This is a security feature ensuring the downloaded binary matches the official release
   - Try running the installation again
   - If it persists, please report it on our GitHub issues

3. **Permission Denied**:
   - Our scripts install to user space and shouldn't require elevated privileges
   - If you see permission errors, ensure you have write access to the installation directory

## Security Notes

- Our installation scripts are transparent and open source
- They only download from official GitHub releases
- All binaries are verified using SHA256 checksums
- No system-wide changes are made without your permission
- Installation is contained within your user directory
- No data collection or tracking

### What the Installation Scripts Do

Our installation scripts are designed to be transparent and secure. Here's exactly what they do:

1. **Safety First**:

   - Installs to user space (Linux/macOS: `~/.local/bin`, Windows: `%LOCALAPPDATA%\Gate\bin`)
   - Downloads only from official GitHub releases
   - Verifies file integrity using SHA256 checksums

2. **Installation Steps**:

   - Detects system architecture (amd64/arm64)
   - Creates installation directory if it doesn't exist
   - Downloads the appropriate Gate binary
   - Verifies the checksum to ensure file integrity
   - Makes the binary executable (Linux/macOS only)
   - Provides clear PATH setup instructions

3. **No System Changes**:
   - Only writes to your user directory
   - Suggests PATH changes but doesn't modify system files
   - Can be easily uninstalled by removing the binary

### Verifying the Scripts

Both installation scripts are open source and can be inspected:

- Unix: [View install script](https://github.com/minekube/gate/blob/master/.web/docs/public/install)
- Windows: [View install.ps1 script](https://github.com/minekube/gate/blob/master/.web/docs/public/install.ps1)
