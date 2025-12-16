# Demo 4: GeoIP-Based Routing

This demo demonstrates OpenGSLB's geographic IP-based routing capabilities, where DNS queries are automatically routed to the nearest datacenter based on the client's geographic location.

## What You'll Learn

- GeoIP database integration (MaxMind/DB-IP)
- EDNS Client Subnet (ECS) support
- Custom CIDR mappings for private networks
- Region fallback behavior
- Real-time geo routing decisions

## What This Demo Proves

| Capability | Demonstration |
|------------|---------------|
| **GeoIP Database Integration** | MaxMind/DB-IP database loads and resolves IPs to countries/continents |
| **Region Mapping** | Countries/continents correctly map to configured regions |
| **Custom CIDR Overrides** | Internal/private IPs route to specified regions via custom mappings |
| **Fallback Behavior** | Unknown IPs fall back to default region gracefully |
| **Real-time Routing** | Changing source IP immediately changes routing decision |
| **Health Integration** | Unhealthy regions skipped even if geographically closest |
| **API Testing** | Test geo routing decisions without actual DNS queries |

## Architecture

```
+-----------------------------------------------------------------------+
|                         DEMO 4 TOPOLOGY                                |
|                       GeoIP-Based Routing                              |
+-----------------------------------------------------------------------+
|                                                                        |
|   CLIENT (dns-client) - Simulates queries from different locations     |
|   +------------------------------------------------------------------+ |
|   | PUBLIC IPs (GeoIP lookup):                                       | |
|   |   8.8.8.8        -> US (Google DNS)        -> us-east            | |
|   |   185.228.168.9  -> Germany                -> eu-west            | |
|   |   1.1.1.1        -> Australia              -> ap-southeast       | |
|   |   202.12.29.205  -> Japan                  -> ap-southeast       | |
|   |                                                                  | |
|   | PRIVATE IPs (Custom CIDR mappings):                              | |
|   |   10.50.0.100    -> Corporate HQ (Kentucky) -> us-chicago        | |
|   |   172.16.50.50   -> VPN Users               -> eu-london         | |
|   |   192.168.1.100  -> Home Office             -> us-east           | |
|   +------------------------------------------------------------------+ |
|                                 |                                      |
|                                 v                                      |
|   +------------------------------------------------------------------+ |
|   |                        OVERWATCH                                 | |
|   |                   (GeoIP Routing Engine)                         | |
|   |                                                                  | |
|   |  +-------------+  +-------------+  +-------------+               | |
|   |  | GeoIP DB    |  | Custom      |  | Health      |               | |
|   |  | MaxMind     |  | CIDRs       |  | State       |               | |
|   |  +------+------+  +------+------+  +------+------+               | |
|   |         |                |                |                      | |
|   |         +----------------+----------------+                      | |
|   |                          |                                       | |
|   |              +-----------v-----------+                           | |
|   |              |   Routing Decision    |                           | |
|   |              | 1. Check custom CIDRs |                           | |
|   |              | 2. Lookup GeoIP DB    |                           | |
|   |              | 3. Map to region      |                           | |
|   |              | 4. Filter by health   |                           | |
|   |              | 5. Fallback if needed |                           | |
|   |              +-----------------------+                           | |
|   +------------------------------------------------------------------+ |
|                                 |                                      |
|         +-----------------------+-----------------------+              |
|         |                       |                       |              |
|         v                       v                       v              |
|   +-----------+           +-----------+           +-----------+        |
|   |  US-EAST  |           |  EU-WEST  |           |AP-SOUTHEAST|       |
|   | US,CA,MX  |           | GB,DE,FR  |           | AU,JP,SG   |       |
|   | NA,SA     |           | EU        |           | AS,OC      |       |
|   +-----------+           +-----------+           +-----------+        |
|                                                                        |
|   Additional regions (Custom CIDR only):                               |
|   US-CHICAGO | US-DALLAS | EU-LONDON                                   |
+-----------------------------------------------------------------------+
```

## Container Inventory

| Container | Role | Network IP | Port(s) |
|-----------|------|------------|---------|
| `overwatch` | DNS + GeoIP Router | 172.28.0.10 | 53, 8080, 9090 |
| `backend-us-1` | US East Backend #1 | 172.28.1.10 | 80 |
| `backend-us-2` | US East Backend #2 | 172.28.1.11 | 80 |
| `backend-eu-1` | EU West Backend #1 | 172.28.2.10 | 80 |
| `backend-eu-2` | EU West Backend #2 | 172.28.2.11 | 80 |
| `backend-ap-1` | AP Southeast Backend #1 | 172.28.3.10 | 80 |
| `backend-ap-2` | AP Southeast Backend #2 | 172.28.3.11 | 80 |
| `backend-chicago-1` | Chicago Backend | 172.28.4.10 | 80 |
| `backend-dallas-1` | Dallas Backend | 172.28.5.10 | 80 |
| `backend-london-1` | London Backend | 172.28.6.10 | 80 |
| `client` | Query Simulator | 172.28.0.50 | 22 (SSH) |

## Quick Start

### 1. Build the Binary

```bash
# From the repository root
CGO_ENABLED=0 GOOS=linux go build -o demos/demo-4-geoip-routing/bin/opengslb ./cmd/opengslb
```

### 2. Start the Demo

```bash
cd demos/demo-4-geoip-routing
docker-compose up -d
```

