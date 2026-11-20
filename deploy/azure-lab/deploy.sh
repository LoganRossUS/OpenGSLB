#!/usr/bin/env bash
# Deploys the OpenGSLB lab to one Azure VM and prints the Overlord URL.
# Run it from Azure Cloud Shell (Bash), inside a clone of the OpenGSLB repository:
#
#   ./deploy.sh --allowed-ip <your public IP>
#
# Options:
#   --allowed-ip IP[/N]   Required. Only this address (or range) can reach the lab.
#   --location NAME       Azure region (default: eastus)
#   --resource-group NAME Resource group to create (default: opengslb-lab)
#   --vm-size SIZE        VM size (default: Standard_B2ms)
#   --overlord-ref REF    Overlord branch or tag to build (default: main)
#
# Overlord is a private repository: the script asks for a GitHub token with
# read access to LoganRossUS/Overlord (or reads it from GITHUB_TOKEN). The
# token is used only in Cloud Shell to clone; it is never sent to the VM.
#
# Remove everything afterwards with:  az group delete -n opengslb-lab --yes
set -euo pipefail

ALLOWED_IP="" LOCATION="eastus" RG="opengslb-lab" VM_SIZE="Standard_B2ms" OVERLORD_REF="main"
while [ $# -gt 0 ]; do
  case "$1" in
    --allowed-ip)     ALLOWED_IP="$2"; shift 2 ;;
    --location)       LOCATION="$2"; shift 2 ;;
    --resource-group) RG="$2"; shift 2 ;;
    --vm-size)        VM_SIZE="$2"; shift 2 ;;
    --overlord-ref)   OVERLORD_REF="$2"; shift 2 ;;
    -h|--help)        sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "Unknown option: $1 (see --help)" >&2; exit 1 ;;
  esac
done

die() { echo "ERROR: $*" >&2; exit 1; }
step() { echo; echo "==> $*"; }

