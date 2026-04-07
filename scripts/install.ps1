$ErrorActionPreference = "Stop"

$SummonVersion = if ($env:SUMMON_VERSION) { $env:SUMMON_VERSION } else { "latest" }
$SummonInstallPath = $env:SUMMON_INSTALL_PATH
$SummonNonInteractive = if ($env:SUMMON_NONINTERACTIVE) { $env:SUMMON_NONINTERACTIVE } else { "0" }
$SummonNoModifyPath = if ($env:SUMMON_NO_MODIFY_PATH) { $env:SUMMON_NO_MODIFY_PATH } else { "0" }
$SummonQuiet = if ($env:SUMMON_QUIET) { $env:SUMMON_QUIET } else { "0" }
$SummonDownloadUrl = $env:SUMMON_DOWNLOAD_URL
$SummonChecksumUrl = $env:SUMMON_CHECKSUM_URL
$SummonChecksum = $env:SUMMON_CHECKSUM
$SummonLatestVersion = if ($env:SUMMON_LATEST_VERSION) { $env:SUMMON_LATEST_VERSION } else { "v0.0.5" }
$SummonReleaseBaseUrl = if ($env:SUMMON_RELEASE_BASE_URL) { $env:SUMMON_RELEASE_BASE_URL } else { "https://github.com/ai-summon/summon/releases/download" }

if ($SummonNonInteractive -ne "1" -and -not [Environment]::UserInteractive) {
    $SummonNonInteractive = "1"
}

# Detect ANSI capability (PowerShell on Windows 10+ with VT support)
$script:UseStyle = $false
if ([Console]::IsOutputRedirected -eq $false -and [Console]::IsErrorRedirected -eq $false) {
    if ($PSVersionTable.PSVersion.Major -ge 7 -or $Host.UI.SupportsVirtualTerminal) {
        $script:UseStyle = $true
    }
}
if ($env:NO_COLOR) { $script:UseStyle = $false }

function Fail-Installer([string]$Category, [string]$Message) {
    if ($script:UseStyle) {
        Write-Error "$([char]27)[1;31merror[$Category]$([char]27)[0m: $Message"
    } else {
        Write-Error "ERROR[$Category]: $Message"
    }
    exit 1
}

function Write-Say([string]$Message) {
    if ($SummonQuiet -eq "1") { return }
    if ($script:UseStyle) {
        Write-Output "$([char]27)[1minfo$([char]27)[0m: $Message"
    } else {
        Write-Output "info: $Message"
    }
}

function Write-Info([string]$Message) {
    if ($SummonQuiet -eq "1") { return }
    Write-Output $Message
}

function Write-Bold([string]$Text) {
    if ($script:UseStyle) {
        return "$([char]27)[1m$Text$([char]27)[0m"
    }
    return $Text
}

function Detect-ExistingVersion([string]$TargetPath) {
    if (Test-Path -Path $TargetPath) {
        try {
            $result = & $TargetPath --version 2>$null | Select-Object -First 1
            if ($result) { return $result }
        } catch { }
    }
    return ""
}

function Show-WelcomeBanner {
    Write-Info ""
    if ($script:UseStyle) {
        Write-Output "$([char]27)[1mWelcome to Summon!$([char]27)[0m"
    } else {
        Write-Output "Welcome to Summon!"
    }
    Write-Info ""
    Write-Info "This will install the summon AI package manager."
}

function Show-InstallDetails([string]$Target, [string]$Existing) {
    Write-Info ""
    if ($Existing) {
        Write-Info "An existing installation was detected ($Existing)."
        Write-Info "The summon binary will be upgraded at:"
    } else {
        Write-Info "The summon binary will be installed at:"
    }
    Write-Info ""
    Write-Info "    $Target"
    Write-Info ""
    Write-Info "This can be changed with the SUMMON_INSTALL_PATH environment variable."
}

function Show-PathInfo([string]$BinDir) {
    Write-Info ""
    if ($SummonNoModifyPath -eq "1") {
        Write-Info "PATH modification is disabled (SUMMON_NO_MODIFY_PATH=1)."
        Write-Info "Add this path manually: $BinDir"
    } else {
        $pathEntries = $env:PATH -split ';'
        if ($pathEntries -contains $BinDir) {
            Write-Info "PATH already includes the install directory ($BinDir)."
        } else {
            Write-Info "The user PATH environment variable will be modified to include:"
            Write-Info ""
            Write-Info "    $BinDir"
        }
    }
}

