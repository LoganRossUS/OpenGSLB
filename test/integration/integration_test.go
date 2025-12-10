func createTestConfig() (string, error) {
	config := `
mode: overwatch

dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

gossip:
  encryption_key: "dGhpcy1pcy1hLTMyLWJ5dGUtdGVzdC1rZXkh"
  bind_address: "127.0.0.1:17946"

regions:
  # Single region with all servers - avoids duplicate registration
  # Different domains use different routing algorithms on the same pool
  - name: test-region
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 300
      - address: "172.28.0.3"
        port: 80
        weight: 100
      - address: "172.28.0.4"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  # TCP health check region (unique server)
  - name: tcp-region
    servers:
      - address: "172.28.0.5"
        port: 9000
        weight: 100
    health_check:
      type: tcp
      interval: 2s
      timeout: 1s
      failure_threshold: 2
      success_threshold: 1

domains:
  - name: roundrobin.test
    routing_algorithm: round-robin
    regions:
      - test-region
    ttl: 10

  - name: weighted.test
    routing_algorithm: weighted
    regions:
      - test-region
    ttl: 10

  - name: failover.test
    routing_algorithm: failover
    regions:
      - test-region
    ttl: 10

  - name: tcp.test
    routing_algorithm: round-robin
    regions:
      - tcp-region
    ttl: 10

logging:
  level: info
  format: text

metrics:
  enabled: true
  address: "127.0.0.1:19090"

api:
  enabled: true
  address: "127.0.0.1:18080"
  allowed_networks:
    - "127.0.0.0/8"
    - "172.28.0.0/16"
`

	tmpFile, err := os.CreateTemp("", "opengslb-test-*.yaml")
	if err != nil {
		return "", err
	}

	if _, err := tmpFile.WriteString(config); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}