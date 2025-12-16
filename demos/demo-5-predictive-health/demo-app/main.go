// Copyright (C) 2025 Logan Ross
//
// OpenGSLB Demo Application
// A simple HTTP server with chaos injection capabilities for demonstrating
// predictive health monitoring.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ChaosEngine manages chaos injection state.
type ChaosEngine struct {
	mu sync.RWMutex

	// CPU chaos
	cpuActive   bool
	cpuCancel   context.CancelFunc
	cpuWorkers  int
	cpuDuration time.Duration

	// Memory chaos
	memActive  bool
	memBalloon []byte
	memCancel  context.CancelFunc

	// Error injection
	errorsActive bool
	errorRate    int // percentage
	errorCancel  context.CancelFunc

	// Latency injection
	latencyActive bool
	latencyMs     int
	latencyCancel context.CancelFunc

	// Metrics
	metrics *Metrics
}

// Metrics holds Prometheus metrics.
type Metrics struct {
	cpuPercent      prometheus.Gauge
	memoryPercent   prometheus.Gauge
	requestsTotal   *prometheus.CounterVec
	errorsInjected  prometheus.Counter
	chaosActive     *prometheus.GaugeVec
	responseLatency prometheus.Histogram
}

// Server is the main HTTP server.
type Server struct {
	hostname string
	chaos    *ChaosEngine
	metrics  *Metrics
	logger   *slog.Logger
	mux      *http.ServeMux
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := NewServer(hostname, logger)

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      server.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("starting demo app", "hostname", hostname, "port", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}

// NewServer creates a new demo server.
func NewServer(hostname string, logger *slog.Logger) *Server {
	metrics := newMetrics(hostname)

	s := &Server{
		hostname: hostname,
		chaos:    &ChaosEngine{metrics: metrics},
		metrics:  metrics,
		logger:   logger,
		mux:      http.NewServeMux(),
	}

	// Register handlers
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.HandleFunc("/chaos/cpu", s.handleChaosCPU)
	s.mux.HandleFunc("/chaos/memory", s.handleChaosMemory)
	s.mux.HandleFunc("/chaos/errors", s.handleChaosErrors)
	s.mux.HandleFunc("/chaos/latency", s.handleChaosLatency)
	s.mux.HandleFunc("/chaos/stop", s.handleChaosStop)
	s.mux.HandleFunc("/chaos/status", s.handleChaosStatus)

	// Start metrics collector
	go s.collectMetrics()

	return s
}

func newMetrics(hostname string) *Metrics {
	m := &Metrics{
		cpuPercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "demo_app_cpu_percent",
			Help:        "Current CPU utilization percentage",
			ConstLabels: prometheus.Labels{"hostname": hostname},
		}),
		memoryPercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "demo_app_memory_percent",
			Help:        "Current memory utilization percentage",
			ConstLabels: prometheus.Labels{"hostname": hostname},
		}),
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "demo_app_requests_total",
			Help:        "Total HTTP requests",
			ConstLabels: prometheus.Labels{"hostname": hostname},
		}, []string{"status", "path"}),
		errorsInjected: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "demo_app_errors_injected_total",
			Help:        "Total injected errors from chaos",
			ConstLabels: prometheus.Labels{"hostname": hostname},
		}),
		chaosActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "demo_app_chaos_active",
			Help:        "Whether chaos injection is active (1=active, 0=inactive)",
			ConstLabels: prometheus.Labels{"hostname": hostname},
		}, []string{"type"}),
		responseLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "demo_app_response_latency_seconds",
			Help:        "Response latency in seconds",
			ConstLabels: prometheus.Labels{"hostname": hostname},
			Buckets:     []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}),
	}

	prometheus.MustRegister(m.cpuPercent)
	prometheus.MustRegister(m.memoryPercent)
	prometheus.MustRegister(m.requestsTotal)
	prometheus.MustRegister(m.errorsInjected)
	prometheus.MustRegister(m.chaosActive)
	prometheus.MustRegister(m.responseLatency)

	// Initialize chaos gauges to 0
	for _, t := range []string{"cpu", "memory", "errors", "latency"} {
		m.chaosActive.WithLabelValues(t).Set(0)
	}

	return m
}