function Show-UninstallInfo {
    Write-Info ""
    Write-Info "You can uninstall at any time with summon self uninstall and"
    Write-Info "these changes will be reverted."
}

function Show-OptionsSummary([string]$Version, [string]$OsName, [string]$ArchName, [string]$Target, [string]$ModifyPath) {
    Write-Info ""
    if ($script:UseStyle) {
        Write-Output "$([char]27)[1mCurrent installation options:$([char]27)[0m"
    } else {
        Write-Output "Current installation options:"
    }
    Write-Info ""
    Write-Output "        version: $(Write-Bold $Version)"
    Write-Output "       platform: $(Write-Bold "$OsName/$ArchName")"
    Write-Output "   install path: $(Write-Bold $Target)"
    Write-Output "    modify PATH: $(Write-Bold $ModifyPath)"
}

function Show-PreInstallSummary([string]$Version, [string]$OsName, [string]$ArchName, [string]$Target, [string]$Existing) {
    $binDir = Split-Path -Parent $Target
    $modifyPathLabel = if ($SummonNoModifyPath -eq "1") { "no" } else { "yes" }
    Show-WelcomeBanner
    Show-InstallDetails -Target $Target -Existing $Existing
    Show-PathInfo -BinDir $binDir
    Show-UninstallInfo
    Show-OptionsSummary -Version $Version -OsName $OsName -ArchName $ArchName -Target $Target -ModifyPath $modifyPathLabel
}

function Show-Menu {
    if ($SummonQuiet -ne "1") {
        Write-Host ""
        Write-Host "1) Proceed with standard installation (default - just press enter)"
        Write-Host "2) Customize installation"
        Write-Host "3) Cancel installation"
    }
    $choice = Read-Host ">"
    return $choice
}

function Show-PostInstallSummary([string]$Target, [bool]$PathModified) {
    $binDir = Split-Path -Parent $Target
    Write-Info ""
    if ($script:UseStyle) {
        Write-Output "$([char]27)[1msummon is installed now. Great!$([char]27)[0m"
    } else {
        Write-Output "summon is installed now. Great!"
    }

    if ($PathModified) {
        Write-Info ""
        Write-Info "To get started you may need to restart your current shell."
        Write-Info "This would reload your PATH environment variable to include"
        Write-Info "the summon install directory ($binDir)."
    }

    Write-Info ""
    Write-Info "To verify your installation:"
    Write-Info ""
    Write-Info "    summon --version"
    Write-Info ""
    Write-Info "To upgrade summon, rerun this installer."
    Write-Info "To uninstall, run: summon self uninstall"
}

function Get-NormalizedOs {
    if ($env:SUMMON_TEST_OS) {
        $raw = $env:SUMMON_TEST_OS.ToLowerInvariant()
    } else {
        $raw = "windows"
    }

    if ($raw -eq "windows" -or $raw -eq "windows_nt") {
        return "windows"
    }

    Fail-Installer "platform" "Unsupported operating system: $raw. Supported: Windows."
}

function Get-NormalizedArch {
    if ($env:SUMMON_TEST_ARCH) {
        $raw = $env:SUMMON_TEST_ARCH.ToLowerInvariant()
    } elseif ($env:PROCESSOR_ARCHITECTURE) {
        $raw = $env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()
    } else {
        $raw = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    }
    switch ($raw) {
        "amd64" { return "amd64" }
        "x86_64" { return "amd64" }
        "arm64" { return "arm64" }
        "x64" { return "amd64" }
        default { Fail-Installer "platform" "Unsupported architecture: $raw. Supported: amd64 and arm64." }
    }
}

function Resolve-Version {
    if ($SummonVersion -ne "latest") {
        return $SummonVersion
    }

    $apiUrl = if ($env:SUMMON_TEST_API_URL) { $env:SUMMON_TEST_API_URL } else { "https://api.github.com/repos/ai-summon/summon/releases/latest" }
    try {
        $response = Invoke-RestMethod -Uri $apiUrl -TimeoutSec 10
        if ($response.tag_name) {
            return $response.tag_name
        }
    } catch {
        # Fall through to fallback
    }

    Write-Warning "Could not determine latest version from GitHub API. Using fallback: $SummonLatestVersion"
    return $SummonLatestVersion
}

function Resolve-TargetPath {
    if ($SummonInstallPath) {
        return $SummonInstallPath
    }
    return Join-Path $HOME ".summon\bin\summon.exe"
}

