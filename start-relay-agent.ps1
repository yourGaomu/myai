[CmdletBinding()]
param(
    [string]$ListenAddress = "0.0.0.0:18080",
    [string]$AgentServer = "",
    [string]$Token = "dearUU",
    [string]$User = "local",
    [string]$Device = "pc-local",
    [string]$Workspace = "",
    [string]$BindCode = ""
)

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($Workspace)) {
    $Workspace = $projectRoot
}
$listenPortText = ($ListenAddress -split ":")[-1]
$listenPort = 0
if (-not [int]::TryParse($listenPortText, [ref]$listenPort) -or $listenPort -lt 1 -or $listenPort -gt 65535) {
    throw "ListenAddress must end with a valid TCP port: $ListenAddress"
}
if ([string]::IsNullOrWhiteSpace($AgentServer)) {
    $AgentServer = "ws://127.0.0.1:$listenPort/ws/agent"
}
$healthUrl = "http://127.0.0.1:$listenPort/health"
$buildDirectory = Join-Path ([System.IO.Path]::GetTempPath()) "myai-startup"
$binary = Join-Path $buildDirectory "myai.exe"
$previousToken = $env:MYAI_RELAY_AGENT_TOKEN
$relayProcess = $null
$agentProcess = $null

function Stop-ChildProcess {
    param([System.Diagnostics.Process]$Process)

    if ($null -ne $Process -and -not $Process.HasExited) {
        Stop-Process -Id $Process.Id -Force -ErrorAction SilentlyContinue
    }
}

try {
    New-Item -ItemType Directory -Force -Path $buildDirectory | Out-Null

    Write-Host "Building MyAI..."
    & go build -o $binary .
    if ($LASTEXITCODE -ne 0) {
        throw "MyAI build failed."
    }

    $env:MYAI_RELAY_AGENT_TOKEN = $Token

    Write-Host "Starting Relay on $ListenAddress (CORS: all origins)..."
    $relayProcess = Start-Process -FilePath $binary -ArgumentList @(
        "relay",
        "--addr", $ListenAddress,
        "--agent-user", $User,
        "--agent-device", $Device,
        "--allowed-origin", "*"
    ) -WorkingDirectory $projectRoot -NoNewWindow -PassThru

    $relayReady = $false
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        if ($relayProcess.HasExited) {
            throw "Relay exited before becoming healthy (exit code $($relayProcess.ExitCode))."
        }
        try {
            $response = Invoke-RestMethod -Uri $healthUrl -TimeoutSec 1
            if ($response.status -eq "ok") {
                $relayReady = $true
                break
            }
        } catch {
            Start-Sleep -Milliseconds 200
        }
    }
    if (-not $relayReady) {
        throw "Relay health check timed out at $healthUrl."
    }

    $agentArguments = @(
        "agent",
        "--server", $AgentServer,
        "--user", $User,
        "--device", $Device,
        "--workspace", (Resolve-Path -LiteralPath $Workspace).Path
    )
    if (-not [string]::IsNullOrWhiteSpace($BindCode)) {
        $agentArguments += @("--bind-code", $BindCode)
    }

    Write-Host "Starting Agent and connecting to $AgentServer..."
    $agentProcess = Start-Process -FilePath $binary -ArgumentList $agentArguments `
        -WorkingDirectory $projectRoot -NoNewWindow -PassThru

    Write-Host "Relay and Agent are running. Press Ctrl+C to stop both."
    Write-Host "Token: $Token"
    Write-Host "Mobile Relay URL: http://<this-PC-LAN-IP>:$listenPort"

    while (-not $relayProcess.HasExited -and -not $agentProcess.HasExited) {
        Start-Sleep -Milliseconds 500
    }

    if ($relayProcess.HasExited) {
        throw "Relay exited with code $($relayProcess.ExitCode)."
    }
    throw "Agent exited with code $($agentProcess.ExitCode)."
} finally {
    Stop-ChildProcess $agentProcess
    Stop-ChildProcess $relayProcess
    $env:MYAI_RELAY_AGENT_TOKEN = $previousToken
}
