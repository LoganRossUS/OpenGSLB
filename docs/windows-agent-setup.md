# OpenGSLB Agent Setup on Windows Server

This guide walks through setting up an OpenGSLB Agent on Windows Server. The agent will automatically register with your Overwatch cluster and participate in latency-based routing.

## Prerequisites

- Windows Server 2019 or 2022
- PowerShell 5.1 or later (included with Windows Server)
- Network access to your Overwatch node (port 7946 for gossip)
- Administrator privileges

## Quick Start

### Option 1: Using the Bootstrap Script

The easiest way to set up an OpenGSLB agent is using the bootstrap script:

```powershell
# Download the bootstrap script
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/LoganRossUS/OpenGSLB/main/scripts/bootstrap-windows.ps1" -OutFile "C:\bootstrap.ps1"

# Run the script (adjust parameters for your environment)
.\bootstrap.ps1 `
    -Role agent `
    -OverwatchIP "10.1.1.10" `
    -Region "us-east" `
    -ServiceToken "your-service-token" `
    -GossipKey "your-base64-gossip-key" `
    -ServiceName "web" `
    -BackendPort 80 `
    -VerboseOutput
```

### Option 2: Manual Installation

#### Step 1: Download OpenGSLB

```powershell
# Create installation directory
New-Item -ItemType Directory -Path "C:\opengslb" -Force
New-Item -ItemType Directory -Path "C:\opengslb\logs" -Force

# Download the latest release
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$releases = Invoke-RestMethod "https://api.github.com/repos/LoganRossUS/OpenGSLB/releases/latest"
$version = $releases.tag_name
$downloadUrl = "https://github.com/LoganRossUS/OpenGSLB/releases/download/$version/opengslb-windows-amd64.exe"

Write-Host "Downloading OpenGSLB $version..."
Invoke-WebRequest -Uri $downloadUrl -OutFile "C:\opengslb\opengslb.exe"

# Verify the download
& C:\opengslb\opengslb.exe --version
```

#### Step 2: Create Configuration File

Create `C:\opengslb\config.yaml` with your agent configuration:

```yaml
# OpenGSLB Agent Configuration
mode: agent

logging:
  level: info
  format: json

agent:
  identity:
    agent_id: "windows-agent-01"      # Unique identifier for this agent
    region: "us-east"                  # Your region identifier
    service_token: "your-token-here"   # Must match Overwatch configuration

  backends:
    - service: "web.test.opengslb.local"  # Service FQDN (must match Overwatch domain)
      address: "10.2.1.11"                # This server's IP address
      port: 80                         # Backend port (IIS, nginx, etc.)
      weight: 100
      health_check:
        type: http
        path: /
        interval: 10s
        timeout: 5s

  gossip:
    overwatch_nodes:
      - "10.1.1.10:7946"               # Overwatch gossip address
    encryption_key: "your-base64-key"  # 32-byte base64 gossip key

  latency_learning:
    enabled: true
    poll_interval: 10s
    report_interval: 30s

metrics:
  enabled: true
  address: "0.0.0.0:9090"
```

**Important**: Replace the following values:
- `agent_id`: Unique name for this agent (e.g., hostname)
- `region`: Your region identifier (must match Overwatch regions config)
- `service_token`: Authentication token (get from Overwatch admin)
- `service`: FQDN format like `web.test.opengslb.local` (must match Overwatch domain config)
- `address`: This server's IP address
- `overwatch_nodes`: IP:port of your Overwatch server
- `encryption_key`: Base64-encoded 32-byte gossip encryption key

#### Step 3: Create a Scheduled Task

```powershell
# Create the scheduled task to run OpenGSLB at startup
$action = New-ScheduledTaskAction `
    -Execute "C:\opengslb\opengslb.exe" `
    -Argument "--config C:\opengslb\config.yaml" `
    -WorkingDirectory "C:\opengslb"

$trigger = New-ScheduledTaskTrigger -AtStartup

$principal = New-ScheduledTaskPrincipal `
    -UserId "SYSTEM" `
    -LogonType ServiceAccount `
    -RunLevel Highest

$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Days 365)

Register-ScheduledTask `
    -TaskName "OpenGSLB-agent" `
    -Action $action `
    -Trigger $trigger `
    -Principal $principal `
    -Settings $settings `
    -Force
```

#### Step 4: Start the Agent

```powershell
# Start the scheduled task
Start-ScheduledTask -TaskName "OpenGSLB-agent"

# Verify the process is running
Get-Process opengslb

# Check the metrics endpoint
Invoke-RestMethod "http://localhost:9090/metrics"
```

## Setting Up IIS as a Backend

If you want to use IIS as your backend web server:

