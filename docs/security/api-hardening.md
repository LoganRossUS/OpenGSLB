# API Hardening Guide

This guide covers securing the OpenGSLB API for production deployments.

## Threat Model

The API exposes:
- Server IP addresses and ports (infrastructure topology)
- Region names (organizational structure)
- Health states and failure messages (operational status)
- Timing information (maintenance windows, patterns)

Potential threats:
- Reconnaissance by external attackers
- Lateral movement after initial compromise
- Insider threats mapping infrastructure

## Defense Layers

### Layer 1: Network Binding (Default)

By default, OpenGSLB binds to `127.0.0.1:8080`. This is the strongest default—the API is only accessible from the local machine.

```yaml
api:
  enabled: true
  address: "127.0.0.1:8080"
```

Access via SSH tunnel:
```bash
ssh -L 8080:localhost:8080 user@opengslb-server
curl http://localhost:8080/api/v1/health/servers
```

### Layer 2: IP-Based ACL

For network access without a reverse proxy, use the built-in ACL:

```yaml
api:
  enabled: true
  address: "0.0.0.0:8080"
  allowed_networks:
    - "10.0.0.0/8"          # Internal network
    - "192.168.100.50/32"   # Monitoring server
```

The ACL is enforced before any request processing. Denied requests receive a `403 Forbidden` with no additional information.

### Layer 3: Reverse Proxy (Recommended for Production)

For production deployments requiring authentication, use a reverse proxy.

#### NGINX with Basic Auth

```nginx
upstream opengslb_api {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl;
    server_name gslb-api.example.com;

    ssl_certificate /etc/nginx/ssl/api.crt;
    ssl_certificate_key /etc/nginx/ssl/api.key;

    # Basic authentication
    auth_basic "OpenGSLB API";
    auth_basic_user_file /etc/nginx/htpasswd;

    location /api/ {
        proxy_pass http://opengslb_api;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Deny access to other paths
    location / {
        return 404;
    }
}
```

Create htpasswd file:
```bash
htpasswd -c /etc/nginx/htpasswd admin
```

OpenGSLB configuration:
```yaml
api:
  enabled: true
  address: "127.0.0.1:8080"
  allowed_networks:
    - "127.0.0.1/32"
  trust_proxy_headers: true  # Trust NGINX headers
```

#### NGINX with Client Certificates (mTLS)

```nginx
server {
    listen 443 ssl;
    server_name gslb-api.example.com;

    ssl_certificate /etc/nginx/ssl/server.crt;
    ssl_certificate_key /etc/nginx/ssl/server.key;
    
    # Client certificate verification
    ssl_client_certificate /etc/nginx/ssl/client-ca.crt;
    ssl_verify_client on;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header X-Client-DN $ssl_client_s_dn;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

#### HAProxy with Basic Auth

```haproxy
frontend api_frontend
    bind *:443 ssl crt /etc/haproxy/certs/api.pem
    
    # Basic auth
    acl auth_ok http_auth(api_users)
    http-request auth realm "OpenGSLB API" unless auth_ok
    
    use_backend opengslb_api

backend opengslb_api
    server local 127.0.0.1:8080

userlist api_users
    user admin password $6$rounds=5000$...  # mkpasswd -m sha-512
```

#### OAuth2 Proxy

For SSO integration:

```yaml
# oauth2-proxy config
provider = "google"  # or oidc, azure, etc.
email_domain = "example.com"
upstream = "http://127.0.0.1:8080"
```

## Configuration Recommendations

### Minimal Production Config

```yaml
api:
  enabled: true
  address: "127.0.0.1:8080"
  allowed_networks:
    - "127.0.0.1/32"
  trust_proxy_headers: true
```

With NGINX/HAProxy handling:
- TLS termination
- Authentication (basic, mTLS, or OAuth2)
- Rate limiting
- Access logging

### Air-Gapped / High Security

```yaml
api:
  enabled: true
  address: "127.0.0.1:8080"
  allowed_networks:
    - "127.0.0.1/32"
  trust_proxy_headers: false
```

Access only via SSH with key-based authentication.

### Internal Monitoring Network

```yaml
api:
  enabled: true
  address: "10.100.0.5:8080"
  allowed_networks:
    - "10.100.0.0/24"  # Monitoring VLAN only
  trust_proxy_headers: false
```

Combined with network-level controls (firewall, VLANs).

## Firewall Rules

### iptables

```bash
# Allow only monitoring network
iptables -A INPUT -p tcp --dport 8080 -s 10.100.0.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP
```

### firewalld

```bash
firewall-cmd --permanent --new-zone=gslb-api
firewall-cmd --permanent --zone=gslb-api --add-source=10.100.0.0/24
firewall-cmd --permanent --zone=gslb-api --add-port=8080/tcp
firewall-cmd --reload
```

## Logging and Auditing

API requests are logged at DEBUG level:

```yaml
logging:
  level: debug  # Enables API request logging
  format: json  # Structured logs for SIEM
```

Log format:
```json
{
  "time": "2025-01-15T10:30:00Z",
  "level": "DEBUG",
  "msg": "api request",
  "method": "GET",
  "path": "/api/v1/health/servers",
  "status": 200,
  "duration_ms": 5,
  "remote_addr": "192.168.1.100:45678"
}
```

ACL denials are logged at WARN level:
```json
{
  "time": "2025-01-15T10:30:00Z",
  "level": "WARN",
  "msg": "access denied by ACL",
  "client_ip": "10.0.0.1",
  "path": "/api/v1/health/servers"
}
```

## Checklist

- [ ] API bound to localhost or specific interface
- [ ] `allowed_networks` restricted to necessary IPs
- [ ] `trust_proxy_headers` only enabled behind trusted proxy
- [ ] Reverse proxy handles authentication
- [ ] TLS encryption for any network access
- [ ] Firewall rules as defense-in-depth
- [ ] API access logged and monitored
- [ ] Regular review of allowed_networks