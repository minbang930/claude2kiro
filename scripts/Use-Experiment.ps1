param(
    [Parameter(Position = 0, Mandatory = $true)]
    [string]$Branch,

    [string]$Repo = "minbang930/claude2kiro",

    [string]$TargetPath = (Join-Path $HOME ".claude2kiro\bin\claude2kiro-patched.exe"),

    [int]$Port = 8080,

    [int]$WaitMinutes = 5,

    [switch]$NoRestart
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Write-Step([string]$Message) {
    Write-Host "[lab] $Message"
}

function Get-RepoRoot {
    $root = (& git -C $PSScriptRoot rev-parse --show-toplevel 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $root) {
        throw "This script must be run from a clone of the claude2kiro repository."
    }
    return $root.Trim()
}

function Invoke-Git([string[]]$GitArgs) {
    & git @GitArgs
    if ($LASTEXITCODE -ne 0) {
        throw "git failed: git $($GitArgs -join ' ')"
    }
}

function Get-BranchHead([string]$Root, [string]$Name) {
    Invoke-Git @("-C", $Root, "fetch", "origin", $Name)
    $sha = (& git -C $Root rev-parse "origin/$Name" 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $sha) {
        throw "Could not resolve origin/$Name"
    }
    return $sha.Trim()
}

function Test-GhReady {
    $cmd = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $cmd) { return $false }

    & gh auth status -h github.com *> $null
    return ($LASTEXITCODE -eq 0)
}

function Download-ArtifactWithGh(
    [string]$Repository,
    [string]$Name,
    [string]$HeadSha,
    [string]$Destination,
    [int]$WaitForMinutes
) {
    $deadline = (Get-Date).AddMinutes($WaitForMinutes)

    do {
        $json = (& gh run list --repo $Repository --workflow "Windows Build" --branch $Name --limit 30 --json databaseId,headSha,status,conclusion 2>$null)
        if ($LASTEXITCODE -eq 0 -and $json) {
            $runs = $json | ConvertFrom-Json
            $match = $runs |
                Where-Object { $_.headSha -eq $HeadSha -and $_.conclusion -eq "success" } |
                Select-Object -First 1

            if ($match) {
                Write-Step "Downloading Windows artifact for $($HeadSha.Substring(0, 12))"
                & gh run download $match.databaseId --repo $Repository --name "claude2kiro-windows-amd64" --dir $Destination
                if ($LASTEXITCODE -ne 0) {
                    throw "gh run download failed."
                }

                $exe = Get-ChildItem -LiteralPath $Destination -Filter "claude2kiro-windows-amd64.exe" -Recurse |
                    Select-Object -First 1

                if (-not $exe) {
                    throw "Windows artifact downloaded, but claude2kiro-windows-amd64.exe was not found."
                }
                return $exe.FullName
            }

            $pending = $runs |
                Where-Object { $_.headSha -eq $HeadSha -and ($_.status -eq "queued" -or $_.status -eq "in_progress") } |
                Select-Object -First 1

            if ($pending -and (Get-Date) -lt $deadline) {
                Write-Step "Windows Build is still running; waiting..."
                Start-Sleep -Seconds 5
                continue
            }
        }

        break
    } while ((Get-Date) -lt $deadline)

    return $null
}

function Build-WithGo(
    [string]$Root,
    [string]$Name,
    [string]$HeadSha,
    [string]$Destination
) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        return $null
    }

    $worktree = Join-Path $Destination "src"
    Write-Step "Building $Name from a temporary worktree"

    try {
        Invoke-Git @("-C", $Root, "worktree", "add", "--detach", $worktree, $HeadSha)

        $out = Join-Path $Destination "claude2kiro-windows-amd64.exe"
        Push-Location $worktree
        try {
            $oldGoos = $env:GOOS
            $oldGoarch = $env:GOARCH
            $oldCgo = $env:CGO_ENABLED
            try {
                $env:GOOS = "windows"
                $env:GOARCH = "amd64"
                $env:CGO_ENABLED = "0"
                & go build -trimpath '-ldflags=-s -w' -o $out .
                if ($LASTEXITCODE -ne 0) {
                    throw "go build failed."
                }
            }
            finally {
                $env:GOOS = $oldGoos
                $env:GOARCH = $oldGoarch
                $env:CGO_ENABLED = $oldCgo
            }
        }
        finally {
            Pop-Location
        }

        if (-not (Test-Path -LiteralPath $out)) {
            throw "go build completed without creating the expected EXE."
        }
        return $out
    }
    finally {
        if (Test-Path -LiteralPath $worktree) {
            & git -C $Root worktree remove --force $worktree *> $null
        }
    }
}