function Ensure-WritableTarget([string]$TargetPath) {
    $parent = Split-Path -Parent $TargetPath
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
    if (-not (Test-Path -Path $parent -PathType Container)) {
        Fail-Installer "permission" "Install directory is not writable: $parent"
    }
}

function Download-File([string]$Url, [string]$OutputPath) {
    if ($env:SUMMON_TEST_DISABLE_DOWNLOAD_TOOL -eq "1") {
        Fail-Installer "download" "Missing download tool. Install curl or wget."
    }

    if ($Url.StartsWith("file://")) {
        if ($env:SUMMON_TEST_ALLOW_INSECURE_URLS -ne "1") {
            Fail-Installer "download" "Only HTTPS URLs are allowed for downloads."
        }

        try {
            $uri = [System.Uri]::new($Url)
            if (-not $uri.IsFile) {
                Fail-Installer "download" "Invalid file URI: $Url"
            }
            $sourcePath = $uri.LocalPath
            if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) {
                Fail-Installer "download" "Local test artifact not found: $sourcePath"
            }
            Copy-Item -LiteralPath $sourcePath -Destination $OutputPath -Force
            return
        }
        catch {
            Fail-Installer "download" "Failed to read local test artifact: $Url"
        }
    }

    if (-not $Url.StartsWith("https://")) {
        if ($env:SUMMON_TEST_ALLOW_INSECURE_URLS -eq "1") {
            Fail-Installer "download" "Only HTTPS URLs are allowed for downloads (test override permits file://)."
        }
        Fail-Installer "download" "Only HTTPS URLs are allowed for downloads."
    }

    try {
        Invoke-WebRequest -Uri $Url -OutFile $OutputPath -MaximumRetryCount 3 -RetryIntervalSec 1
    }
    catch {
        Fail-Installer "download" "Failed to download artifact: $Url"
    }
}

function Resolve-DownloadUrl([string]$Version, [string]$OsName, [string]$ArchName) {
    if ($SummonDownloadUrl) {
        return $SummonDownloadUrl
    }

    $artifact = "summon_$($Version.TrimStart('v'))_${OsName}_${ArchName}.zip"
    return "$SummonReleaseBaseUrl/$Version/$artifact"
}

function Resolve-Checksum([string]$Version, [string]$ArtifactName) {
    if ($SummonChecksum) {
        return $SummonChecksum
    }

    if ($SummonChecksumUrl) {
        $manifestFile = Join-Path $script:TmpDir "checksums.txt"
        Download-File -Url $SummonChecksumUrl -OutputPath $manifestFile
        $line = Get-Content $manifestFile | Where-Object { $_ -match " $ArtifactName$" } | Select-Object -First 1
        if (-not $line) {
            Fail-Installer "checksum" "Checksum entry not found for $ArtifactName"
        }
        return ($line -split "\s+")[0]
    }

    $versionNoV = $Version.TrimStart('v')
    $manifestUrl = "$SummonReleaseBaseUrl/$Version/summon_${versionNoV}_checksums.txt"
    $manifestFile = Join-Path $script:TmpDir "checksums.txt"
    Download-File -Url $manifestUrl -OutputPath $manifestFile
    $line = Get-Content $manifestFile | Where-Object { $_ -match " $ArtifactName$" } | Select-Object -First 1
    if (-not $line) {
        Fail-Installer "checksum" "Checksum entry not found for $ArtifactName in manifest"
    }
    return ($line -split "\s+")[0]
}

function Expand-Artifact([string]$ArtifactPath) {
    $dest = Join-Path $script:TmpDir "summon.exe"
    if ($ArtifactPath.EndsWith(".zip")) {
        Expand-Archive -Path $ArtifactPath -DestinationPath $script:TmpDir -Force
        if (-not (Test-Path $dest)) {
            Fail-Installer "download" "Archive did not contain summon.exe"
        }
    }
    else {
        Copy-Item -Path $ArtifactPath -Destination $dest -Force
    }
    return $dest
}

