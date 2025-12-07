# Show HN: OpenGSLB – Because There's No Open-Source GSLB That Actually Exists

**TL;DR**: I needed self-hosted Global Server Load Balancing without a commercial license. I looked for open-source options. There basically aren't any. So I built one.

**GitHub**: github.com/loganrossus/OpenGSLB

---

## The Gap in the Market

Global Server Load Balancing (GSLB) is a solved problem—if you have budget. F5 BIG-IP DNS, Citrix NetScaler, A10 Thunder, Radware Alteon, Kemp LoadMaster, Infoblox. All excellent products. All require commercial licenses ranging from $7,500 to $50,000+ annually depending on capacity and features.

These vendors all offer on-premises deployment. You can absolutely run F5 or NetScaler in your private datacenter without exposing anything to the internet. That's not the issue.

The issue is: **what if you don't want to pay for a license?**

I went looking for open-source GSLB solutions. Here's what I found:

| Project | Status | Catch |
|---------|--------|-------|
| **polaris-gslb** | Last meaningful commit 2019 | Requires PowerDNS backend, effectively abandoned |
| **k8gb** | Active | Requires Kubernetes. If you're not running K8s, it's not an option |
| **gdnsd** | Active | DNS server with basic geographic features, limited health checking, not really GSLB |
| **PowerDNS + Lua** | DIY | Roll your own with scripting. Complex, fragile, undocumented |
| **BIND + external scripts** | DIY | Same story. Duct tape and hope |

That's it. That's the list.

If you want DNS-based multi-site failover without Kubernetes and without a commercial license, your options are abandoned projects or DIY scripting.

---

## What OpenGSLB Is

OpenGSLB fills this gap. It's a purpose-built GSLB system that:

- **Runs standalone** - Single Go binary, no external dependencies
- **Doesn't require Kubernetes** - Works on any Linux box, VM, or container
- **Is actually maintained** - Active development, not a 5-year-old repo
- **Does real GSLB** - Health checks, multiple routing algorithms, automatic failover
- **Costs nothing** - AGPLv3 for open-source use, commercial license available, no registration, no phone-home

**Current Features:**
- Authoritative DNS server (A/AAAA records)
- HTTP and TCP health checks with configurable failure/success thresholds
- Round-robin, weighted, and active/standby routing
- Prometheus metrics
- YAML configuration with hot-reload (SIGHUP)
- Docker images on ghcr.io

**Roadmap:**
- Geolocation routing
- Latency-based routing
- High availability (keepalived/VIP)
- Gossip-based clustering for distributed health consensus

---

## How It Works

```yaml
dns:
  listen_address: ":53"
  default_ttl: 30

regions:
  - name: dc1
    servers:
      - address: "10.1.1.10"
        port: 443
    health_check:
      type: http
      path: /health
      interval: 5s
      timeout: 2s
      failure_threshold: 2

  - name: dc2
    servers:
      - address: "10.2.1.10"
        port: 443
    health_check:
      type: http
      path: /health
      interval: 5s
      timeout: 2s
      failure_threshold: 2

domains:
  - name: app.internal.example.com
    routing_algorithm: failover
    failover_order:
      - dc1
      - dc2
    ttl: 30
```

Point your internal DNS to delegate `app.internal.example.com` to OpenGSLB. It health-checks both datacenters, returns the healthy one, and fails over automatically when a site goes down.

That's it. No appliance to rack. No license key to manage. No sales call.

---

## Who This Is For

**You need GSLB if:**
- You run the same application in multiple datacenters or sites
- You want automatic failover when a site goes down
- You want to distribute load across sites
- You're tired of manually updating DNS records during outages

**You need OpenGSLB specifically if:**
- You don't have budget for F5/NetScaler/etc.
- You're not running Kubernetes (so k8gb isn't an option)
- You want something actively maintained (so polaris-gslb is out)
- You don't want to build a fragile DIY solution with scripts

---