[ -n "$ALLOWED_IP" ] || die "--allowed-ip is required: your own public IP (search 'what is my IP'). Cloud Shell's address is not yours."
[[ "$ALLOWED_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}(/[0-9]{1,2})?$ ]] || die "--allowed-ip must be an IPv4 address or range, e.g. 203.0.113.7"
[[ "$ALLOWED_IP" == */* ]] || ALLOWED_IP="$ALLOWED_IP/32"
[ "$ALLOWED_IP" != "0.0.0.0/0" ] || die "refusing to open the lab to the whole internet"
command -v az >/dev/null || die "the Azure CLI is not installed (use Azure Cloud Shell)"
az account show -o none 2>/dev/null || die "not signed in to Azure: run az login"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
VM="opengslb-lab"
NSG="$VM-nsg"
USER_NAME="azureuser"
SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 -o ServerAliveInterval=30)

echo "Subscription:   $(az account show --query name -o tsv)"
echo "Resource group: $RG ($LOCATION), VM size $VM_SIZE"
echo "Allowed client: $ALLOWED_IP"

# --- Sources ---------------------------------------------------------------------
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

step "Packaging OpenGSLB from this checkout ($(git -C "$REPO_ROOT" rev-parse --short HEAD))"
mkdir -p "$WORK/bundle/src"
git -C "$REPO_ROOT" archive --format=tar --prefix=opengslb/ HEAD | tar -x -C "$WORK/bundle/src"
cp -r "$SCRIPT_DIR/lab" "$WORK/bundle/lab"
rm -rf "$WORK/bundle/lab/runtime"

step "Cloning Overlord ($OVERLORD_REF)"
if [ -z "${GITHUB_TOKEN:-}" ]; then
  read -rsp "GitHub token with read access to LoganRossUS/Overlord: " GITHUB_TOKEN; echo
fi
GITHUB_TOKEN="$(printf '%s' "$GITHUB_TOKEN" | tr -d '[:space:]')"
[ -n "$GITHUB_TOKEN" ] || die "no token entered"
# Check the token before cloning, so a bad token gets a clear message.
status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $GITHUB_TOKEN" \
  -H 'Accept: application/vnd.github+json' https://api.github.com/repos/LoganRossUS/Overlord)"
case "$status" in
  200) ;;
  401) die "GitHub rejected the token (HTTP 401): it is mistyped, expired or revoked. Use a GitHub token (github_pat_... or ghp_...), not the value of a repository secret you generated yourself." ;;
  403|404) die "the token is valid but cannot read LoganRossUS/Overlord (HTTP $status). For a fine-grained token: Resource owner LoganRossUS, Repository access includes Overlord, Contents: Read-only." ;;
  *) die "could not check the token with GitHub (HTTP $status)" ;;
esac
auth="$(printf 'x-access-token:%s' "$GITHUB_TOKEN" | base64 | tr -d '\n')"
GIT_TERMINAL_PROMPT=0 git -c credential.helper= -c http.extraHeader="Authorization: Basic $auth" \
  clone -q --depth 1 --branch "$OVERLORD_REF" \
  https://github.com/LoganRossUS/Overlord.git "$WORK/bundle/src/overlord" \
  || die "could not clone LoganRossUS/Overlord (branch $OVERLORD_REF) with that token"
unset GITHUB_TOKEN auth
rm -rf "$WORK/bundle/src/overlord/.git"
tar -czf "$WORK/lab.tgz" -C "$WORK/bundle" .

# --- Azure resources ------------------------------------------------------------------
DEPLOYER_IP="$(curl -fsS https://api.ipify.org)" || die "could not determine Cloud Shell's public IP"

step "Creating resource group and network security group"
az group create -n "$RG" -l "$LOCATION" -o none
az network nsg create -g "$RG" -n "$NSG" -o none
# Delete before create so a re-run applies changed addresses and ports.
az network nsg rule delete -g "$RG" --nsg-name "$NSG" -n allow-lab-user -o none 2>/dev/null || true
az network nsg rule create -g "$RG" --nsg-name "$NSG" -n allow-lab-user --priority 100 \
  --source-address-prefixes "$ALLOWED_IP" --destination-port-ranges 22 3001 8053 8054 \
  --protocol '*' --access Allow --direction Inbound -o none
# Temporary: lets this Cloud Shell copy the lab to the VM. Removed at the end.
az network nsg rule delete -g "$RG" --nsg-name "$NSG" -n allow-deployer-ssh -o none 2>/dev/null || true
az network nsg rule create -g "$RG" --nsg-name "$NSG" -n allow-deployer-ssh --priority 110 \
  --source-address-prefixes "$DEPLOYER_IP/32" --destination-port-ranges 22 \
  --protocol Tcp --access Allow --direction Inbound -o none

if az vm show -g "$RG" -n "$VM" -o none 2>/dev/null; then
  step "VM $VM already exists; reusing it"
else
  step "Creating VM (a few minutes)"
  az vm create -g "$RG" -n "$VM" --image Ubuntu2404 --size "$VM_SIZE" \
    --admin-username "$USER_NAME" --generate-ssh-keys --nsg "$NSG" \
    --public-ip-sku Standard --os-disk-size-gb 64 \
    --custom-data "$SCRIPT_DIR/cloud-init.yaml" -o none
fi
IP="$(az vm show -d -g "$RG" -n "$VM" --query publicIps -o tsv)"
[ -n "$IP" ] || die "the VM has no public IP"

step "Waiting for the VM to finish installing Docker"
for _ in $(seq 1 60); do
  ssh "${SSH_OPTS[@]}" "$USER_NAME@$IP" true 2>/dev/null && break
  sleep 5
done
ssh "${SSH_OPTS[@]}" "$USER_NAME@$IP" "cloud-init status --wait >/dev/null; docker --version" \
  || die "cannot reach the VM over SSH from Cloud Shell ($DEPLOYER_IP)"

step "Copying the lab to the VM"
scp "${SSH_OPTS[@]}" -q "$WORK/lab.tgz" "$USER_NAME@$IP:/tmp/lab.tgz"
ssh "${SSH_OPTS[@]}" "$USER_NAME@$IP" \
  "sudo mkdir -p /opt/opengslb-lab && sudo tar -xzf /tmp/lab.tgz -C /opt/opengslb-lab && rm /tmp/lab.tgz"

step "Building and starting the lab on the VM (10 to 15 minutes on first run)"
ssh "${SSH_OPTS[@]}" "$USER_NAME@$IP" "sudo /opt/opengslb-lab/lab/up.sh"
CREDS="$(ssh "${SSH_OPTS[@]}" "$USER_NAME@$IP" "sudo grep '^ADMIN_' /opt/opengslb-lab/lab/runtime/.env")"

az network nsg rule delete -g "$RG" --nsg-name "$NSG" -n allow-deployer-ssh -o none

ADMIN_EMAIL="$(printf '%s\n' "$CREDS" | sed -n 's/^ADMIN_EMAIL=//p')"
ADMIN_PASSWORD="$(printf '%s\n' "$CREDS" | sed -n 's/^ADMIN_PASSWORD=//p')"
cat <<DONE

============================================================
 OpenGSLB lab is ready

 Overlord:  http://$IP:3001
 Email:     $ADMIN_EMAIL
 Password:  $ADMIN_PASSWORD

 DNS (from your machine, $ALLOWED_IP):
   dig @$IP -p 8053 app.gslb.lab        # overwatch-1
   dig @$IP -p 8054 app.gslb.lab        # overwatch-2
   nslookup -vc -type=A -port=8053 app.gslb.lab. $IP   (Windows: use TCP)
   Domains: app (round-robin), failover, weighted, ap (agent-managed)

 Shell on the VM (works from Cloud Shell; SSH itself is only open to $ALLOWED_IP):
   az vm run-command invoke -g $RG -n $VM --command-id RunShellScript \
     --scripts "cd /opt/opengslb-lab/lab && docker compose ps"
 Delete:    az group delete -n $RG --yes
============================================================
DONE
