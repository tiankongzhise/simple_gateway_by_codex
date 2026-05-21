# go_build.ps1
# Build Go binary as static, stripped binary for Baota (Linux amd64 by default)

param(
    [string]$OutputName = "simple_gateway_by_codex",
    [string]$GoOS = "linux",
    [string]$GoArch = "amd64",
    [switch]$CompressWithUpx,
    [string]$LdFlags = "-s -w"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

function Fail-Build {
    param([string]$Message)

    Write-Host "ERROR: $Message" -ForegroundColor Red
    exit 1
}

function Quote-ProcessArgument {
    param([string]$Argument)

    if ($null -eq $Argument -or $Argument.Length -eq 0) {
        return '""'
    }
    if ($Argument -notmatch '[\s"]') {
        return $Argument
    }

    $result = '"'
    $backslashes = 0
    foreach ($char in $Argument.ToCharArray()) {
        if ($char -eq '\') {
            $backslashes++
            continue
        }
        if ($char -eq '"') {
            if ($backslashes -gt 0) {
                $result += ('\' * ($backslashes * 2))
            }
            $result += '\"'
            $backslashes = 0
            continue
        }
        if ($backslashes -gt 0) {
            $result += ('\' * $backslashes)
            $backslashes = 0
        }
        $result += $char
    }
    if ($backslashes -gt 0) {
        $result += ('\' * ($backslashes * 2))
    }
    $result += '"'

    return $result
}

function Invoke-GoCapture {
    param([string[]]$Arguments)

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = "go"
    $startInfo.Arguments = ($Arguments | ForEach-Object { Quote-ProcessArgument $_ }) -join " "
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo

    try {
        [void]$process.Start()
        $stdout = $process.StandardOutput.ReadToEnd()
        $stderr = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        $output = (($stdout, $stderr) -join "").Trim()
        $exitCode = $process.ExitCode
    } finally {
        $process.Dispose()
    }

    [pscustomobject]@{
        ExitCode = $exitCode
        Output = $output
    }
}

function Get-GoVersion {
    param([string]$VersionOutput)

    if ($VersionOutput -notmatch "go version go(?<Version>\d+\.\d+(?:\.\d+)?)") {
        return $null
    }

    return [version]$Matches.Version
}

$minGoVersion = [version]"1.26.3"
$env:GOCACHE = Join-Path $PSScriptRoot ".gocache"
$env:GOMODCACHE = Join-Path $PSScriptRoot ".gomodcache"

Write-Host "[1/4] Preparing local Go caches..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force -Path $env:GOMODCACHE | Out-Null

Write-Host "[2/4] Checking Go toolchain..."
$goToolchainResult = Invoke-GoCapture @("env", "GOTOOLCHAIN")
if ($goToolchainResult.ExitCode -ne 0) {
    Fail-Build "Unable to read GOTOOLCHAIN. Output: $($goToolchainResult.Output)"
}
$goToolchain = $goToolchainResult.Output

$goVersionResult = Invoke-GoCapture @("version")
if ($goVersionResult.ExitCode -ne 0) {
    Fail-Build "Unable to run go version. Install Go $minGoVersion or newer, or allow GOTOOLCHAIN=auto to select it. Output: $($goVersionResult.Output)"
}
$goVersionText = $goVersionResult.Output
$goVersion = Get-GoVersion $goVersionText
if ($null -eq $goVersion) {
    Fail-Build "Unable to parse Go version from: $goVersionText"
}
if ($goVersion -lt $minGoVersion) {
    Fail-Build "Detected $goVersionText with GOTOOLCHAIN=$goToolchain. This build requires Go $minGoVersion or newer; install a newer Go toolchain or set GOTOOLCHAIN=auto."
}

Write-Host "Using $goVersionText (GOTOOLCHAIN=$goToolchain)" -ForegroundColor Gray
Write-Host "[3/4] Building Go binary for $GoOS/$GoArch ..." -ForegroundColor Cyan

$env:GOOS = $GoOS
$env:GOARCH = $GoArch
$env:CGO_ENABLED = "0"

$buildArgs = @("build", "-trimpath", "-buildvcs=false", "-ldflags=$LdFlags", "-o", $OutputName, "./cmd/gateway")
Write-Host "Running: go $($buildArgs -join ' ')" -ForegroundColor Gray

$buildResult = Invoke-GoCapture $buildArgs
if ($buildResult.Output) {
    Write-Host $buildResult.Output
}
if ($buildResult.ExitCode -ne 0) {
    Fail-Build "Build failed"
}

Write-Host "[4/4] Build succeeded: $OutputName" -ForegroundColor Green

$fileInfo = Get-Item $OutputName
Write-Host "File size: $([math]::Round($fileInfo.Length / 1MB, 2)) MB" -ForegroundColor Yellow

if ($CompressWithUpx) {
    if (Get-Command upx -ErrorAction SilentlyContinue) {
        Write-Host "Compressing with UPX..."
        upx --best --lzma $OutputName
        $compressedInfo = Get-Item $OutputName
        Write-Host "Compressed size: $([math]::Round($compressedInfo.Length / 1MB, 2)) MB" -ForegroundColor Green
    } else {
        Write-Host "Warning: UPX not found. Skipping compression." -ForegroundColor Yellow
    }
}

Write-Host "Done. Binary: $OutputName" -ForegroundColor Cyan
