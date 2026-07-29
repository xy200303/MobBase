[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$SourcePath,
    [string]$InstallDir,
    [switch]$NoPath
)

$ErrorActionPreference = "Stop"
$modulePath = "github.com/xy200303/MobBase/cmd/mob"

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
    $exists = $entries | Where-Object { $_.TrimEnd("\\") -ieq $Directory.TrimEnd("\\") }
    if (-not $exists) {
        $updated = @($entries + $Directory) -join ";"
        [Environment]::SetEnvironmentVariable("Path", $updated, "User")
    }
    if (($env:Path -split ";" | Where-Object { $_.TrimEnd("\\") -ieq $Directory.TrimEnd("\\") }).Count -eq 0) {
        $env:Path = "$Directory;$env:Path"
    }
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-InstallError "Go 1.26 or later is required. Install Go, reopen PowerShell, then rerun this script."
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

if ([string]::IsNullOrWhiteSpace($SourcePath) -and -not $PSBoundParameters.ContainsKey("Version")) {
    $candidate = Join-Path $PSScriptRoot ".."
    if (Test-Path (Join-Path $candidate "go.mod")) {
        $SourcePath = $candidate
    }
}

if (-not [string]::IsNullOrWhiteSpace($SourcePath)) {
    $SourcePath = (Resolve-Path $SourcePath).Path
    if (-not (Test-Path (Join-Path $SourcePath "go.mod"))) {
        Write-InstallError "-SourcePath must point to a Mob source checkout containing go.mod."
    }
    $temporaryBinary = Join-Path ([System.IO.Path]::GetTempPath()) ("mob-" + [System.Guid]::NewGuid().ToString() + ".exe")
    try {
        Push-Location $SourcePath
        & go build -o $temporaryBinary ./cmd/mob
        if ($LASTEXITCODE -ne 0) {
            Write-InstallError "go build returned exit code $LASTEXITCODE."
        }
        Copy-Item -LiteralPath $temporaryBinary -Destination (Join-Path $InstallDir "mob.exe") -Force
    } finally {
        Pop-Location -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $temporaryBinary -Force -ErrorAction SilentlyContinue
    }
} else {
    $previousGoBin = $env:GOBIN
    try {
        $env:GOBIN = $InstallDir
        & go install "$modulePath@$Version"
        if ($LASTEXITCODE -ne 0) {
            Write-InstallError "go install returned exit code $LASTEXITCODE."
        }
    } finally {
        $env:GOBIN = $previousGoBin
    }
}

if (-not $NoPath) {
    Add-UserPath $InstallDir
}

$mob = Join-Path $InstallDir "mob.exe"
& $mob help | Select-Object -First 2

Write-Host "Mob installed to $mob"
if ($NoPath) {
    Write-Host "Add $InstallDir to PATH before running mob from a new terminal."
} else {
    Write-Host "Open a new terminal to use mob from PATH."
}
