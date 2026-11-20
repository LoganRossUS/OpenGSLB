# OpenGSLB lab on Azure

One Ubuntu VM running a small, complete OpenGSLB setup in Docker, managed with Overlord. For trying the
features by hand; not hardened and not for production.

## What you get

| Component | Detail |
|---|---|
| Overlord | Web portal on port 3001, admin account created for you, both overwatches already registered |
| overwatch-1, overwatch-2 | Two independent overwatch nodes (identities us-east and eu-west). DNS on ports 8053 and 8054 |
| agent-ap-1 | An agent beside the ap-southeast backend, reporting its health to both overwatches over gossip |
| Web backends | Seven nginx containers in three regions. Each answers `Hello from <name> (<region>)` |

Zone `gslb.lab` with these domains:

| Domain | Routing | Backends |
|---|---|---|
| `app.gslb.lab` | round-robin | web-us-east-1, web-eu-west-1 |
| `failover.gslb.lab` | failover (us-east first) | web-us-east-2, web-eu-west-2 |
| `weighted.gslb.lab` | weighted (300 : 100) | web-us-east-3, web-eu-west-3 |
| `ap.gslb.lab` | round-robin, agent-registered | web-ap-1 |

Backends use private addresses on the VM's Docker network (`172.30.0.0/24`). DNS answers are reachable from
your machine, but the backend addresses in them are not; use the commands below to fetch from the VM.

## Deploy

1. Find your public IP address (search "what is my IP"). Only that address can reach the lab.
2. Have a GitHub token that can read `LoganRossUS/Overlord` (a fine-grained token with Contents: read-only).
3. Open [Azure Cloud Shell](https://shell.azure.com) in Bash and run:

```bash
git clone --depth 1 -b chore/azure-lab https://github.com/LoganRossUS/OpenGSLB.git
```

```bash
cd OpenGSLB/deploy/azure-lab && ./deploy.sh --allowed-ip <your-ip>
```

The script asks for the GitHub token, creates resource group `opengslb-lab` with one VM, builds both projects
on the VM and prints the Overlord URL and the admin login. The first run takes about 15 to 20 minutes, most
of it building images. Options: `--location`, `--resource-group`, `--vm-size`, `--overlord-ref` (see
`./deploy.sh --help`).

The token is used only inside Cloud Shell to clone Overlord; it is not copied to the VM. The admin password
and the other lab secrets are generated on the VM and kept in `/opt/opengslb-lab/lab/runtime/.env`.

## Things to try

- **Nodes**: both overwatches healthy, with version and polling.
- **Domains / Servers**: the four domains and their backends. Create a domain or server in Overlord: it is
  pushed to both overwatches. Then query both nodes to see whether they agree.
- **DNS**, from your machine:

```bash
dig @<vm-ip> -p 8053 app.gslb.lab +short
```

On Windows, use TCP (`-vc`). In testing, Windows `nslookup` UDP queries to a non-standard port timed out
and never reached the VM, while a raw UDP DNS query from PowerShell to the same port was answered. `-type=A`
and the trailing dot avoid the AAAA lookup and the search-suffix lookups:

```powershell
nslookup -vc -type=A -port=8053 app.gslb.lab. <vm-ip>
```

- **Failover**: stop the primary and watch `failover.gslb.lab` move to eu-west within about 20 seconds:

```bash
az vm run-command invoke -g opengslb-lab -n opengslb-lab --command-id RunShellScript --scripts "cd /opt/opengslb-lab/lab && docker compose stop web-us-east-2"
```

Start it again with `docker compose start web-us-east-2`.

- **Overwatch API directly** (from the VM): `curl -s http://172.30.0.11:8080/api/v1/overwatch/backends`.

## Known gaps you will see

These are existing issues recorded in `docs/api-alignment.md`, not lab problems:

- Some Overlord pages (Health, Overrides, Geo Mappings, DNSSEC) show empty placeholder data.
- A routing algorithm chosen when creating a domain in Overlord is not applied on the nodes (M5).
- Deleting a server in Overlord does not remove it from the nodes (M7); editing a server Overlord created
  fails on the nodes (M15).
- Overlord polls agents directly, which fails, so agents are not shown as nodes (M11). The agent's backend
  appears under Servers.
- DNSSEC is not active in this build.
- An AAAA query for these IPv4-only names answers SERVFAIL instead of an empty answer, and reverse (PTR)
  lookups answer NOTIMP. Tools that ask for both A and AAAA, such as Windows `nslookup` without `-type=A`,
  may report an error even though the A answer is fine. Phase 1 fixes this.

## Remove

```bash
az group delete -n opengslb-lab --yes
```

The VM is billed while it exists, including when idle.

## Files

| File | Purpose |
|---|---|
| `deploy.sh` | Runs in Cloud Shell: creates the VM, copies the lab, starts it |
| `cloud-init.yaml` | Installs Docker on the VM |
| `lab/docker-compose.yml` | The lab's containers and network |
| `lab/templates/` | Overwatch and agent configuration, rendered with generated secrets |
| `lab/up.sh` | Runs on the VM: builds images, starts the lab, creates the admin account and registers the nodes |
