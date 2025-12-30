# Hacker News Launch Post for OpenGSLB

## Title Options (choose one)

**Option A (Show HN format - recommended):**
> Show HN: OpenGSLB – Self-hosted global load balancing with passive latency learning

**Option B (More descriptive):**
> Show HN: OpenGSLB – Open-source alternative to F5 GTM and Route53 for multi-region traffic routing

**Option C (Problem-focused):**
> Show HN: OpenGSLB – DNS-based GSLB that learns real client latency, not proxy perspective

---

## Post Text

```
I've been building OpenGSLB, an open-source Global Server Load Balancing system for self-hosters and organizations that want intelligent DNS-based traffic routing without vendor lock-in.

**The Problem:**
Enterprise GSLB solutions (F5 GTM, Citrix, Route53) solve real problems but come with SaaS pricing, vendor lock-in, and a fundamental limitation: they measure latency from their edge PoPs or via LDNS probing, not from actual clients. Plus, many regulated industries can't send DNS queries to external services.

**What OpenGSLB Does:**
- Intelligent DNS routing across regions with 6 algorithms: round-robin, weighted, failover, geolocation, latency-based, and learned latency
- Health checking with HTTP/HTTPS/TCP probes, predictive monitoring (CPU, memory spikes), and agent-based reporting
- Single binary, no dependencies, runs anywhere (Linux, Windows, Docker)
- Hot-reload configuration, Prometheus metrics, management API

**The Unique Part (Passive Latency Learning):**
Most GSLB products measure latency from their proxy/edge perspective. OpenGSLB takes a different approach: agents on your application servers read TCP RTT data directly from the kernel (Linux netlink INET_DIAG, Windows GetPerTcpConnectionEStats) for existing connections. This means we're learning the actual client-to-backend latency your users experience, not a proxy's view of it.

The agents gossip this data to the DNS servers, which route clients to the backend that has historically served their subnet with the lowest latency. It's like building a global latency map from real traffic.

**Architecture:**
Instead of complex Raft consensus or VRRP for HA, OpenGSLB uses a simpler agent-overwatch model. Agents run on app servers and gossip health status to any available Overwatch (DNS server). No leader election, no cluster coordination—DNS clients already retry across nameservers, so we leverage that built-in redundancy.

**Self-hosting wins:**
- Data stays on your infrastructure (important for regulated industries)
- No per-query billing
- Full control over routing decisions
- Works without internet connectivity to any vendor

Current release is v1.1.9. Written in Go, dual-licensed (AGPLv3 + commercial). There are 6 interactive demos from standalone setup to multi-region Azure deployments.

Would love feedback from anyone running multi-region infrastructure or dealing with similar routing challenges.

GitHub: https://github.com/LoganRossUS/OpenGSLB
Docs: https://docs.opengslb.org
```

---

## Publishing Steps for Hacker News