## What OpenGSLB Is Not

Let me be clear about limitations:

**It's not a replacement for F5 BIG-IP DNS if you need:**
- 100 million RPS on a hardware chassis (though OpenGSLB should handle significant load—see performance notes below)
- DNSSEC signing today (it's on the roadmap)
- DDoS mitigation (OpenGSLB assumes you're behind a firewall)
- 15 different load balancing algorithms (we have 3, adding more)
- iRules or complex programmable traffic policies
- 20 years of battle-tested edge case handling

**It's not a replacement for k8gb if:**
- You're already running Kubernetes everywhere
- You want CRD-based configuration
- You need tight integration with K8s health probes

**It is:**
- The open-source GSLB that exists for non-Kubernetes environments
- Good enough for internal applications that need multi-site availability
- Simple enough to deploy in an afternoon
- Cheap enough (free) to run for every application that needs it

---

## Performance Expectations

I haven't done formal load testing yet, but here's a theoretical estimate based on the architecture:

**What happens per query:**
1. Receive UDP packet (~200 bytes)
2. Parse DNS packet (miekg/dns library—same one CoreDNS uses)
3. Map lookup for domain registry
4. Map lookup for health status
5. Routing decision (round-robin is an atomic increment + modulo)
6. Build and send response

That's maybe 1-2 microseconds of actual CPU work. The rest is network I/O.

**On a modest VM (4 vCPU, 16GB RAM, 10Gbps NIC):**

- CPU: At ~2μs per query with realistic syscall overhead, 100,000-200,000 QPS per core is reasonable. With 3 cores available for query processing, that's a 300,000-600,000 QPS theoretical ceiling.
- Memory: Irrelevant. DNS responses are tiny. This would run fine on 256MB.
- Network: ~400 bytes per query/response round trip. 10Gbps could push 3-4 million QPS before saturation.

**Realistic estimate: 50,000-200,000 QPS** on that spec before you'd need to tune or scale horizontally. The variance depends on lock contention in the current implementation, Go's UDP handling under load, and kernel tuning.

This is a theoretical estimate—I'll publish real benchmarks once I do proper load testing. But the architecture has no obvious bottlenecks. CoreDNS uses the same DNS library and handles millions of QPS in production Kubernetes clusters.

For context, most internal GSLB use cases are probably doing hundreds to low thousands of QPS. There should be massive headroom for typical deployments.

---

## Current State

This is real, working software, not vaporware:

- **Sprint 1 complete**: CI/CD, Docker builds, integration testing
- **Sprint 2 complete**: DNS server, health checking, round-robin routing, metrics
- **Sprint 3 in progress**: Weighted routing, active/standby failover, TCP health checks, hot-reload

I'm running it in my own test environment across Azure regions. The integration test suite covers the core scenarios. It's not production-hardened at enterprise scale, but it works.

---

## Try It

```bash
# Docker
docker run -p 53:53/udp -p 53:53/tcp -p 9090:9090 \
  -v ./config.yaml:/etc/opengslb/config.yaml \
  ghcr.io/loganrossus/opengslb:latest

# Binary
git clone https://github.com/loganrossus/OpenGSLB
cd OpenGSLB
go build -o opengslb ./cmd/opengslb
./opengslb --config config.yaml
```

---

## Why I Built This

I needed GSLB for internal services. I didn't have F5 budget. I looked for open source options and found a wasteland of abandoned projects and Kubernetes-only solutions.

So I built the thing that should exist: a simple, standalone, open-source GSLB that you can deploy on any Linux box without a license or a container orchestrator.

If you've been duct-taping DNS failover together with scripts, or you've been wishing you could justify F5 licensing for internal apps, or you've looked at k8gb and thought "but I don't run Kubernetes"—this is for you.

---

**GitHub**: github.com/loganrossus/OpenGSLB  
**License**: AGPLv3 / Commercial  
**Language**: Go  
**Status**: Active development, functional for core use cases