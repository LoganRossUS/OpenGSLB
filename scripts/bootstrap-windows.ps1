#Requires -RunAsAdministrator
<#
.SYNOPSIS
    OpenGSLB Bootstrap Script for Windows

.DESCRIPTION
    This script downloads and configures OpenGSLB for automated deployments on Windows.

.PARAMETER Role
    Role: 'overwatch' or 'agent' (required)

.PARAMETER OverwatchIP
    IP address of the Overwatch node (required for agents)

.PARAMETER Version
    OpenGSLB version to install (default: latest)

.PARAMETER Region
    Region identifier (e.g., 'us-east', 'eu-west')

.PARAMETER ServiceToken
    Service token for agent authentication

.PARAMETER GossipKey
    Base64-encoded 32-byte gossip encryption key

.PARAMETER ServiceName
    Service name for backend registration (agents only, default: web)

.PARAMETER BackendPort
    Backend port (agents only, default: 80)

.PARAMETER NodeID
    Node identifier (default: hostname)

.PARAMETER GitHubRepo
    GitHub repository (default: LoganRossUS/OpenGSLB)

.PARAMETER SkipStart
    Don't start the service after installation

.PARAMETER Verbose
    Enable verbose output

.EXAMPLE
    .\bootstrap-windows.ps1 -Role agent -OverwatchIP 10.1.1.10 -Region us-east

.EXAMPLE
    .\bootstrap-windows.ps1 -Role overwatch -GossipKey "base64key==" -Verbose

.NOTES
    Exit codes:
      0 - Success
      1 - Invalid arguments
      2 - Download failed
      3 - Configuration failed
      4 - Service start failed
      5 - Health check failed
#>

param(
    [Parameter(Mandatory=$true)]
    [ValidateSet('overwatch', 'agent')]
    [string]$Role,

    [Parameter(Mandatory=$false)]
    [string]$OverwatchIP = "",

    [Parameter(Mandatory=$false)]
    [string]$Version = "latest",

    [Parameter(Mandatory=$false)]
    [string]$Region = "",

    [Parameter(Mandatory=$false)]
    [string]$ServiceToken = "",

    [Parameter(Mandatory=$false)]
    [string]$GossipKey = "",

    [Parameter(Mandatory=$false)]
    [string]$ServiceName = "web",

    [Parameter(Mandatory=$false)]
    [int]$BackendPort = 80,

    [Parameter(Mandatory=$false)]
    [string]$NodeID = "",

    [Parameter(Mandatory=$false)]
    [string]$GitHubRepo = "LoganRossUS/OpenGSLB",

    [Parameter(Mandatory=$false)]
    [switch]$SkipStart,

    [Parameter(Mandatory=$false)]
    [switch]$VerboseOutput
)

# Configuration
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"  # Speed up Invoke-WebRequest

$InstallDir = "C:\opengslb"
$ConfigDir = "C:\opengslb"
$LogDir = "C:\opengslb\logs"
$BinaryName = "opengslb.exe"
$LogFile = "C:\opengslb-bootstrap.log"

# Logging functions
function Write-Log {
    param([string]$Message, [string]$Level = "INFO")
    $timestamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
    $logMessage = "[$timestamp] [$Level] $Message"
    Write-Host $logMessage
    Add-Content -Path $LogFile -Value $logMessage -ErrorAction SilentlyContinue
}

function Write-Section {
    param([string]$Title)
    Write-Host ""
    Write-Host ("=" * 80)
    Write-Host " $Title"
    Write-Host ("=" * 80)
    Write-Log $Title "SECTION"
}

function Write-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
    Write-Log $Message "SUCCESS"
}

function Write-Warning {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
    Write-Log $Message "WARN"
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
    Write-Log $Message "ERROR"
}

function Write-DebugInfo {
    param([string]$Message)
    if ($VerboseOutput) {
        Write-Host "[DEBUG] $Message" -ForegroundColor Gray
        Write-Log $Message "DEBUG"
    }
}

