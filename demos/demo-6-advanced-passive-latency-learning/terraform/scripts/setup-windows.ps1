# OpenGSLB Windows Agent Setup Script
# Usage: .\setup-windows.ps1 -GitBranch <branch> -GitRepo <repo> -ServiceToken <token> -GossipKey <key> -AdminUser <user>

param(
    [string]$GitBranch = "main",
    [string]$GitRepo = "https://github.com/LoganRossUS/OpenGSLB.git",
    [string]$ServiceToken = "test-token-for-latency-testing",
    [string]$GossipKey = "kUgbkXysBGdpykayRXgRwHeLMQs4gCena6jvqfxQBpE=",
    [string]$AdminUser = "azureuser"
)

$ErrorActionPreference = "Stop"
$LogFile = "C:\opengslb-setup.log"

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
    Log "Params: GitBranch=$GitBranch, AdminUser=$AdminUser"
    Log "=========================================="

    # Create directories
    Log "Creating directories..."
    New-Item -Path "C:\opengslb" -ItemType Directory -Force | Out-Null
    New-Item -Path "C:\opengslb\logs" -ItemType Directory -Force | Out-Null

    # Install IIS
    Log "Installing IIS..."
    Install-WindowsFeature -Name Web-Server -IncludeManagementTools
    Log "IIS installed"

    # Install Chocolatey
    Log "Installing Chocolatey..."
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    $env:ChocolateyInstall = "C:\ProgramData\chocolatey"
    Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    Log "Chocolatey installed"

    # Refresh PATH
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

    # Install Git
    Log "Installing Git..."
    & "C:\ProgramData\chocolatey\bin\choco.exe" install git -y --no-progress
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    Log "Git installed"

    # Install Go
    Log "Installing Go..."
    & "C:\ProgramData\chocolatey\bin\choco.exe" install golang -y --no-progress
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    Log "Go installed"

    # Set Go environment
    $env:GOPATH = "C:\Users\$AdminUser\go"
    $env:GOMODCACHE = "C:\Users\$AdminUser\go\pkg\mod"
    New-Item -Path $env:GOPATH -ItemType Directory -Force | Out-Null
    New-Item -Path $env:GOMODCACHE -ItemType Directory -Force | Out-Null
    Log "Go env: GOPATH=$env:GOPATH"

    # Clone OpenGSLB
    Log "Cloning $GitRepo branch $GitBranch..."
    Set-Location C:\opengslb
    & "C:\Program Files\Git\bin\git.exe" clone --depth 1 --branch $GitBranch $GitRepo src
    Log "Clone complete"

    # Build OpenGSLB
    Log "Building OpenGSLB..."
    Set-Location C:\opengslb\src
    & "C:\Program Files\Go\bin\go.exe" build -o C:\opengslb\opengslb.exe ./cmd/opengslb

    if (Test-Path "C:\opengslb\opengslb.exe") {
        Log "Build successful"
    } else {
        throw "Build failed - binary not found"
    }

    # Create config file
    Log "Creating config..."
    $config = @"
mode: agent
agent:
  identity:
    service_token: $ServiceToken
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
    encryption_key: $GossipKey
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
    # Write config without BOM (UTF8 BOM breaks YAML parsing)
    [System.IO.File]::WriteAllText("C:\opengslb\agent.yaml", $config, [System.Text.UTF8Encoding]::new($false))
    Log "Config created (UTF8 without BOM)"

    # Set secure permissions on config file (contains sensitive tokens)
    Log "Setting config file permissions..."
    $configPath = "C:\opengslb\agent.yaml"
    icacls $configPath /inheritance:r /grant:r "SYSTEM:F" /grant:r "Administrators:F"
    Log "Config permissions set"

    # Create scheduled task
    Log "Creating scheduled task..."
    $action = New-ScheduledTaskAction -Execute "C:\opengslb\opengslb.exe" -Argument "--mode agent --config C:\opengslb\agent.yaml"
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName "OpenGSLB Agent" -Action $action -Trigger $trigger -Principal $principal -Force
    Log "Task created"

    # Start the agent
    Start-ScheduledTask -TaskName "OpenGSLB Agent"
    Log "Agent started"

    Log "=========================================="
    Log "OpenGSLB Windows Setup Complete!"
    Log "=========================================="
    exit 0
}
catch {
    $msg = $_.Exception.Message
    $line = $_.InvocationInfo.ScriptLineNumber
    Log "ERROR at line $line : $msg"
    exit 1
}