### Step 1: Prepare Your Account
- You need a Hacker News account at https://news.ycombinator.com
- Accounts with some history perform better (if you're new, participate in discussions for a few days first)
- Your profile should have a brief bio - this builds trust

### Step 2: Choose the Right Time
**Best times to post on HN (all times US Eastern):**
- **Optimal:** Tuesday-Thursday, 8:00-10:00 AM ET
- **Good:** Monday-Friday, 7:00 AM - 12:00 PM ET
- **Avoid:** Weekends, late evenings, major holidays

The front page algorithm favors early engagement, so posting when the US tech crowd is starting their day maximizes initial upvotes.

### Step 3: Submit the Post
1. Go to https://news.ycombinator.com/submit
2. Enter the title (use "Show HN:" prefix for project launches)
3. Either:
   - **URL:** Link to GitHub repo (https://github.com/LoganRossUS/OpenGSLB)
   - **Text:** Paste the post text above

   **Recommendation:** Use the text post. Show HN posts with explanatory text often perform better for project launches because you can explain the unique value proposition directly.

4. Submit and immediately add a first comment with technical details or "Ask me anything"

### Step 4: Engage Authentically
- **Respond to every comment** in the first few hours - this is critical for HN success
- Be honest about limitations and roadmap
- Accept constructive criticism gracefully
- If someone points out a similar project, acknowledge it and explain differentiation
- Don't get defensive about architecture choices - explain the tradeoffs

### Step 5: What to Expect
- The `/newest` page shows recent submissions - your post starts there
- If you get a few upvotes quickly, you'll hit the front page
- Front page position depends on upvotes vs. time
- Even 20-30 upvotes can mean significant traffic
- Expect technical questions about Go choice, DNS handling, security model

---

## Potential Questions & Prepared Answers

**Q: Why not just use CoreDNS with a plugin?**
> CoreDNS is excellent for DNS serving but isn't designed for GSLB. You'd need to build health checking, latency measurement, weighted routing, and failover yourself. OpenGSLB provides these as integrated, tested components.

**Q: How does this compare to external-dns for Kubernetes?**
> external-dns syncs Kubernetes resources to DNS providers. OpenGSLB *is* the DNS provider with intelligent routing. They're complementary - you could use external-dns to register services with OpenGSLB.

**Q: What about DNSSEC?**
> Supported. Automatic key management and zone signing built-in.

**Q: Is it production-ready?**
> v1.1.9 is our stable release. We have comprehensive test coverage, 18 architecture decision records documenting design choices, and 6 working demos. That said, we'd love production feedback to continue hardening.

**Q: Why Go?**
> Single binary deployment, excellent network performance, strong concurrency primitives, and the miekg/dns library (same one CoreDNS/Kubernetes uses) is battle-tested.

**Q: AGPL scares me. What if I need to deploy internally?**
> Internal use doesn't trigger AGPL sharing requirements. AGPL only requires source sharing if you're providing the service to external parties. For truly proprietary integrations, commercial licensing is available.

---

## Follow-Up Posts for Other Platforms

### r/selfhosted (Reddit)
```
Title: OpenGSLB - Self-hosted global load balancing for multi-region deployments

Just released v1.1.9 of OpenGSLB, an open-source GSLB system for self-hosters who want to route traffic intelligently across multiple servers or regions.

If you're running services in multiple locations and want DNS-based load balancing without paying for Route53 or Cloudflare load balancing, this might interest you.

Features:
- 6 routing algorithms (round-robin, weighted, failover, geo, latency, learned latency)
- Health checking (HTTP/HTTPS/TCP)
- Single Go binary, runs on Linux/Windows/Docker
- Hot-reload config, Prometheus metrics

What makes it different: It learns actual client-to-backend latency from real TCP connections, not from proxy measurements. Your agents report RTT data back to the DNS servers for smarter routing decisions.

GitHub: [link]
Docs: [link]
Docker: ghcr.io/loganrossus/opengslb:latest

Happy to answer questions!
```

### r/devops (Reddit)
```
Title: OpenGSLB: Open-source GSLB with passive latency learning - looking for feedback

Built an open-source alternative to F5 GTM/Route53 for teams managing multi-region infrastructure who want to own their traffic routing decisions.

The interesting technical bit: instead of measuring latency from edge PoPs, agents on your app servers read TCP RTT from the kernel for actual connections. This data gets gossiped to DNS servers for routing decisions based on real client experience.

Architecture avoids Raft/VRRP complexity - DNS clients already have failover built in via multiple nameservers, so we leverage that instead of reinventing HA.

v1.1.9 stable, written in Go, AGPLv3 + commercial dual-licensed.

Looking for feedback from anyone dealing with multi-region routing challenges.

[links]
```

---

## Tips for Success

1. **Don't ask for upvotes** - This violates HN guidelines and can get you banned
2. **Be genuinely helpful** - Answer questions thoroughly
3. **Acknowledge competitors** - Show you understand the landscape
4. **Share the journey** - HN loves hearing about technical challenges and decisions
5. **Follow up** - If you get useful feedback, implement it and share updates
6. **Cross-post strategically** - Wait a day or two between platforms to sustain momentum

Good luck with the launch! 🚀