function Get-ListenerInfo([int]$ListenPort) {
    $conn = Get-NetTCPConnection -LocalPort $ListenPort -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if (-not $conn) { return $null }

    $proc = Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
    if (-not $proc) {
        return [pscustomobject]@{
            Pid  = $conn.OwningProcess
            Path = $null
            Name = $null
        }
    }

    $path = $null
    try { $path = $proc.Path } catch {}

    return [pscustomobject]@{
        Pid  = $proc.Id
        Path = $path
        Name = $proc.ProcessName
    }
}

function Stop-InstalledProxy([string]$InstalledPath, [int]$ListenPort) {
    $targetFull = [IO.Path]::GetFullPath($InstalledPath)

    $listener = Get-ListenerInfo $ListenPort
    if ($listener) {
        $listenerPath = $listener.Path
        $safe = $false

        if ($listenerPath) {
            try {
                $safe = ([IO.Path]::GetFullPath($listenerPath) -ieq $targetFull)
            }
            catch {}
        }

        if (-not $safe -and $listener.Name -and $listener.Name -like "claude2kiro*") {
            $safe = $true
        }

        if (-not $safe) {
            throw "Port $ListenPort is owned by another process (PID $($listener.Pid)); refusing to stop it."
        }

        Write-Step "Stopping proxy on port $ListenPort"
        Stop-Process -Id $listener.Pid -Force -ErrorAction Stop
    }

    $deadline = (Get-Date).AddSeconds(8)
    do {
        if (-not (Get-ListenerInfo $ListenPort)) { return }
        Start-Sleep -Milliseconds 200
    } while ((Get-Date) -lt $deadline)

    throw "Port $ListenPort did not become free."
}

function Start-And-VerifyProxy([string]$InstalledPath, [int]$ListenPort) {
    Write-Step "Starting proxy on port $ListenPort"
    Start-Process -FilePath $InstalledPath -ArgumentList @("server", "$ListenPort") -WindowStyle Hidden | Out-Null

    $deadline = (Get-Date).AddSeconds(12)
    do {
        try {
            $body = (Invoke-WebRequest "http://127.0.0.1:$ListenPort/health" -UseBasicParsing -TimeoutSec 1).Content
            if ($body -eq "OK") {
                return
            }
        }
        catch {}
        Start-Sleep -Milliseconds 300
    } while ((Get-Date) -lt $deadline)

    throw "Proxy did not pass /health after restart."
}

$repoRoot = Get-RepoRoot
$headSha = Get-BranchHead $repoRoot $Branch
$tmp = Join-Path $env:TEMP ("claude2kiro-lab-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

$backup = "$TargetPath.lab-backup"

try {
    $artifact = $null

    if (Test-GhReady) {
        $artifact = Download-ArtifactWithGh $Repo $Branch $headSha $tmp $WaitMinutes
    }

    if (-not $artifact) {
        $artifact = Build-WithGo $repoRoot $Branch $headSha $tmp
    }

    if (-not $artifact) {
        throw "Need either authenticated GitHub CLI ('gh auth login') or Go on PATH. No install was attempted."
    }

    $artifactHash = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash
    Write-Step "Artifact SHA256 $artifactHash"

    $targetDir = Split-Path -Parent $TargetPath
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null

    if (-not $NoRestart) {
        Stop-InstalledProxy $TargetPath $Port
    }

    Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $TargetPath) {
        Copy-Item -LiteralPath $TargetPath -Destination $backup -Force
    }

    try {
        Copy-Item -LiteralPath $artifact -Destination $TargetPath -Force

        if (-not $NoRestart) {
            Start-And-VerifyProxy $TargetPath $Port
        }
    }
    catch {
        if (Test-Path -LiteralPath $backup) {
            Write-Step "Install failed; restoring previous runtime"
            Copy-Item -LiteralPath $backup -Destination $TargetPath -Force
            if (-not $NoRestart) {
                try { Start-And-VerifyProxy $TargetPath $Port } catch {}
            }
        }
        throw
    }

    $installedHash = (Get-FileHash -LiteralPath $TargetPath -Algorithm SHA256).Hash
    if ($installedHash -ne $artifactHash) {
        throw "Installed EXE hash does not match the selected artifact."
    }

    $statePath = Join-Path $HOME ".claude2kiro\lab-current.json"
    [pscustomobject]@{
        branch      = $Branch
        commit      = $headSha
        sha256      = $installedHash
        target      = $TargetPath
        installedAt = (Get-Date).ToString("o")
    } | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding UTF8

    Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue

    Write-Host ""
    Write-Host "Installed: $Branch"
    Write-Host "Commit   : $headSha"
    Write-Host "SHA256   : $installedHash"
    if (-not $NoRestart) {
        Write-Host "Proxy    : http://127.0.0.1:$Port/health = OK"
    }
}
finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