func (s *Server) collectMetrics() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastCPUTotal, lastCPUIdle uint64
	var lastTime time.Time

	for range ticker.C {
		// Collect CPU from /proc/stat
		if _, total, idle := readCPUStats(); total > 0 {
			if lastCPUTotal > 0 {
				totalDelta := total - lastCPUTotal
				idleDelta := idle - lastCPUIdle
				if totalDelta > 0 {
					cpuPercent := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
					s.metrics.cpuPercent.Set(cpuPercent)
				}
			}
			lastCPUTotal = total
			lastCPUIdle = idle
			lastTime = time.Now()
		}
		_ = lastTime // suppress unused warning

		// Collect memory from /proc/meminfo
		if memPercent := readMemoryPercent(); memPercent >= 0 {
			s.metrics.memoryPercent.Set(memPercent)
		}

		// Update chaos status metrics
		s.chaos.mu.RLock()
		if s.chaos.cpuActive {
			s.metrics.chaosActive.WithLabelValues("cpu").Set(1)
		} else {
			s.metrics.chaosActive.WithLabelValues("cpu").Set(0)
		}
		if s.chaos.memActive {
			s.metrics.chaosActive.WithLabelValues("memory").Set(1)
		} else {
			s.metrics.chaosActive.WithLabelValues("memory").Set(0)
		}
		if s.chaos.errorsActive {
			s.metrics.chaosActive.WithLabelValues("errors").Set(1)
		} else {
			s.metrics.chaosActive.WithLabelValues("errors").Set(0)
		}
		if s.chaos.latencyActive {
			s.metrics.chaosActive.WithLabelValues("latency").Set(1)
		} else {
			s.metrics.chaosActive.WithLabelValues("latency").Set(0)
		}
		s.chaos.mu.RUnlock()
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Apply latency if active
	s.chaos.mu.RLock()
	if s.chaos.latencyActive {
		time.Sleep(time.Duration(s.chaos.latencyMs) * time.Millisecond)
	}
	s.chaos.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>OpenGSLB Demo - %s</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
        }
        .container {
            text-align: center;
            padding: 40px;
            background: rgba(255,255,255,0.1);
            border-radius: 20px;
            backdrop-filter: blur(10px);
        }
        h1 { font-size: 2.5em; margin-bottom: 10px; }
        .hostname { font-size: 1.8em; font-weight: bold; color: #ffd700; }
        .info { margin-top: 20px; opacity: 0.8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>OpenGSLB Demo 5</h1>
        <p>Predictive Health Detection</p>
        <p class="hostname">%s</p>
        <p class="info">Response time: %s</p>
    </div>
</body>
</html>`, s.hostname, s.hostname, time.Since(start))

	s.metrics.requestsTotal.WithLabelValues("200", "/").Inc()
	s.metrics.responseLatency.Observe(time.Since(start).Seconds())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Apply latency if active
	s.chaos.mu.RLock()
	if s.chaos.latencyActive {
		time.Sleep(time.Duration(s.chaos.latencyMs) * time.Millisecond)
	}

	// Check for error injection
	if s.chaos.errorsActive && rand.Intn(100) < s.chaos.errorRate {
		s.chaos.mu.RUnlock()
		s.metrics.errorsInjected.Inc()
		s.metrics.requestsTotal.WithLabelValues("500", "/health").Inc()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "error",
			"message":  "chaos: injected error",
			"hostname": s.hostname,
		})
		return
	}
	s.chaos.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "healthy",
		"hostname": s.hostname,
		"latency":  time.Since(start).String(),
	})

	s.metrics.requestsTotal.WithLabelValues("200", "/health").Inc()
	s.metrics.responseLatency.Observe(time.Since(start).Seconds())
}

func (s *Server) handleChaosCPU(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	durationStr := r.URL.Query().Get("duration")
	if durationStr == "" {
		durationStr = "30s"
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid duration: %v", err), http.StatusBadRequest)
		return
	}

	intensityStr := r.URL.Query().Get("intensity")
	intensity := 80
	if intensityStr != "" {
		intensity, err = strconv.Atoi(intensityStr)
		if err != nil || intensity < 1 || intensity > 100 {
			http.Error(w, "intensity must be 1-100", http.StatusBadRequest)
			return
		}
	}

	s.startCPUChaos(duration, intensity)

	s.logger.Warn("chaos: CPU spike started",
		"duration", duration,
		"intensity", intensity)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "cpu_spike_started",
		"duration":  duration.String(),
		"intensity": intensity,
		"message":   fmt.Sprintf("CPU spike at %d%% intensity for %s", intensity, duration),
	})
}

func (s *Server) startCPUChaos(duration time.Duration, intensity int) {
	s.chaos.mu.Lock()
	// Stop existing CPU chaos if any
	if s.chaos.cpuCancel != nil {
		s.chaos.cpuCancel()
	}

	s.chaos.cpuActive = true
	s.chaos.cpuDuration = duration
	s.chaos.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	s.chaos.mu.Lock()
	s.chaos.cpuCancel = cancel
	s.chaos.mu.Unlock()

	numCPU := runtime.NumCPU()
	workersNeeded := (numCPU * intensity) / 100
	if workersNeeded < 1 {
		workersNeeded = 1
	}

	var stopped atomic.Bool

	for i := 0; i < workersNeeded; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if stopped.Load() {
						return
					}
					// Busy work - compute something useless
					_ = math.Sqrt(rand.Float64())
				}
			}
		}()
	}

	// Auto-stop after duration
	go func() {
		<-ctx.Done()
		stopped.Store(true)
		s.chaos.mu.Lock()
		s.chaos.cpuActive = false
		s.chaos.cpuCancel = nil
		s.chaos.mu.Unlock()
		s.logger.Info("chaos: CPU spike ended")
	}()
}

func (s *Server) handleChaosMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	durationStr := r.URL.Query().Get("duration")
	if durationStr == "" {
		durationStr = "30s"
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid duration: %v", err), http.StatusBadRequest)
		return
	}

	amountStr := r.URL.Query().Get("amount")
	amount := 500 // MB
	if amountStr != "" {
		amount, err = strconv.Atoi(amountStr)
		if err != nil || amount < 1 || amount > 4096 {
			http.Error(w, "amount must be 1-4096 MB", http.StatusBadRequest)
			return
		}
	}

	s.startMemoryChaos(duration, amount)

	s.logger.Warn("chaos: Memory pressure started",
		"duration", duration,
		"amount_mb", amount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "memory_pressure_started",
		"duration":  duration.String(),
		"amount_mb": amount,
		"message":   fmt.Sprintf("Allocating %dMB for %s", amount, duration),
	})
}

func (s *Server) startMemoryChaos(duration time.Duration, amountMB int) {
	s.chaos.mu.Lock()
	// Stop existing memory chaos if any
	if s.chaos.memCancel != nil {
		s.chaos.memCancel()
	}
	s.chaos.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), duration)

	s.chaos.mu.Lock()
	s.chaos.memActive = true
	s.chaos.memCancel = cancel
	// Allocate and hold memory
	s.chaos.memBalloon = make([]byte, amountMB*1024*1024)
	// Touch pages to ensure allocation
	for i := 0; i < len(s.chaos.memBalloon); i += 4096 {
		s.chaos.memBalloon[i] = 1
	}
	s.chaos.mu.Unlock()

	// Release after duration
	go func() {
		<-ctx.Done()
		s.chaos.mu.Lock()
		s.chaos.memBalloon = nil
		s.chaos.memActive = false
		s.chaos.memCancel = nil
		s.chaos.mu.Unlock()
		runtime.GC()
		s.logger.Info("chaos: Memory pressure ended")
	}()
}

func (s *Server) handleChaosErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	durationStr := r.URL.Query().Get("duration")
	if durationStr == "" {
		durationStr = "30s"
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid duration: %v", err), http.StatusBadRequest)
		return
	}

	rateStr := r.URL.Query().Get("rate")
	rate := 50 // percentage
	if rateStr != "" {
		rate, err = strconv.Atoi(rateStr)
		if err != nil || rate < 1 || rate > 100 {
			http.Error(w, "rate must be 1-100", http.StatusBadRequest)
			return
		}
	}

	s.startErrorChaos(duration, rate)

	s.logger.Warn("chaos: Error injection started",
		"duration", duration,
		"rate", rate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "error_injection_started",
		"duration": duration.String(),
		"rate":     rate,
		"message":  fmt.Sprintf("%d%% of /health requests will return 500 for %s", rate, duration),
	})
}

func (s *Server) startErrorChaos(duration time.Duration, rate int) {
	s.chaos.mu.Lock()
	// Stop existing error chaos if any
	if s.chaos.errorCancel != nil {
		s.chaos.errorCancel()
	}
	s.chaos.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), duration)

	s.chaos.mu.Lock()
	s.chaos.errorsActive = true
	s.chaos.errorRate = rate
	s.chaos.errorCancel = cancel
	s.chaos.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.chaos.mu.Lock()
		s.chaos.errorsActive = false
		s.chaos.errorRate = 0
		s.chaos.errorCancel = nil
		s.chaos.mu.Unlock()
		s.logger.Info("chaos: Error injection ended")
	}()
}

func (s *Server) handleChaosLatency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	durationStr := r.URL.Query().Get("duration")
	if durationStr == "" {
		durationStr = "30s"
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid duration: %v", err), http.StatusBadRequest)
		return
	}

	latencyStr := r.URL.Query().Get("latency")
	latency := 500 // ms
	if latencyStr != "" {
		latency, err = strconv.Atoi(latencyStr)
		if err != nil || latency < 1 || latency > 30000 {
			http.Error(w, "latency must be 1-30000 ms", http.StatusBadRequest)
			return
		}
	}

	s.startLatencyChaos(duration, latency)

	s.logger.Warn("chaos: Latency injection started",
		"duration", duration,
		"latency_ms", latency)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "latency_injection_started",
		"duration":   duration.String(),
		"latency_ms": latency,
		"message":    fmt.Sprintf("Adding %dms latency to all requests for %s", latency, duration),
	})
}

func (s *Server) startLatencyChaos(duration time.Duration, latencyMs int) {
	s.chaos.mu.Lock()
	// Stop existing latency chaos if any
	if s.chaos.latencyCancel != nil {
		s.chaos.latencyCancel()
	}
	s.chaos.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), duration)

	s.chaos.mu.Lock()
	s.chaos.latencyActive = true
	s.chaos.latencyMs = latencyMs
	s.chaos.latencyCancel = cancel
	s.chaos.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.chaos.mu.Lock()
		s.chaos.latencyActive = false
		s.chaos.latencyMs = 0
		s.chaos.latencyCancel = nil
		s.chaos.mu.Unlock()
		s.logger.Info("chaos: Latency injection ended")
	}()
}

func (s *Server) handleChaosStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.chaos.mu.Lock()
	if s.chaos.cpuCancel != nil {
		s.chaos.cpuCancel()
	}
	if s.chaos.memCancel != nil {
		s.chaos.memCancel()
	}
	if s.chaos.errorCancel != nil {
		s.chaos.errorCancel()
	}
	if s.chaos.latencyCancel != nil {
		s.chaos.latencyCancel()
	}
	s.chaos.mu.Unlock()

	// Wait a moment for cleanup
	time.Sleep(100 * time.Millisecond)

	s.logger.Info("chaos: All chaos stopped")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "all_chaos_stopped",
		"message": "All chaos conditions have been cleared",
	})
}

func (s *Server) handleChaosStatus(w http.ResponseWriter, r *http.Request) {
	s.chaos.mu.RLock()
	status := map[string]any{
		"cpu": map[string]any{
			"active": s.chaos.cpuActive,
		},
		"memory": map[string]any{
			"active":    s.chaos.memActive,
			"allocated": len(s.chaos.memBalloon) / (1024 * 1024),
		},
		"errors": map[string]any{
			"active": s.chaos.errorsActive,
			"rate":   s.chaos.errorRate,
		},
		"latency": map[string]any{
			"active":     s.chaos.latencyActive,
			"latency_ms": s.chaos.latencyMs,
		},
	}
	s.chaos.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// readCPUStats reads CPU stats from /proc/stat.
func readCPUStats() (cpuPercent float64, total, idle uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, 0
	}

	// Parse first line: cpu user nice system idle iowait irq softirq steal
	var user, nice, system, idleVal, iowait, irq, softirq, steal uint64
	_, err = fmt.Sscanf(string(data), "cpu %d %d %d %d %d %d %d %d",
		&user, &nice, &system, &idleVal, &iowait, &irq, &softirq, &steal)
	if err != nil {
		return 0, 0, 0
	}

	total = user + nice + system + idleVal + iowait + irq + softirq + steal
	idle = idleVal + iowait

	return 0, total, idle
}

// readMemoryPercent reads memory usage from /proc/meminfo.
func readMemoryPercent() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return -1
	}

	var memTotal, memAvailable uint64
	lines := string(data)
	for _, line := range splitLines(lines) {
		if len(line) == 0 {
			continue
		}
		var val uint64
		if n, _ := fmt.Sscanf(line, "MemTotal: %d kB", &val); n == 1 {
			memTotal = val
		}
		if n, _ := fmt.Sscanf(line, "MemAvailable: %d kB", &val); n == 1 {
			memAvailable = val
		}
	}

	if memTotal == 0 {
		return -1
	}

	used := memTotal - memAvailable
	return float64(used) / float64(memTotal) * 100
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
