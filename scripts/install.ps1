$ErrorActionPreference = "Stop"

$SummonVersion = if ($env:SUMMON_VERSION) { $env:SUMMON_VERSION } else { "latest" }
$SummonInstallPath = $env:SUMMON_INSTALL_PATH
$SummonNonInteractive = if ($env:SUMMON_NONINTERACTIVE) { $env:SUMMON_NONINTERACTIVE } else { "0" }
$SummonNoModifyPath = if ($env:SUMMON_NO_MODIFY_PATH) { $env:SUMMON_NO_MODIFY_PATH } else { "0" }
$SummonDownloadUrl = $env:SUMMON_DOWNLOAD_URL
$SummonChecksumUrl = $env:SUMMON_CHECKSUM_URL
$SummonChecksum = $env:SUMMON_CHECKSUM
$SummonLatestVersion = if ($env:SUMMON_LATEST_VERSION) { $env:SUMMON_LATEST_VERSION } else { "v0.1.0" }
$SummonReleaseBaseUrl = if ($env:SUMMON_RELEASE_BASE_URL) { $env:SUMMON_RELEASE_BASE_URL } else { "https://github.com/ai-summon/summon/releases/download" }

if (-not $env:SUMMON_NONINTERACTIVE -and -not [Environment]::UserInteractive) {
    $SummonNonInteractive = "1"
}

function Fail-Installer([string]$Category, [string]$Message) {
    Write-Error "ERROR[$Category]: $Message"
    exit 1
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
    if ($SummonNoModifyPath -eq "1") {
        Write-Output "PATH update skipped (SUMMON_NO_MODIFY_PATH=1)."
        Write-Output "Add this path manually: $BinDir"
        return
    }

    if ($env:PATH -split ';' | Where-Object { $_ -eq $BinDir }) {
        Write-Output "PATH already includes $BinDir"
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
        Write-Output "Updated user PATH."
    }
    catch {
        Write-Output "Could not update PATH automatically."
        Write-Output "Run manually: setx Path \"$BinDir;%Path%\""
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

$script:TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $script:TmpDir -Force | Out-Null
try {
    $downloadUrl = Resolve-DownloadUrl -Version $Version -OsName $OsName -ArchName $ArchName
    $artifactPath = Join-Path $script:TmpDir "artifact"
    $artifactName = "summon_$($Version.TrimStart('v'))_${OsName}_${ArchName}.zip"

    Write-Output "Installing summon $Version for $OsName/$ArchName"
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

    Write-Output "Installed summon at: $TargetPath"
    Write-Output "Verify with: $TargetPath --version"
    Write-Output "Upgrade by rerunning this installer command."

    $null = $SummonNonInteractive
}
finally {
    if (Test-Path $script:TmpDir) {
        Remove-Item -Path $script:TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
