# OpenGSLB Windows Setup Script
# This script is executed by Azure CustomScriptExtension

$ErrorActionPreference = "Stop"
$LogFile = "C:\opengslb-setup.log"

# Initialize log file first
"[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] Script starting..." | Out-File -FilePath $LogFile -Encoding utf8

function Log {
    param([string]$Message)
    $line = "[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] $Message"
    Write-Host $line
    $line | Out-File -FilePath $LogFile -Append -Encoding utf8
}

try {
    Log "=========================================="
    Log "OpenGSLB Windows Setup Starting"
    Log "=========================================="

    # Create directories
    Log "Creating directories..."
    New-Item -ItemType Directory -Force -Path "C:\opengslb" | Out-Null
    New-Item -ItemType Directory -Force -Path "C:\opengslb\logs" | Out-Null

    # Install IIS
    Log "Installing IIS Web Server..."
    Install-WindowsFeature -Name Web-Server -IncludeManagementTools
    Log "IIS installed successfully"

    # Install Chocolatey
    Log "Installing Chocolatey..."
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    $env:ChocolateyInstall = "C:\ProgramData\chocolatey"
    Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    Log "Chocolatey installed successfully"

    # Refresh environment variables
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    Log "PATH updated"

    # Install Git and Go using Chocolatey
    Log "Installing Git via Chocolatey..."
    & "C:\ProgramData\chocolatey\bin\choco.exe" install git -y --no-progress
    Log "Git installed"

    Log "Installing Go via Chocolatey..."
    & "C:\ProgramData\chocolatey\bin\choco.exe" install golang -y --no-progress
    Log "Go installed"

    # Refresh PATH again
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    Log "PATH refreshed"

    # Set Go environment
    $env:GOPATH = "C:\Users\${admin_username}\go"
    $env:GOMODCACHE = "C:\Users\${admin_username}\go\pkg\mod"
    New-Item -ItemType Directory -Force -Path $env:GOPATH | Out-Null
    New-Item -ItemType Directory -Force -Path $env:GOMODCACHE | Out-Null
    Log "Go environment configured"

    # Clone OpenGSLB repository
    Log "Cloning OpenGSLB repository..."
    & "C:\Program Files\Git\bin\git.exe" clone --depth 1 --branch "${git_branch}" "${git_repo}" "C:\opengslb\src"
    Log "Repository cloned"

    # Build OpenGSLB
    Log "Building OpenGSLB..."
    Push-Location "C:\opengslb\src"
    & "C:\Program Files\Go\bin\go.exe" build -o "C:\opengslb\opengslb.exe" ./cmd/opengslb
    Pop-Location
    Log "OpenGSLB built"

    # Write agent configuration
    Log "Writing agent configuration..."
    @"
mode: agent
identity:
  service_token: ${service_token}
  region: eu-west
backends:
  - service: web
    address: 0.0.0.0
    port: 80
    weight: 100
    health_check:
      type: http
      path: /
      interval: 5s
      timeout: 2s
latency_learning:
  enabled: true
  poll_interval: 10s
  ipv4_prefix: 24
  ipv6_prefix: 48
  min_connection_age: 5s
  max_subnets: 10000
  subnet_ttl: 24h
  min_samples: 3
  report_interval: 30s
  ewma_alpha: 0.3
gossip:
  encryption_key: ${gossip_key}
  overwatch_nodes:
    - 10.1.1.10:7946
heartbeat:
  interval: 10s
metrics:
  enabled: true
  address: :9090
logging:
  level: debug
  format: json
"@ | Out-File -Encoding utf8 -FilePath "C:\opengslb\agent.yaml"
    Log "Configuration written"

    # Create scheduled task for OpenGSLB
    Log "Creating scheduled task..."
    $action = New-ScheduledTaskAction -Execute "C:\opengslb\opengslb.exe" -Argument "--mode agent --config C:\opengslb\agent.yaml"
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName "OpenGSLB Agent" -Action $action -Trigger $trigger -Principal $principal -Force
    Log "Scheduled task created"

    Start-ScheduledTask -TaskName "OpenGSLB Agent"
    Log "OpenGSLB Agent started"

    Log "=========================================="
    Log "OpenGSLB Windows Setup Complete!"
    Log "=========================================="
    exit 0
}
catch {
    $errorMsg = $_.Exception.Message
    $errorLine = $_.InvocationInfo.ScriptLineNumber
    "[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] ERROR at line $errorLine : $errorMsg" | Out-File -FilePath $LogFile -Append -Encoding utf8
    exit 1
}