function Update-PathIfNeeded([string]$BinDir) {
    $script:PathWasModified = $false

    if ($SummonNoModifyPath -eq "1") {
        Write-Say "PATH update skipped (SUMMON_NO_MODIFY_PATH=1)."
        Write-Info "Add this path manually: $BinDir"
        return
    }

    if ($env:PATH -split ';' | Where-Object { $_ -eq $BinDir }) {
        Write-Say "PATH already includes $BinDir"
        return
    }

    try {
        $current = [Environment]::GetEnvironmentVariable("Path", "User")
        if ([string]::IsNullOrEmpty($current)) {
            [Environment]::SetEnvironmentVariable("Path", $BinDir, "User")
        }
        else {
            [Environment]::SetEnvironmentVariable("Path", "$BinDir;$current", "User")
        }
        Write-Say "Updated user PATH."
        $script:PathWasModified = $true
    }
    catch {
        Write-Say "Could not update PATH automatically."
        Write-Info "Run manually: setx Path `"$BinDir;%Path%`""
    }
}

function Warn-IfShadowed([string]$TargetPath) {
    $command = Get-Command summon -ErrorAction SilentlyContinue
    if ($command -and $command.Source -ne $TargetPath) {
        Write-Warning "Another summon binary appears earlier in PATH: $($command.Source)"
    }
}

$OsName = Get-NormalizedOs
$ArchName = Get-NormalizedArch
$Version = Resolve-Version
$TargetPath = Resolve-TargetPath

Ensure-WritableTarget -TargetPath $TargetPath

# Detect existing installation
$existingVersion = Detect-ExistingVersion -TargetPath $TargetPath

# Show welcome banner and interactive menu
if ($SummonNonInteractive -ne "1") {
    Show-PreInstallSummary -Version $Version -OsName $OsName -ArchName $ArchName -Target $TargetPath -Existing $existingVersion

    $menuDone = $false
    while (-not $menuDone) {
        $choice = Show-Menu
        switch ($choice) {
            { $_ -eq "1" -or $_ -eq "" } {
                $menuDone = $true
            }
            "2" {
                Write-Info ""
                $newPath = Read-Host "Enter install path [$TargetPath]"
                if ($newPath) {
                    $TargetPath = $newPath
                    $SummonInstallPath = $TargetPath
                }
                $modifyChoice = Read-Host "Modify PATH? [Y/n]"
                if ($modifyChoice -match '^[nN]') {
                    $SummonNoModifyPath = "1"
                } else {
                    $SummonNoModifyPath = "0"
                }
                Ensure-WritableTarget -TargetPath $TargetPath
                $existingVersion = Detect-ExistingVersion -TargetPath $TargetPath
                Show-PreInstallSummary -Version $Version -OsName $OsName -ArchName $ArchName -Target $TargetPath -Existing $existingVersion
            }
            "3" {
                Write-Info ""
                Write-Info "Installation cancelled."
                exit 0
            }
            default {
                Write-Info "Invalid option. Please select 1, 2, or 3."
            }
        }
    }
}

$script:TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
$script:PathWasModified = $false
New-Item -ItemType Directory -Path $script:TmpDir -Force | Out-Null
try {
    $downloadUrl = Resolve-DownloadUrl -Version $Version -OsName $OsName -ArchName $ArchName
    $artifactPath = Join-Path $script:TmpDir "artifact"
    $artifactName = "summon_$($Version.TrimStart('v'))_${OsName}_${ArchName}.zip"

    Write-Say "Installing summon $Version for $OsName/$ArchName"
    Download-File -Url $downloadUrl -OutputPath $artifactPath

    $expected = Resolve-Checksum -Version $Version -ArtifactName $artifactName
    if ($env:SUMMON_TEST_DISABLE_HASH_TOOL -eq "1") {
        Fail-Installer "checksum" "Missing checksum tool. Install shasum or sha256sum."
    }
    $actual = (Get-FileHash -Path $artifactPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expected.ToLowerInvariant() -ne $actual) {
        Fail-Installer "checksum" "Checksum mismatch. Expected $expected got $actual"
    }

    $expanded = Expand-Artifact -ArtifactPath $artifactPath
    Move-Item -Path $expanded -Destination $TargetPath -Force

    $binDir = Split-Path -Parent $TargetPath
    Update-PathIfNeeded -BinDir $binDir
    Warn-IfShadowed -TargetPath $TargetPath

    if ($SummonNonInteractive -ne "1") {
        Show-PostInstallSummary -Target $TargetPath -PathModified $script:PathWasModified
    } else {
        Write-Say "Installed summon at: $TargetPath"
        Write-Info "Verify with: $TargetPath --version"
        Write-Info "Upgrade by rerunning this installer command."
    }
}
finally {
    if (Test-Path $script:TmpDir) {
        Remove-Item -Path $script:TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