# Print debug information
function Show-DebugInfo {
    Write-Section "DEBUG INFORMATION"
    Write-Host "Timestamp:        $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ')"
    Write-Host "Hostname:         $env:COMPUTERNAME"
    Write-Host "OS:               $([System.Environment]::OSVersion.VersionString)"
    Write-Host "PowerShell:       $($PSVersionTable.PSVersion)"
    Write-Host ""
    Write-Host "Network Interfaces:"
    Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -notlike '127.*' } |
        ForEach-Object { Write-Host "  $($_.InterfaceAlias): $($_.IPAddress)" }
    Write-Host ""
    Write-Host "Configuration:"
    Write-Host "  Role:           $Role"
    Write-Host "  Version:        $Version"
    Write-Host "  Overwatch IP:   $OverwatchIP"
    Write-Host "  Region:         $Region"
    Write-Host "  Node ID:        $NodeID"
    Write-Host "  Service Name:   $ServiceName"
    Write-Host "  Backend Port:   $BackendPort"
    Write-Host "  GitHub Repo:    $GitHubRepo"
    Write-Host ""
}

# Validate arguments
function Test-Arguments {
    Write-Section "Validating Arguments"

    if ($Role -eq "agent" -and [string]::IsNullOrEmpty($OverwatchIP)) {
        Write-Error "Missing required argument for agent: -OverwatchIP"
        exit 1
    }

    # Set defaults
    if ([string]::IsNullOrEmpty($NodeID)) {
        $script:NodeID = $env:COMPUTERNAME
    }

    # Generate random gossip key if not provided
    if ([string]::IsNullOrEmpty($GossipKey)) {
        Write-Warning "No gossip key provided, generating random key"
        $bytes = New-Object byte[] 32
        [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
        $script:GossipKey = [Convert]::ToBase64String($bytes)
    }

    # Generate random service token if not provided (for agents)
    if ($Role -eq "agent" -and [string]::IsNullOrEmpty($ServiceToken)) {
        Write-Warning "No service token provided, generating random token"
        $bytes = New-Object byte[] 24
        [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
        $script:ServiceToken = [Convert]::ToBase64String($bytes) -replace '[+/=]', ''
        $script:ServiceToken = $script:ServiceToken.Substring(0, [Math]::Min(32, $script:ServiceToken.Length))
    }

    Write-Success "Arguments validated"
    Write-DebugInfo "Role: $Role, Node ID: $NodeID, Overwatch IP: $OverwatchIP"
}

# Discover local IP address
function Get-LocalIP {
    Write-Section "Discovering Local IP Address"

    # Try Azure Instance Metadata Service first
    Write-DebugInfo "Trying Azure IMDS..."
    try {
        $response = Invoke-RestMethod -Uri "http://169.254.169.254/metadata/instance/network/interface/0/ipv4/ipAddress/0/privateIpAddress?api-version=2021-02-01&format=text" `
            -Headers @{"Metadata"="true"} -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($response -and $response -ne "null") {
            Write-Success "Discovered IP from Azure IMDS: $response"
            return $response
        }
    } catch {
        Write-DebugInfo "Azure IMDS not available"
    }

    # Try AWS Instance Metadata Service
    Write-DebugInfo "Trying AWS IMDS..."
    try {
        $response = Invoke-RestMethod -Uri "http://169.254.169.254/latest/meta-data/local-ipv4" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($response -match '^\d+\.\d+\.\d+\.\d+$') {
            Write-Success "Discovered IP from AWS IMDS: $response"
            return $response
        }
    } catch {
        Write-DebugInfo "AWS IMDS not available"
    }

    # Fallback: get IP from primary interface
    Write-DebugInfo "Falling back to interface discovery..."
    $ip = (Get-NetIPAddress -AddressFamily IPv4 |
           Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } |
           Sort-Object -Property InterfaceIndex |
           Select-Object -First 1).IPAddress

    if ($ip) {
        Write-Success "Discovered IP from interface: $ip"
        return $ip
    }

    Write-Error "Failed to discover local IP address"
    Write-Host ""
    Write-Host "Network interfaces:"
    Get-NetIPAddress -AddressFamily IPv4 | Format-Table InterfaceAlias, IPAddress -AutoSize
    exit 3
}

# Download the binary
function Get-Binary {
    Write-Section "Downloading OpenGSLB Binary"

    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
    }

    # Get latest version if needed
    if ($Version -eq "latest") {
        Write-Log "Fetching latest release version..."
        try {
            $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$GitHubRepo/releases/latest" -ErrorAction Stop
            $script:Version = $releases.tag_name
            Write-Log "Latest version: $Version"
        } catch {
            Write-Error "Failed to fetch latest version from GitHub"
            Write-Host ""
            Write-Host "Possible causes:"
            Write-Host "  1. GitHub API rate limit exceeded"
            Write-Host "  2. Repository does not exist or is private"
            Write-Host "  3. No releases have been published"
            Write-Host ""
            Write-Host "Try specifying a version explicitly: -Version v0.1.0"
            exit 2
        }
    }

    $downloadUrl = "https://github.com/$GitHubRepo/releases/download/$Version/opengslb-windows-amd64.exe"
    $binaryPath = Join-Path $InstallDir $BinaryName

    Write-Log "Downloading from: $downloadUrl"

    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $binaryPath -ErrorAction Stop
    } catch {
        Write-Error "Failed to download binary: $_"
        Write-Host ""
        Write-Host "Possible causes:"
        Write-Host "  1. Version $Version does not exist"
        Write-Host "  2. Release does not include windows-amd64 binary"
        Write-Host "  3. Network connectivity issues"
        Write-Host ""
        try {
            Write-Host "Available releases:"
            $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$GitHubRepo/releases" -ErrorAction SilentlyContinue
            $releases | Select-Object -First 5 | ForEach-Object { Write-Host "  - $($_.tag_name)" }
        } catch {
            Write-Host "  Unable to fetch releases"
        }
        exit 2
    }

    # Verify binary works
    try {
        $versionOutput = & $binaryPath --version 2>&1
        Write-Success "Installed: $versionOutput"
    } catch {
        Write-Error "Downloaded binary is not executable or corrupt"
        exit 2
    }
}

# Create configuration file
function New-Config {
    Write-Section "Creating Configuration"

    $localIP = Get-LocalIP
    $configPath = Join-Path $ConfigDir "config.yaml"

    if ($Role -eq "overwatch") {
        Write-Log "Generating Overwatch configuration..."
        $config = @"
# OpenGSLB Overwatch Configuration
# Generated by bootstrap script at $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ')

mode: overwatch

logging:
  level: info
  format: json

dns:
  listen_address: "0.0.0.0:53"
  zone: "test.opengslb.local"

api:
  enabled: true
  address: "0.0.0.0:8080"
  allowed_networks:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
    - "127.0.0.0/8"

metrics:
  enabled: true
  address: "0.0.0.0:9090"

overwatch:
  identity:
    node_id: "$NodeID"
    region: "$(if ($Region) { $Region } else { 'default' })"
  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "$GossipKey"
  routing:
    algorithm: latency
    fallback: geo

domains:
  - name: "test.opengslb.local"
    services:
      - name: "$ServiceName"
        routing:
          algorithm: latency
          fallback: geo

regions:
  - id: "us-east"
    name: "US East"
  - id: "eu-west"
    name: "EU West"
  - id: "ap-southeast"
    name: "Asia Pacific Southeast"
"@
    } else {
        Write-Log "Generating Agent configuration..."
        $config = @"
# OpenGSLB Agent Configuration
# Generated by bootstrap script at $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ')

mode: agent

logging:
  level: info
  format: json

agent:
  identity:
    agent_id: "$NodeID"
    region: "$(if ($Region) { $Region } else { 'default' })"
    service_token: "$ServiceToken"
  backends:
    - service: "$ServiceName"
      address: "$localIP"
      port: $BackendPort
      weight: 100
      health_check:
        type: http
        path: /
        interval: 10s
        timeout: 5s
  gossip:
    overwatch_nodes:
      - "${OverwatchIP}:7946"
    encryption_key: "$GossipKey"
  latency:
    enabled: true
    poll_interval: 10s
    ipv4_prefix: 24
    min_connection_age: 5s
    max_subnets: 10000
    subnet_ttl: 24h
    min_samples_per_subnet: 3
    report_interval: 30s
    ewma_alpha: 0.3

metrics:
  enabled: true
  address: "0.0.0.0:9090"
"@
    }

    # Write config with UTF8 no BOM
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($configPath, $config, $utf8NoBom)

    Write-Success "Configuration created: $configPath"
}

# Create Windows service using NSSM or Scheduled Task
function New-Service {
    Write-Section "Creating Windows Service"

    $serviceName = "OpenGSLB-$Role"
    $binaryPath = Join-Path $InstallDir $BinaryName
    $configPath = Join-Path $ConfigDir "config.yaml"

    # Check if NSSM is available (preferred method)
    $nssm = Get-Command nssm -ErrorAction SilentlyContinue

    if ($nssm) {
        Write-Log "Using NSSM to create Windows service..."

        # Remove existing service if present
        & nssm stop $serviceName 2>$null
        & nssm remove $serviceName confirm 2>$null

        # Install new service
        & nssm install $serviceName $binaryPath "--config" $configPath
        & nssm set $serviceName Description "OpenGSLB $Role Service"
        & nssm set $serviceName AppStdout (Join-Path $LogDir "stdout.log")
        & nssm set $serviceName AppStderr (Join-Path $LogDir "stderr.log")
        & nssm set $serviceName AppRotateFiles 1
        & nssm set $serviceName AppRotateBytes 10485760
        & nssm set $serviceName Start SERVICE_AUTO_START

        Write-Success "Windows service created: $serviceName"
    } else {
        Write-Warning "NSSM not found, using Scheduled Task instead"
        Write-Log "Creating scheduled task..."

        # Remove existing task if present
        Unregister-ScheduledTask -TaskName $serviceName -Confirm:$false -ErrorAction SilentlyContinue

        # Create task action
        $action = New-ScheduledTaskAction -Execute $binaryPath -Argument "--config `"$configPath`""

        # Create trigger (at startup)
        $trigger = New-ScheduledTaskTrigger -AtStartup

        # Create settings
        $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

        # Create principal (run as SYSTEM)
        $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

        # Register task
        Register-ScheduledTask -TaskName $serviceName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null

        Write-Success "Scheduled task created: $serviceName"
    }
}

# Start the service
function Start-OpenGSLBService {
    Write-Section "Starting Service"

    if ($SkipStart) {
        Write-Log "Skipping service start (-SkipStart flag)"
        return
    }

    $serviceName = "OpenGSLB-$Role"

    Write-Log "Starting $serviceName..."

    # Check if it's a Windows service or Scheduled Task
    $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue

    if ($service) {
        Start-Service -Name $serviceName -ErrorAction Stop
        Start-Sleep -Seconds 2

        $service = Get-Service -Name $serviceName
        if ($service.Status -ne "Running") {
            Write-Error "Service started but is not running"
            Write-Host ""
            Write-Host "Service status: $($service.Status)"
            Write-Host ""
            Write-Host "Check logs at: $LogDir"
            exit 4
        }
    } else {
        # Try scheduled task
        Start-ScheduledTask -TaskName $serviceName -ErrorAction Stop
        Start-Sleep -Seconds 2

        # Check if process is running
        $process = Get-Process -Name "opengslb" -ErrorAction SilentlyContinue
        if (-not $process) {
            Write-Error "Scheduled task started but process is not running"
            Write-Host ""
            Write-Host "Check logs at: $LogDir"
            exit 4
        }
    }

    Write-Success "Service started successfully"
}

# Run health check
function Test-Health {
    Write-Section "Running Health Check"

    if ($SkipStart) {
        Write-Log "Skipping health check (service not started)"
        return
    }

    $metricsPort = 9090
    $maxAttempts = 30
    $attempt = 1

    Write-Log "Waiting for metrics endpoint to be ready..."

    while ($attempt -le $maxAttempts) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:$metricsPort/metrics" -TimeoutSec 2 -ErrorAction SilentlyContinue
            if ($response.StatusCode -eq 200) {
                Write-Success "Metrics endpoint responding on port $metricsPort"
                break
            }
        } catch {
            # Continue waiting
        }

        if ($attempt -eq $maxAttempts) {
            Write-Error "Health check failed after $maxAttempts attempts"
            Write-Host ""
            Write-Host "Debugging information:"
            Write-Host ""
            Write-Host "Process status:"
            Get-Process -Name "opengslb" -ErrorAction SilentlyContinue | Format-Table Id, CPU, WorkingSet -AutoSize
            Write-Host ""
            Write-Host "Listening ports:"
            Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -in @(53, 8080, 7946, 9090) } | Format-Table LocalAddress, LocalPort -AutoSize
            Write-Host ""
            Write-Host "Check logs at: $LogDir"
            exit 5
        }

        Write-DebugInfo "Attempt $attempt/$maxAttempts - waiting..."
        Start-Sleep -Seconds 1
        $attempt++
    }

    # Role-specific checks
    if ($Role -eq "overwatch") {
        # Check API port
        try {
            $response = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/cluster/status" -TimeoutSec 2 -ErrorAction SilentlyContinue
            Write-Success "API server responding on port 8080"
        } catch {
            Write-Warning "API server not responding on port 8080"
        }
    } else {
        # Agent: check gossip connectivity
        Write-Log "Verifying gossip connectivity to Overwatch..."

        $gossipCheck = Test-NetConnection -ComputerName $OverwatchIP -Port 7946 -WarningAction SilentlyContinue -ErrorAction SilentlyContinue
        if ($gossipCheck.TcpTestSucceeded) {
            Write-Success "Gossip port reachable at ${OverwatchIP}:7946"
        } else {
            Write-Warning "Cannot reach Overwatch gossip port at ${OverwatchIP}:7946"
            Write-Warning "This may be normal if Overwatch is still starting"
        }
    }
}

