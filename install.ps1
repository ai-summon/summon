$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

$Repo = "ai-summon/summon"
$InstallDir = if ($env:SUMMON_INSTALL_DIR) { $env:SUMMON_INSTALL_DIR } else { Join-Path $HOME ".summon" }
$BinDir = Join-Path $InstallDir "bin"

function Get-SummonArch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    # Handle WOW64: 32-bit PowerShell on 64-bit Windows
    if ($arch -eq "x86" -and $env:PROCESSOR_ARCHITEW6432) {
        $arch = $env:PROCESSOR_ARCHITEW6432
    }
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "Error: Unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    if ($env:SUMMON_VERSION) {
        return $env:SUMMON_VERSION
    }
    $url = "https://api.github.com/repos/$Repo/releases/latest"
    $response = Invoke-RestMethod -Uri $url -UseBasicParsing
    if (-not $response.tag_name) {
        throw "Error: Failed to determine latest version from GitHub API."
    }
    return $response.tag_name
}

function Test-Checksum {
    param(
        [string]$ArchivePath,
        [string]$ChecksumsPath
    )
    $archiveName = Split-Path -Leaf $ArchivePath
    $checksumLines = Get-Content $ChecksumsPath
    $matchLine = $checksumLines | Where-Object { $_ -match $archiveName }
    if (-not $matchLine) {
        throw "Error: No checksum found for $archiveName in checksums.txt."
    }
    $expected = ($matchLine -split '\s+')[0]
    $actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) {
        throw "Error: Checksum verification failed! The downloaded file may have been tampered with."
    }
}

function Add-ToPath {
    if ($env:SUMMON_NO_MODIFY_PATH) {
        return
    }
    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -and $currentPath.Split(';') -contains $BinDir) {
        return
    }
    $newPath = if ($currentPath) { "$BinDir;$currentPath" } else { $BinDir }
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
}

$TempDir = $null
try {
    $Arch = Get-SummonArch
    Write-Host "Detecting platform: windows/$Arch"

    $TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

    $Version = Get-LatestVersion
    Write-Host "Installing summon $Version"

    $Archive = "summon-windows-${Arch}.zip"
    $DownloadBase = if ($env:SUMMON_DOWNLOAD_BASE) { $env:SUMMON_DOWNLOAD_BASE } else { "https://github.com/$Repo/releases/download" }
    $DownloadUrl = "$DownloadBase/$Version/$Archive"
    $ChecksumsUrl = "$DownloadBase/$Version/checksums.txt"

    $ArchivePath = Join-Path $TempDir $Archive
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"

    Write-Host "Downloading $Archive..."
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ArchivePath -UseBasicParsing
    Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath -UseBasicParsing

    Write-Host "Verifying checksum..."
    Test-Checksum -ArchivePath $ArchivePath -ChecksumsPath $ChecksumsPath

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    Expand-Archive -Path $ArchivePath -DestinationPath $BinDir -Force

    Add-ToPath

    Write-Host ""
    Write-Host "summon $Version installed successfully to $(Join-Path $BinDir 'summon.exe')"
    if ($env:SUMMON_NO_MODIFY_PATH) {
        Write-Host "Add the following to your PATH: $BinDir"
    } else {
        Write-Host "Restart your terminal for PATH changes to take effect."
    }
} catch {
    Write-Error $_.Exception.Message
    exit 1
} finally {
    if ($TempDir -and (Test-Path $TempDir)) {
        Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
    }
}
