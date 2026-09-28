param(
    [int]$Tail = 120,
    [int]$Last = 8
)

$ErrorActionPreference = "Stop"

$log = Join-Path $HOME (".claude2kiro\logs\" + (Get-Date -Format "yyyy-MM-dd") + ".log")
if (-not (Test-Path -LiteralPath $log)) {
    throw "Log file not found: $log"
}

Get-Content -LiteralPath $log -Tail $Tail |
    Select-String 'Model route:|Backend response model' |
    Select-Object -Last $Last |
    ForEach-Object { $_.Line }