# Write completion marker
function Write-CompletionMarker {
    $markerFile = "C:\opengslb-ready"
    Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ' | Out-File -FilePath $markerFile -Encoding UTF8
    Write-Success "Completion marker written: $markerFile"
}

# Print summary
function Show-Summary {
    Write-Section "Installation Complete"

    Write-Host ""
    Write-Host "OpenGSLB $($Role.Substring(0,1).ToUpper() + $Role.Substring(1)) has been installed and configured."
    Write-Host ""
    Write-Host "Details:"
    Write-Host "  Binary:          $InstallDir\$BinaryName"
    Write-Host "  Configuration:   $ConfigDir\config.yaml"
    Write-Host "  Service:         OpenGSLB-$Role"
    Write-Host "  Logs:            $LogDir"
    Write-Host ""

    if ($Role -eq "overwatch") {
        Write-Host "Endpoints:"
        Write-Host "  DNS:             0.0.0.0:53"
        Write-Host "  API:             http://localhost:8080/api/v1/"
        Write-Host "  Metrics:         http://localhost:9090/metrics"
        Write-Host "  Cluster Status:  http://localhost:8080/api/v1/cluster/status"
        Write-Host ""
        Write-Host "Test DNS:"
        Write-Host "  nslookup $ServiceName.test.opengslb.local localhost"
        Write-Host ""
    } else {
        Write-Host "Endpoints:"
        Write-Host "  Metrics:         http://localhost:9090/metrics"
        Write-Host ""
        Write-Host "Agent is registered with Overwatch at: ${OverwatchIP}:7946"
        Write-Host ""
    }

    Write-Host "Useful commands:"
    Write-Host "  Status:     Get-Service OpenGSLB-$Role  (or Get-ScheduledTask)"
    Write-Host "  Logs:       Get-Content $LogDir\*.log -Tail 50"
    Write-Host "  Restart:    Restart-Service OpenGSLB-$Role"
    Write-Host ""
}

# Main
function Main {
    Write-Section "OpenGSLB Bootstrap Script for Windows"
    Write-Log "Starting installation at $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ')"

    if ($VerboseOutput) {
        Show-DebugInfo
    }

    Test-Arguments
    Get-Binary
    New-Config
    New-Service
    Start-OpenGSLBService
    Test-Health
    Write-CompletionMarker
    Show-Summary

    Write-Success "Bootstrap completed successfully!"
    exit 0
}

# Run main
Main