```powershell
# Install IIS
Install-WindowsFeature -Name Web-Server -IncludeManagementTools

# Start IIS
Start-Service W3SVC

# Create a test page
$html = @"
<!DOCTYPE html>
<html>
<head><title>OpenGSLB Backend</title></head>
<body>
<h1>OpenGSLB Backend Server</h1>
<p>Hostname: $env:COMPUTERNAME</p>
<p>Region: us-east</p>
</body>
</html>
"@
Set-Content -Path "C:\inetpub\wwwroot\index.html" -Value $html

# Test it
Invoke-WebRequest "http://localhost/" -UseBasicParsing
```

## Firewall Configuration

Ensure the following ports are open:

```powershell
# Gossip port (required for cluster communication)
New-NetFirewallRule -DisplayName "OpenGSLB Gossip" -Direction Inbound -LocalPort 7946 -Protocol TCP -Action Allow
New-NetFirewallRule -DisplayName "OpenGSLB Gossip UDP" -Direction Inbound -LocalPort 7946 -Protocol UDP -Action Allow

# Metrics port (optional, for monitoring)
New-NetFirewallRule -DisplayName "OpenGSLB Metrics" -Direction Inbound -LocalPort 9090 -Protocol TCP -Action Allow

# Backend port (e.g., IIS on port 80)
New-NetFirewallRule -DisplayName "HTTP" -Direction Inbound -LocalPort 80 -Protocol TCP -Action Allow
```

## Verifying the Agent

### Check Agent Status

```powershell
# Check if process is running
Get-Process opengslb

# Check scheduled task status
Get-ScheduledTask -TaskName "OpenGSLB-agent" | Select-Object State
Get-ScheduledTaskInfo -TaskName "OpenGSLB-agent"

# Check metrics
Invoke-RestMethod "http://localhost:9090/metrics" | Select-String "opengslb"
```

### Check Registration with Overwatch

From a machine with access to Overwatch:

```bash
# List registered agents
curl http://<overwatch-ip>:8080/api/v1/overwatch/agents | jq .

# Check cluster status
curl http://<overwatch-ip>:8080/api/v1/cluster/status | jq .
```

## Troubleshooting

### Agent Won't Start

1. Check the scheduled task last run result:
   ```powershell
   Get-ScheduledTaskInfo -TaskName "OpenGSLB-agent"
   ```

2. Try running manually to see errors:
   ```powershell
   & C:\opengslb\opengslb.exe --config C:\opengslb\config.yaml
   ```

3. Check Windows Event Viewer for errors

### Can't Connect to Overwatch

1. Test network connectivity:
   ```powershell
   Test-NetConnection -ComputerName 10.1.1.10 -Port 7946
   ```

2. Verify gossip key matches between agent and Overwatch

3. Check firewall rules on both ends

### Agent Registered but Backend Unhealthy

1. Verify your backend service is running:
   ```powershell
   # For IIS
   Get-Service W3SVC

   # Test the endpoint
   Invoke-WebRequest "http://localhost:80/" -UseBasicParsing
   ```

2. Check the health check configuration in config.yaml matches your backend

### Viewing Logs

OpenGSLB logs to the console by default. To capture logs:

```powershell
# Run manually with output redirection
& C:\opengslb\opengslb.exe --config C:\opengslb\config.yaml 2>&1 | Tee-Object -FilePath C:\opengslb\logs\agent.log
```

Or modify the scheduled task to redirect output:

```powershell
$action = New-ScheduledTaskAction `
    -Execute "cmd.exe" `
    -Argument "/c C:\opengslb\opengslb.exe --config C:\opengslb\config.yaml >> C:\opengslb\logs\agent.log 2>&1" `
    -WorkingDirectory "C:\opengslb"
```

## Updating the Agent

```powershell
# Stop the current agent
Stop-ScheduledTask -TaskName "OpenGSLB-agent"
Start-Sleep -Seconds 5

# Download new version
$releases = Invoke-RestMethod "https://api.github.com/repos/LoganRossUS/OpenGSLB/releases/latest"
$version = $releases.tag_name
$downloadUrl = "https://github.com/LoganRossUS/OpenGSLB/releases/download/$version/opengslb-windows-amd64.exe"

Invoke-WebRequest -Uri $downloadUrl -OutFile "C:\opengslb\opengslb.exe"

# Verify and restart
& C:\opengslb\opengslb.exe --version
Start-ScheduledTask -TaskName "OpenGSLB-agent"
```

## Uninstalling

```powershell
# Stop and remove the scheduled task
Stop-ScheduledTask -TaskName "OpenGSLB-agent" -ErrorAction SilentlyContinue
Unregister-ScheduledTask -TaskName "OpenGSLB-agent" -Confirm:$false

# Remove files
Remove-Item -Path "C:\opengslb" -Recurse -Force
```
