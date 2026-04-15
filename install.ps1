$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

$Repo = "ai-summon/summon"

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

# Resolve the install directory using XDG conventions (matching UV's logic).
# Priority: $SUMMON_INSTALL_DIR > $XDG_BIN_HOME > $XDG_DATA_HOME\..\bin > $HOME\.local\bin
function Resolve-InstallDir {
    if ($env:SUMMON_INSTALL_DIR) {
        return $env:SUMMON_INSTALL_DIR
    }
    if ($env:XDG_BIN_HOME) {
        return $env:XDG_BIN_HOME
    }
    if ($env:XDG_DATA_HOME) {
        return Join-Path $env:XDG_DATA_HOME "../bin"
    }
    return Join-Path $HOME ".local/bin"
}

# Add install dir to CI PATH ($GITHUB_PATH) if available.
function Add-CiPath($PathToAdd) {
    if ($env:GITHUB_PATH) {
        Write-Output "$PathToAdd" | Out-File -FilePath "$($env:GITHUB_PATH)" -Encoding utf8 -Append
    }
}

# Add install dir to user PATH via Windows registry (matching UV's approach).
# Returns $true if the registry was modified, $false if already present.
function Add-ToPath {
    if ($env:SUMMON_NO_MODIFY_PATH) {
        return
    }

    Add-CiPath $BinDir

    $RegistryPath = 'registry::HKEY_CURRENT_USER\Environment'

    # Read the unexpanded value to avoid resolving %VARS%
    $CurrentDirectories = (Get-Item -LiteralPath $RegistryPath).GetValue('Path', '', 'DoNotExpandEnvironmentNames') -split ';' -ne ''

    if ($BinDir -in $CurrentDirectories) {
        return
    }

    # Prepend install dir to PATH
    $NewPath = (,$BinDir + $CurrentDirectories) -join ';'
    Set-ItemProperty -Type ExpandString -LiteralPath $RegistryPath Path $NewPath

    # Broadcast WM_SETTINGCHANGE to reload environment in open shells
    $DummyName = 'summon-' + [guid]::NewGuid().ToString()
    [Environment]::SetEnvironmentVariable($DummyName, 'summon-dummy', 'User')
    [Environment]::SetEnvironmentVariable($DummyName, [NullString]::value, 'User')

    Write-Host ""
    Write-Host "To add $BinDir to your PATH, either restart your shell or run:"
    Write-Host ""
    Write-Host "    set Path=$BinDir;%Path%   (cmd)"
    Write-Host "    `$env:Path = `"$BinDir;`$env:Path`"   (powershell)"
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

    $BinDir = Resolve-InstallDir
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    Expand-Archive -Path $ArchivePath -DestinationPath $BinDir -Force

    Add-ToPath

    Write-Host ""
    Write-Host "summon $Version installed successfully to $(Join-Path $BinDir 'summon.exe')"
    if ($env:SUMMON_NO_MODIFY_PATH) {
        Write-Host "Add the following to your PATH: $BinDir"
    }
} catch {
    Write-Error $_.Exception.Message
    exit 1
} finally {
    if ($TempDir -and (Test-Path $TempDir)) {
        Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
    }
}
