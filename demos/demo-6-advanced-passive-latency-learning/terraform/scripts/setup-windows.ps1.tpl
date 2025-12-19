# OpenGSLB Windows Setup Script
# This script is executed by Azure CustomScriptExtension

$ErrorActionPreference = "Continue"
$LogFile = "C:\opengslb-setup.log"

function Log {
    param([string]$Message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "[$timestamp] $Message"
    Write-Host $logMessage
    Add-Content -Path $LogFile -Value $logMessage
}

Log "=========================================="
Log "OpenGSLB Windows Setup Starting"
Log "=========================================="

# Create directories
Log "Creating directories..."
New-Item -ItemType Directory -Force -Path "C:\opengslb" | Out-Null
New-Item -ItemType Directory -Force -Path "C:\opengslb\logs" | Out-Null

# Install IIS
Log "Installing IIS Web Server..."
try {
    Install-WindowsFeature -Name Web-Server -IncludeManagementTools -ErrorAction Stop
    Log "IIS installed successfully"
} catch {
    Log "WARNING: Failed to install IIS: $_"
}

# Install Chocolatey
Log "Installing Chocolatey..."
try {
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    $env:ChocolateyInstall = "C:\ProgramData\chocolatey"
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    Log "Chocolatey installed successfully"
} catch {
    Log "ERROR: Failed to install Chocolatey: $_"
    exit 1
}

# Refresh environment variables
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
Log "PATH updated: $env:Path"

# Install Git and Go using Chocolatey
Log "Installing Git and Go via Chocolatey..."
$chocoExe = "C:\ProgramData\chocolatey\bin\choco.exe"
if (Test-Path $chocoExe) {
    try {
        & $chocoExe install git -y --no-progress 2>&1 | ForEach-Object { Log $_ }
        Log "Git installed"
        & $chocoExe install golang -y --no-progress 2>&1 | ForEach-Object { Log $_ }
        Log "Go installed"
    } catch {
        Log "ERROR: Failed to install packages: $_"
        exit 1
    }
} else {
    Log "ERROR: Chocolatey not found at $chocoExe"
    exit 1
}

# Refresh PATH again after installing git and go
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
Log "PATH refreshed after package installs"

# Set Go environment
$env:GOPATH = "C:\Users\${admin_username}\go"
$env:GOMODCACHE = "C:\Users\${admin_username}\go\pkg\mod"
New-Item -ItemType Directory -Force -Path $env:GOPATH | Out-Null
New-Item -ItemType Directory -Force -Path $env:GOMODCACHE | Out-Null
Log "GOPATH set to: $env:GOPATH"
Log "GOMODCACHE set to: $env:GOMODCACHE"

# Clone OpenGSLB repository
Log "Cloning OpenGSLB repository..."
$gitExe = "C:\Program Files\Git\bin\git.exe"
if (Test-Path $gitExe) {
    try {
        & $gitExe clone --depth 1 --branch "${git_branch}" "${git_repo}" "C:\opengslb\src" 2>&1 | ForEach-Object { Log $_ }
        Log "Repository cloned successfully"
    } catch {
        Log "ERROR: Failed to clone repository: $_"
        exit 1
    }
} else {
    Log "ERROR: Git not found at $gitExe"
    exit 1
}

# Build OpenGSLB
Log "Building OpenGSLB..."
$goExe = "C:\Program Files\Go\bin\go.exe"
if (Test-Path $goExe) {
    try {
        Push-Location "C:\opengslb\src"
        & $goExe build -o "C:\opengslb\opengslb.exe" ./cmd/opengslb 2>&1 | ForEach-Object { Log $_ }
        Pop-Location
        if (Test-Path "C:\opengslb\opengslb.exe") {
            Log "OpenGSLB built successfully"
        } else {
            Log "ERROR: Build completed but opengslb.exe not found"
            exit 1
        }
    } catch {
        Log "ERROR: Failed to build OpenGSLB: $_"
        exit 1
    }
} else {
    Log "ERROR: Go not found at $goExe"
    exit 1
}

# Write agent configuration
Log "Writing agent configuration..."
$configContent = @"
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
"@

$configContent | Out-File -Encoding utf8 -FilePath "C:\opengslb\agent.yaml"
Log "Configuration written to C:\opengslb\agent.yaml"

# Create scheduled task for OpenGSLB
Log "Creating scheduled task for OpenGSLB..."
try {
    $action = New-ScheduledTaskAction -Execute "C:\opengslb\opengslb.exe" -Argument "--mode agent --config C:\opengslb\agent.yaml"
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName "OpenGSLB Agent" -Action $action -Trigger $trigger -Principal $principal -Force
    Log "Scheduled task created"

    # Start the task immediately
    Start-ScheduledTask -TaskName "OpenGSLB Agent"
    Log "OpenGSLB Agent started"
} catch {
    Log "ERROR: Failed to create scheduled task: $_"
    exit 1
}

Log "=========================================="
Log "OpenGSLB Windows Setup Complete!"
Log "=========================================="

exit 0
