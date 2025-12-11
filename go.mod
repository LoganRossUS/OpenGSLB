module github.com/loganrossus/OpenGSLB

go 1.23.0

require (
	github.com/miekg/dns v1.1.68
	github.com/prometheus/client_golang v1.23.2
	go.etcd.io/bbolt v1.3.5
	gopkg.in/yaml.v3 v3.0.1
)

// ADR-015: Removed Raft dependencies
// - github.com/hashicorp/raft
// - github.com/hashicorp/raft-boltdb/v2

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)
