# OpenGSLB Windows Agent Setup Script
# This script installs IIS, Git, Go, and builds OpenGSLB

$ErrorActionPreference = "Continue"
Start-Transcript -Path "C:\opengslb-setup.log" -Append

Write-Host "========== OpenGSLB Windows Setup Started at $(Get-Date) =========="

# Create directories
Write-Host "Creating directories..."
New-Item -Path "C:\opengslb" -ItemType Directory -Force | Out-Null
New-Item -Path "C:\opengslb\logs" -ItemType Directory -Force | Out-Null
New-Item -Path "C:\temp" -ItemType Directory -Force | Out-Null

# Install IIS
Write-Host "Installing IIS..."
Install-WindowsFeature -Name Web-Server -IncludeManagementTools
Write-Host "IIS installed"

# Install Chocolatey (package manager)
Write-Host "Installing Chocolatey..."
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
Write-Host "Chocolatey installed"

# Install Git using Chocolatey
Write-Host "Installing Git..."
choco install git -y --no-progress
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
Write-Host "Git installed"

# Install Go using Chocolatey
Write-Host "Installing Go..."
choco install golang -y --no-progress
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
Write-Host "Go installed"

# Refresh environment
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

# Clone OpenGSLB
Write-Host "Cloning OpenGSLB from ${git_repo} branch ${git_branch}..."
Set-Location C:\opengslb
& "C:\Program Files\Git\bin\git.exe" clone --depth 1 --branch "${git_branch}" "${git_repo}" src
if ($LASTEXITCODE -ne 0) {
    Write-Host "FAILED to clone repository"
} else {
    Write-Host "Clone successful"
}

# Build OpenGSLB
Write-Host "Building OpenGSLB..."
Set-Location C:\opengslb\src
& go build -o C:\opengslb\opengslb.exe ./cmd/opengslb
if (Test-Path "C:\opengslb\opengslb.exe") {
    Write-Host "Build successful"
} else {
    Write-Host "Build failed - binary not found"
}

# Create config file
Write-Host "Creating config file..."
$config = @"
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
  encryption_key: ${gossip_encryption_key}
  overwatch_nodes:
    - 10.1.1.10:7946
heartbeat:
  interval: 10s
metrics:
  enabled: true
  address: ':9090'
logging:
  level: debug
  format: json
"@
$config | Out-File -FilePath "C:\opengslb\agent.yaml" -Encoding utf8
Write-Host "Config file created"

# Create and register scheduled task
Write-Host "Creating scheduled task..."
$action = New-ScheduledTaskAction -Execute "C:\opengslb\opengslb.exe" -Argument "--mode agent --config C:\opengslb\agent.yaml"
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName "OpenGSLB Agent" -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force
Write-Host "Scheduled task created"

# Start the agent
Write-Host "Starting OpenGSLB Agent..."
Start-ScheduledTask -TaskName "OpenGSLB Agent"

Write-Host "========== OpenGSLB Windows Setup Completed at $(Get-Date) =========="
Write-Host "View logs: type C:\opengslb-setup.log"
Write-Host "Check binary: Test-Path C:\opengslb\opengslb.exe"
Write-Host "Check task: Get-ScheduledTask -TaskName 'OpenGSLB Agent'"

Stop-Transcript
