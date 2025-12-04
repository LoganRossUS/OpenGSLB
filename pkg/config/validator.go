package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// ValidationError contains details about a configuration validation failure.
type ValidationError struct {
	Field   string
	Value   any
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s (got: %v)", e.Field, e.Message, e.Value)
}

// Validate checks the configuration for errors and returns a combined error if any are found.
func Validate(cfg *Config) error {
	var errs []error

	errs = append(errs, validateDNS(&cfg.DNS)...)
	errs = append(errs, validateRegions(cfg.Regions)...)
	errs = append(errs, validateDomains(cfg.Domains, cfg.Regions)...)
	errs = append(errs, validateLogging(&cfg.Logging)...)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func validateDNS(dns *DNSConfig) []error {
	var errs []error

	if dns.ListenAddress == "" {
		errs = append(errs, &ValidationError{
			Field:   "dns.listen_address",
			Value:   dns.ListenAddress,
			Message: "cannot be empty",
		})
	} else {
		host, port, err := net.SplitHostPort(dns.ListenAddress)
		if err != nil {
			errs = append(errs, &ValidationError{
				Field:   "dns.listen_address",
				Value:   dns.ListenAddress,
				Message: fmt.Sprintf("invalid address format: %v", err),
			})
		} else if host != "" {
			if ip := net.ParseIP(host); ip == nil {
				errs = append(errs, &ValidationError{
					Field:   "dns.listen_address",
					Value:   dns.ListenAddress,
					Message: "invalid IP address",
				})
			}
		}
		_ = port
	}

	if dns.DefaultTTL < 1 || dns.DefaultTTL > 86400 {
		errs = append(errs, &ValidationError{
			Field:   "dns.default_ttl",
			Value:   dns.DefaultTTL,
			Message: "must be between 1 and 86400 seconds",
		})
	}

	return errs
}

func validateRegions(regions []Region) []error {
	var errs []error

	if len(regions) == 0 {
		errs = append(errs, &ValidationError{
			Field:   "regions",
			Value:   nil,
			Message: "at least one region must be defined",
		})
		return errs
	}

	seen := make(map[string]bool)
	for i := range regions {
		r := &regions[i]
		prefix := fmt.Sprintf("regions[%d]", i)

		if r.Name == "" {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Value:   r.Name,
				Message: "cannot be empty",
			})
		} else if seen[r.Name] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Value:   r.Name,
				Message: "duplicate region name",
			})
		} else {
			seen[r.Name] = true
		}

		errs = append(errs, validateServers(r.Servers, prefix)...)
		errs = append(errs, validateHealthCheck(&r.HealthCheck, prefix)...)
	}

	return errs
}

func validateServers(servers []Server, prefix string) []error {
	var errs []error

	if len(servers) == 0 {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".servers",
			Value:   nil,
			Message: "at least one server must be defined",
		})
		return errs
	}

	for i, s := range servers {
		sPrefix := fmt.Sprintf("%s.servers[%d]", prefix, i)

		if s.Address == "" {
			errs = append(errs, &ValidationError{
				Field:   sPrefix + ".address",
				Value:   s.Address,
				Message: "cannot be empty",
			})
		} else if ip := net.ParseIP(s.Address); ip == nil {
			errs = append(errs, &ValidationError{
				Field:   sPrefix + ".address",
				Value:   s.Address,
				Message: "invalid IP address",
			})
		}

		if s.Port < 1 || s.Port > 65535 {
			errs = append(errs, &ValidationError{
				Field:   sPrefix + ".port",
				Value:   s.Port,
				Message: "must be between 1 and 65535",
			})
		}

		if s.Weight < 1 || s.Weight > 1000 {
			errs = append(errs, &ValidationError{
				Field:   sPrefix + ".weight",
				Value:   s.Weight,
				Message: "must be between 1 and 1000",
			})
		}
	}

	return errs
}

func validateHealthCheck(hc *HealthCheck, prefix string) []error {
	var errs []error
	hcPrefix := prefix + ".health_check"

	validTypes := map[string]bool{"http": true, "https": true, "tcp": true}
	if !validTypes[hc.Type] {
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".type",
			Value:   hc.Type,
			Message: "must be one of: http, https, tcp",
		})
	}

	if hc.Interval < 1_000_000_000 { // 1 second in nanoseconds
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".interval",
			Value:   hc.Interval,
			Message: "must be at least 1 second",
		})
	}

	if hc.Timeout < 100_000_000 { // 100ms in nanoseconds
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".timeout",
			Value:   hc.Timeout,
			Message: "must be at least 100ms",
		})
	}

	if hc.Timeout >= hc.Interval {
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".timeout",
			Value:   hc.Timeout,
			Message: "must be less than interval",
		})
	}

	if (hc.Type == "http" || hc.Type == "https") && !strings.HasPrefix(hc.Path, "/") {
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".path",
			Value:   hc.Path,
			Message: "must start with /",
		})
	}

	if hc.FailureThreshold < 1 || hc.FailureThreshold > 10 {
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".failure_threshold",
			Value:   hc.FailureThreshold,
			Message: "must be between 1 and 10",
		})
	}

	if hc.SuccessThreshold < 1 || hc.SuccessThreshold > 10 {
		errs = append(errs, &ValidationError{
			Field:   hcPrefix + ".success_threshold",
			Value:   hc.SuccessThreshold,
			Message: "must be between 1 and 10",
		})
	}

	return errs
}

func validateDomains(domains []Domain, regions []Region) []error {
	var errs []error

	if len(domains) == 0 {
		errs = append(errs, &ValidationError{
			Field:   "domains",
			Value:   nil,
			Message: "at least one domain must be defined",
		})
		return errs
	}

	regionNames := make(map[string]bool)
	for _, r := range regions {
		regionNames[r.Name] = true
	}

	validAlgorithms := map[string]bool{
		"round-robin": true,
		"weighted":    true,
		"geolocation": true,
		"latency":     true,
		"failover":    true,
	}

	seen := make(map[string]bool)
	for i, d := range domains {
		prefix := fmt.Sprintf("domains[%d]", i)

		if d.Name == "" {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Value:   d.Name,
				Message: "cannot be empty",
			})
		} else if seen[d.Name] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Value:   d.Name,
				Message: "duplicate domain name",
			})
		} else {
			seen[d.Name] = true
		}

		if !validAlgorithms[d.RoutingAlgorithm] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".routing_algorithm",
				Value:   d.RoutingAlgorithm,
				Message: "must be one of: round-robin, weighted, geolocation, latency, failover",
			})
		}

		if len(d.Regions) == 0 {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".regions",
				Value:   nil,
				Message: "at least one region must be specified",
			})
		}

		for j, rName := range d.Regions {
			if !regionNames[rName] {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("%s.regions[%d]", prefix, j),
					Value:   rName,
					Message: "references undefined region",
				})
			}
		}

		if d.TTL < 1 || d.TTL > 86400 {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".ttl",
				Value:   d.TTL,
				Message: "must be between 1 and 86400 seconds",
			})
		}
	}

	return errs
}

func validateLogging(logging *LoggingConfig) []error {
	var errs []error

	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[logging.Level] {
		errs = append(errs, &ValidationError{
			Field:   "logging.level",
			Value:   logging.Level,
			Message: "must be one of: debug, info, warn, error",
		})
	}

	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[logging.Format] {
		errs = append(errs, &ValidationError{
			Field:   "logging.format",
			Value:   logging.Format,
			Message: "must be one of: json, text",
		})
	}

	return errs
}
