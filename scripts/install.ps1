[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$SourcePath,
    [string]$InstallDir,
    [switch]$NoPath
)

$ErrorActionPreference = "Stop"
$releaseAPI = "https://api.github.com/repos/xy200303/MobBase/releases"

function Write-InstallError([string]$Message) {
    Write-Error "Mob installation failed: $Message"
    exit 1
}

function Add-UserPath([string]$Directory) {
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @()
    if (-not [string]::IsNullOrWhiteSpace($current)) {
        $entries = @($current -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    }
    $exists = $entries | Where-Object { $_.TrimEnd("\") -ieq $Directory.TrimEnd("\") }
    if (-not $exists) {
        [Environment]::SetEnvironmentVariable("Path", (@($entries + $Directory) -join ";"), "User")
    }
    if (($env:Path -split ";" | Where-Object { $_.TrimEnd("\") -ieq $Directory.TrimEnd("\") }).Count -eq 0) {
        $env:Path = "$Directory;$env:Path"
    }
}

function Resolve-ReleaseTag([string]$RequestedVersion) {
    if ($RequestedVersion -ne "latest") {
        return $RequestedVersion
    }
    try {
        $release = Invoke-RestMethod -Uri "$releaseAPI/latest" -Headers @{ "User-Agent" = "MobBase-Installer" }
    } catch {
        Write-InstallError "Could not resolve the latest GitHub Release: $($_.Exception.Message)"
    }
    if ([string]::IsNullOrWhiteSpace($release.tag_name)) {
        Write-InstallError "The latest GitHub Release does not contain a tag name."
    }
    return $release.tag_name
}

function Get-MobDataHome {
    if (-not [string]::IsNullOrWhiteSpace($env:MOB_HOME)) {
        return $env:MOB_HOME
    }
    return (Join-Path $HOME ".mob")
}

function Get-ReleaseCacheDirectory([string]$Tag) {
    $tagBytes = [System.Text.Encoding]::UTF8.GetBytes($Tag)
    $hasher = [System.Security.Cryptography.SHA256]::Create()
    try {
        $tagHash = $hasher.ComputeHash($tagBytes)
    } finally {
        $hasher.Dispose()
    }
    $tagID = -join ($tagHash | ForEach-Object { $_.ToString("x2") })
    return (Join-Path (Get-MobDataHome) (Join-Path "cache\releases\windows-amd64" $tagID))
}

function Get-Checksum([object]$Content) {
    if ($Content -is [byte[]]) {
        $text = [System.Text.Encoding]::UTF8.GetString($Content)
    } else {
        $text = [string]$Content
    }
    return [regex]::Match($text, "(?i)\b[a-f0-9]{64}\b").Value.ToLowerInvariant()
}

function Get-VerifiedCachedBinary([string]$CacheBinary, [string]$CacheChecksum) {
    if (-not (Test-Path -LiteralPath $CacheBinary -PathType Leaf) -or -not (Test-Path -LiteralPath $CacheChecksum -PathType Leaf)) {
        return $null
    }
    try {
        $expected = Get-Checksum ([System.IO.File]::ReadAllBytes($CacheChecksum))
        if ($expected.Length -ne 64) {
            return $null
        }
        $actual = (Get-FileHash -LiteralPath $CacheBinary -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -eq $expected) {
            return $expected
        }
    } catch {
        return $null
    }
    return $null
}

function Install-ReleaseBinary([string]$Destination) {
    $tag = Resolve-ReleaseTag $Version
    $assetName = "mob-windows-amd64.exe"
    $escapedTag = [uri]::EscapeDataString($tag)
    $assetURL = "https://github.com/xy200303/MobBase/releases/download/$escapedTag/$assetName"
    $cacheDir = Get-ReleaseCacheDirectory $tag
    $cacheBinary = Join-Path $cacheDir $assetName
    $cacheChecksum = Join-Path $cacheDir "$assetName.sha256"
    if ($null -ne (Get-VerifiedCachedBinary $cacheBinary $cacheChecksum)) {
        Copy-Item -LiteralPath $cacheBinary -Destination $Destination -Force
        Write-Host "Using cached Mob release $tag"
        return
    }

    $temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("mob-" + [System.Guid]::NewGuid().ToString() + ".exe")
    try {
        $expected = Get-Checksum ((Invoke-WebRequest -Uri "$assetURL.sha256").Content)
        if ($expected.Length -ne 64) {
            Write-InstallError "Release $tag does not provide a valid $assetName.sha256 file."
        }
        Invoke-WebRequest -Uri $assetURL -OutFile $temporary
        $actual = (Get-FileHash -LiteralPath $temporary -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            Write-InstallError "Downloaded $assetName does not match the release SHA-256."
        }
        New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null
        Copy-Item -LiteralPath $temporary -Destination $cacheBinary -Force
        [System.IO.File]::WriteAllText($cacheChecksum, "$expected  $assetName`n", [System.Text.Encoding]::ASCII)
        Copy-Item -LiteralPath $temporary -Destination $Destination -Force
    } finally {
        Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
    }
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    if (-not [string]::IsNullOrWhiteSpace($env:MOB_INSTALL_DIR)) {
        $InstallDir = $env:MOB_INSTALL_DIR
    } elseif (-not [string]::IsNullOrWhiteSpace($env:MOB_HOME)) {
        $InstallDir = Join-Path $env:MOB_HOME "bin"
    } else {
        $InstallDir = Join-Path $HOME ".mob\bin"
    }
}
$InstallDir = [System.IO.Path]::GetFullPath($InstallDir)
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
$mob = Join-Path $InstallDir "mob.exe"

if (-not [string]::IsNullOrWhiteSpace($SourcePath)) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-InstallError "Go 1.26 or later is required only with -SourcePath. Omit it to install a release binary."
    }
    $SourcePath = (Resolve-Path $SourcePath).Path
    if (-not (Test-Path (Join-Path $SourcePath "go.mod"))) {
        Write-InstallError "-SourcePath must point to a Mob source checkout containing go.mod."
    }
    $temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("mob-" + [System.Guid]::NewGuid().ToString() + ".exe")
    try {
        Push-Location $SourcePath
        & go build -trimpath -o $temporary ./cmd/mob
        if ($LASTEXITCODE -ne 0) {
            Write-InstallError "go build returned exit code $LASTEXITCODE."
        }
        Copy-Item -LiteralPath $temporary -Destination $mob -Force
    } finally {
        Pop-Location -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
    }
} else {
    Install-ReleaseBinary $mob
}

if (-not $NoPath) {
    Add-UserPath $InstallDir
}

& $mob help | Select-Object -First 2
Write-Host "Mob installed to $mob"
if ($NoPath) {
    Write-Host "Add $InstallDir to PATH before running mob from a new terminal."
} else {
    Write-Host "Open a new terminal to use mob from PATH."
}
