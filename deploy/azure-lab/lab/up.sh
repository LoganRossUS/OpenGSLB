#!/usr/bin/env bash
# Builds and starts the lab on the VM, then creates the Overlord admin account
# and registers both overwatches in Overlord. Safe to run again.
#
#   sudo ./up.sh            (run from this directory; the sources are expected
#                            in ../src/opengslb and ../src/overlord)
set -euo pipefail

LAB_DIR="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$LAB_DIR/../src"
RUNTIME="$LAB_DIR/runtime"
cd "$LAB_DIR"
mkdir -p "$RUNTIME"
chmod 700 "$RUNTIME"

# --- Secrets, generated once and kept in runtime/.env -------------------------
if [ ! -f "$RUNTIME/.env" ]; then
  umask 077
  {
    echo "DB_PASSWORD=$(openssl rand -hex 24)"
    echo "SESSION_SECRET=$(openssl rand -hex 32)"
    echo "GOSSIP_KEY=$(openssl rand -base64 32)"
    echo "AGENT_TOKEN=$(openssl rand -hex 32)"
    echo "ADMIN_EMAIL=admin@opengslb.lab"
    echo "ADMIN_PASSWORD=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-20)"
  } > "$RUNTIME/.env"
  umask 022
fi
set -a; . "$RUNTIME/.env"; set +a
# So that plain "docker compose ..." in this directory finds the variables.
ln -sf runtime/.env "$LAB_DIR/.env"

render() { # template node_id region output
  sed -e "s|__NODE_ID__|$2|" -e "s|__REGION__|$3|" \
      -e "s|__AGENT_TOKEN__|$AGENT_TOKEN|" -e "s|__GOSSIP_KEY__|$GOSSIP_KEY|" \
      "$LAB_DIR/templates/$1.yaml" > "$RUNTIME/$4"
  chmod 644 "$RUNTIME/$4"   # read by the container user; copied to 0600 inside
}
render overwatch overwatch-1 us-east overwatch-1.yaml
render overwatch overwatch-2 eu-west overwatch-2.yaml
render agent     unused      unused  agent.yaml

# --- Port 53 ----------------------------------------------------------------------
# Ubuntu's systemd-resolved listens on 127.0.0.53:53, which stops Docker from
# publishing port 53. Turn its stub listener off once; the VM then resolves
# through the upstream servers in /run/systemd/resolve/resolv.conf.
if command -v systemctl >/dev/null && systemctl is-active -q systemd-resolved 2>/dev/null \
   && [ ! -f /etc/systemd/resolved.conf.d/opengslb-lab.conf ]; then
  echo "==> Freeing port 53 (systemd-resolved stub listener off)"
  mkdir -p /etc/systemd/resolved.conf.d
  printf '[Resolve]\nDNSStubListener=no\n' > /etc/systemd/resolved.conf.d/opengslb-lab.conf
  ln -sf /run/systemd/resolve/resolv.conf /etc/resolv.conf
  systemctl restart systemd-resolved
fi

# --- Images --------------------------------------------------------------------
# LAB_SKIP_SOURCE_BUILD=1 uses existing opengslb-lab and overlord-lab-base images.
if [ "${LAB_SKIP_SOURCE_BUILD:-0}" != 1 ]; then
  echo "==> Building the OpenGSLB image (several minutes on first run)"
  docker build -q -t opengslb-lab:latest "$SRC_DIR/opengslb" >/dev/null
  echo "==> Building the Overlord image (several minutes on first run)"
  docker build -q -t overlord-lab-base:latest "$SRC_DIR/overlord" >/dev/null
fi
# The Overlord front end has its API address fixed at build time and defaults
# to http://localhost:3001, which only works in a browser on the same machine.
# Blank it so the browser uses the address it loaded the page from.
docker build -q -t overlord-lab:latest - >/dev/null <<'DOCKERFILE'
FROM overlord-lab-base:latest
USER root
RUN grep -l 'http://localhost:3001' dist/assets/*.js | xargs sed -i 's#http://localhost:3001##g' \
 && ! grep -q 'http://localhost:3001' dist/assets/*.js
USER overlord
DOCKERFILE

# --- Start -----------------------------------------------------------------------
echo "==> Starting the lab"
docker compose up -d --remove-orphans

echo "==> Waiting for Overlord"
for _ in $(seq 1 90); do
  curl -fsS http://127.0.0.1:3001/api/health >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS http://127.0.0.1:3001/api/health >/dev/null

# --- Bootstrap Overlord ------------------------------------------------------------
api() { # method path [json]
  curl -fsS -X "$1" "http://127.0.0.1:3001$2" -H 'Content-Type: application/json' \
       ${TOKEN:+-H "Authorization: Bearer $TOKEN"} ${3:+-d "$3"}
}

TOKEN=""
if api GET /api/v1/auth/setup/status | grep -q '"setupRequired":true'; then
  echo "==> Creating the Overlord admin account"
  TOKEN=$(api POST /api/v1/auth/setup \
    "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\",\"displayName\":\"Lab Admin\"}" |
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
else
  TOKEN=$(api POST /api/v1/auth/login "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" |
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
fi
[ -n "$TOKEN" ] || { echo "Could not sign in to Overlord" >&2; exit 1; }

echo "==> Registering the overwatches in Overlord"
existing=$(api GET /api/v1/nodes/overwatch || true)
for n in "overwatch-1 172.30.0.11 us-east" "overwatch-2 172.30.0.12 eu-west"; do
  set -- $n
  if ! printf '%s' "$existing" | grep -q "\"$1\""; then
    api POST /api/v1/nodes "{\"nodeId\":\"$1\",\"nodeType\":\"overwatch\",\"name\":\"$1 ($3)\",\"address\":\"$2\",\"apiPort\":8080,\"metricsPort\":9090,\"region\":\"$3\"}" >/dev/null
  fi
done
api POST /api/v1/polling/trigger >/dev/null || true

echo "==> Lab is up"