### 3. Access the Client

```bash
# Option 1: SSH into client container
ssh -p 2222 root@localhost
# Password: demo

# Option 2: Direct docker exec
docker exec -it client /bin/bash
```

### 4. Run the Interactive Demo

```bash
./demo.sh
```

## Testing GeoIP Routing

### Using dig with EDNS Client Subnet

Simulate queries from different locations:

```bash
# Query from US IP (Google DNS)
dig @172.28.0.10 app.global.example.com +short +subnet=8.8.8.8/32

# Query from German IP
dig @172.28.0.10 app.global.example.com +short +subnet=185.228.168.9/32

# Query from Australian IP (Cloudflare)
dig @172.28.0.10 app.global.example.com +short +subnet=1.1.1.1/32

# Query from Japanese IP
dig @172.28.0.10 app.global.example.com +short +subnet=202.12.29.205/32

# Query from custom CIDR (Kentucky office -> Chicago)
dig @172.28.0.10 app.global.example.com +short +subnet=10.50.100.50/32

# Query from VPN range (-> London)
dig @172.28.0.10 app.global.example.com +short +subnet=172.16.50.50/32
```

### Using the API

```bash
# Test IP routing decision
curl http://localhost:8080/api/v1/geo/test?ip=8.8.8.8 | jq .

# List custom CIDR mappings
curl http://localhost:8080/api/v1/geo/mappings | jq .

# Check backend health
curl http://localhost:8080/api/v1/health/servers | jq .
```

## Demo Scenarios

### Scenario 1: GeoIP Routing by Country

Public IPs from different countries route to their nearest regional datacenter:

- **8.8.8.8** (US - Google DNS) -> us-east
- **185.228.168.9** (Germany) -> eu-west
- **202.12.29.205** (Japan) -> ap-southeast
- **1.1.1.1** (Australia) -> ap-southeast
- **200.160.0.8** (Brazil) -> us-east (South America fallback)

### Scenario 2: Custom CIDR Mappings

Private/internal IPs use custom CIDR mappings (checked BEFORE GeoIP lookup):

- **10.50.x.x** (Corporate HQ - Kentucky) -> us-chicago
- **172.16.x.x** (VPN Users) -> eu-london
- **192.168.x.x** (Home Office) -> us-east

### Scenario 3: Fallback for Unknown IPs

IPs not in GeoIP database and not matching custom CIDRs use default region:

- **192.0.2.1** (TEST-NET-1, reserved) -> us-east (default)
- **198.51.100.1** (TEST-NET-2, reserved) -> us-east (default)

### Scenario 4: Real-Time Region Switching

Simulates a user "traveling" between locations:

1. Start in New York (8.8.8.8) -> us-east
2. Fly to London (185.228.168.9) -> eu-west
3. Connect to Corporate VPN (172.16.50.50) -> eu-london
4. Fly to Tokyo (202.12.29.205) -> ap-southeast
5. Arrive at Kentucky Office (10.50.100.50) -> us-chicago

## Region Configuration

### GeoIP-Mapped Regions

| Region | Countries | Continents |
|--------|-----------|------------|
| us-east | US, CA, MX | NA, SA |
| eu-west | GB, DE, FR, ES, IT, NL, BE, CH, AT, PL, SE, NO, DK, FI, IE, PT | EU |
| ap-southeast | AU, JP, SG, KR, IN, NZ, TH, MY, PH, ID, VN, TW, HK | AS, OC |

### Custom CIDR-Only Regions

| Region | CIDR | Use Case |
|--------|------|----------|
| us-chicago | 10.50.0.0/16, 10.40.0.0/16 | Corporate HQ, Chicago DC |
| us-dallas | 10.60.0.0/16 | Texas Datacenter |
| eu-london | 172.16.0.0/12 | VPN Users |

## Resolution Order

1. **Custom CIDR Mappings** - Longest prefix match (checked first)
2. **GeoIP Database Lookup** - Country match > Continent match
3. **Default Region Fallback** - us-east

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/geo/test?ip=X.X.X.X` | GET | Test routing decision for IP |
| `/api/v1/geo/mappings` | GET | List custom CIDR mappings |
| `/api/v1/geo/mappings` | PUT | Add custom CIDR mapping |
| `/api/v1/geo/mappings/{cidr}` | DELETE | Remove custom CIDR mapping |
| `/api/v1/health/servers` | GET | Get all backend health status |

## GeoIP Database Setup

:::{tip}
For full GeoIP functionality, register for a free MaxMind license at https://dev.maxmind.com/geoip/geolite2-free-geolocation-data and set `MAXMIND_LICENSE_KEY` environment variable. Without this, the demo uses the free DB-IP database.
:::

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| All queries go to default region | GeoIP database not loaded | Check `docker logs overwatch` for database load errors |
| Custom CIDR not matching | CIDR format incorrect | Verify CIDR notation (e.g., `10.50.0.0/16`) |
| "Unknown" country for public IP | IP not in GeoIP database | Ensure database downloaded successfully |
| Permission denied on config | Config permissions too open | Ensure config files are chmod 600 |

## Cleanup

```bash
# Stop and remove all containers
docker-compose down

# Also remove volumes (GeoIP database)
docker-compose down -v
```

## Next Steps

Continue to [Demo 5: Predictive Health](demo-5-predictive-health) to learn about proactive health monitoring and chaos engineering.
