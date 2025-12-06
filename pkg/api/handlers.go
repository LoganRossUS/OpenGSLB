package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// HealthProvider is the interface for retrieving health status.
// This matches the health.Manager's capabilities.
type HealthProvider interface {
	GetAllStatus() []health.Snapshot
	ServerCount() int
}

// ReadinessChecker provides readiness status for the application.
type ReadinessChecker interface {
	IsDNSReady() bool
	IsHealthCheckReady() bool
}

// RegionMapper maps server addresses to their region names.
type RegionMapper interface {
	GetServerRegion(address string, port int) string
}

// Handlers contains all API endpoint handlers.
type Handlers struct {
	healthProvider   HealthProvider
	readinessChecker ReadinessChecker
	regionMapper     RegionMapper
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(hp HealthProvider, rc ReadinessChecker, rm RegionMapper) *Handlers {
	return &Handlers{
		healthProvider:   hp,
		readinessChecker: rc,
		regionMapper:     rm,
	}
}

// HealthServers handles GET /api/v1/health/servers
func (h *Handlers) HealthServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	snapshots := h.healthProvider.GetAllStatus()

	servers := make([]ServerHealthResponse, 0, len(snapshots))
	for _, snap := range snapshots {
		addr, port := parseAddress(snap.Address)

		var region string
		if h.regionMapper != nil {
			region = h.regionMapper.GetServerRegion(addr, port)
		}

		srv := ServerHealthResponse{
			Address:              addr,
			Port:                 port,
			Region:               region,
			Healthy:              snap.Status == health.StatusHealthy,
			Status:               snap.Status.String(),
			ConsecutiveFailures:  snap.ConsecutiveFails,
			ConsecutiveSuccesses: snap.ConsecutivePasses,
		}

		if !snap.LastCheck.IsZero() {
			srv.LastCheck = &snap.LastCheck
		}
		if !snap.LastHealthy.IsZero() {
			srv.LastHealthy = &snap.LastHealthy
		}
		if snap.LastError != nil {
			srv.LastError = snap.LastError.Error()
		}

		servers = append(servers, srv)
	}

	resp := HealthResponse{
		Servers:     servers,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// HealthRegions handles GET /api/v1/health/regions
func (h *Handlers) HealthRegions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.regionMapper == nil {
		writeError(w, http.StatusInternalServerError, "region mapping not available")
		return
	}

	snapshots := h.healthProvider.GetAllStatus()

	// Aggregate by region
	regionStats := make(map[string]*RegionHealthResponse)

	for _, snap := range snapshots {
		addr, port := parseAddress(snap.Address)
		region := h.regionMapper.GetServerRegion(addr, port)
		if region == "" {
			region = "unknown"
		}

		stats, exists := regionStats[region]
		if !exists {
			stats = &RegionHealthResponse{Name: region}
			regionStats[region] = stats
		}

		stats.TotalServers++
		if snap.Status == health.StatusHealthy {
			stats.HealthyCount++
		} else {
			stats.UnhealthyCount++
		}
	}

	// Calculate percentages and build response
	regions := make([]RegionHealthResponse, 0, len(regionStats))
	for _, stats := range regionStats {
		if stats.TotalServers > 0 {
			stats.HealthPercent = float64(stats.HealthyCount) / float64(stats.TotalServers) * 100
		}
		regions = append(regions, *stats)
	}

	resp := RegionsResponse{
		Regions:     regions,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// Ready handles GET /api/v1/ready
func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dnsReady := h.readinessChecker.IsDNSReady()
	healthReady := h.readinessChecker.IsHealthCheckReady()

	ready := dnsReady && healthReady

	resp := ReadyResponse{
		Ready:       ready,
		DNSReady:    dnsReady,
		HealthReady: healthReady,
	}

	if !ready {
		var reasons []string
		if !dnsReady {
			reasons = append(reasons, "DNS server not ready")
		}
		if !healthReady {
			reasons = append(reasons, "health checks not ready")
		}
		resp.Message = strings.Join(reasons, "; ")
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, resp)
}

// Live handles GET /api/v1/live
func (h *Handlers) Live(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// If we can handle the request, we're alive
	writeJSON(w, http.StatusOK, LiveResponse{Alive: true})
}

// parseAddress splits "addr:port" into address and port.
func parseAddress(addrPort string) (string, int) {
	parts := strings.Split(addrPort, ":")
	if len(parts) != 2 {
		return addrPort, 0
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return parts[0], 0
	}
	return parts[0], port
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Can't do much here, response already started
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
