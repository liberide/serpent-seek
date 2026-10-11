// Package metrics is a minimal, dependency-free Prometheus-compatible registry
// for the gateway's own counters and gauges.
//
// It deliberately does not pull in the Prometheus client library: the
// exposition format is small, the label cardinality is bounded (provider
// display names) and the project keeps its dependency/license surface tight.
package metrics

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/liberide/serpent-seek/internal/version"
)

// agg is one counter with a millisecond sum.
type agg struct {
	count int64
	sumMS int64
}

// Metrics collects process-wide search metrics. It is safe for concurrent use.
type Metrics struct {
	active atomic.Int64

	mu        sync.Mutex
	search    map[string]*agg            // final status -> counter
	providers map[string]map[string]*agg // provider display name -> status -> counter
}

// New builds an empty registry.
func New() *Metrics {
	return &Metrics{
		search:    map[string]*agg{},
		providers: map[string]map[string]*agg{},
	}
}

// RequestStarted increments the in-flight request gauge.
func (m *Metrics) RequestStarted() { m.active.Add(1) }

// RequestEnded decrements the in-flight request gauge.
func (m *Metrics) RequestEnded() { m.active.Add(-1) }

// ActiveRequests returns the number of in-flight requests.
func (m *Metrics) ActiveRequests() int64 { return m.active.Load() }

// RecordSearch records one completed search by final status (ok|empty|fail).
func (m *Metrics) RecordSearch(status string, ms int) {
	if status == "" {
		status = "fail"
	}
	m.record(m.search, status, ms)
}

// RecordProvider records one provider attempt by outcome (ok|empty|fail|skip).
func (m *Metrics) RecordProvider(provider, status string, ms int) {
	if provider == "" {
		provider = "unknown"
	}
	if status == "" {
		status = "fail"
	}
	m.mu.Lock()
	byStatus := m.providers[provider]
	if byStatus == nil {
		byStatus = map[string]*agg{}
		m.providers[provider] = byStatus
	}
	m.mu.Unlock()
	m.record(byStatus, status, ms)
}

func (m *Metrics) record(byKey map[string]*agg, key string, ms int) {
	m.mu.Lock()
	a := byKey[key]
	if a == nil {
		a = &agg{}
		byKey[key] = a
	}
	a.count++
	a.sumMS += int64(ms)
	m.mu.Unlock()
}

// searchStatuses is the fixed status set rendered for searches so dashboards do
// not have to cope with missing series.
var searchStatuses = []string{"ok", "empty", "fail"}

// Render writes the metrics in the Prometheus text exposition format
// (version 0.0.4). sseClients is the current SSE subscriber count.
func (m *Metrics) Render(w io.Writer, sseClients int) error {
	search, providers := m.snapshot()

	bw := bufio.NewWriter(w)

	header(bw, "serpentseek_build_info", "Build information (always 1).", "gauge")
	fmt.Fprintf(bw, "serpentseek_build_info{%s} 1\n", label("version", version.Version))

	header(bw, "serpentseek_active_requests", "In-flight search requests.", "gauge")
	fmt.Fprintf(bw, "serpentseek_active_requests %d\n", m.ActiveRequests())

	header(bw, "serpentseek_sse_clients", "Connected SSE subscribers.", "gauge")
	fmt.Fprintf(bw, "serpentseek_sse_clients %d\n", sseClients)

	header(bw, "serpentseek_search_requests_total", "Completed search requests by final status.", "counter")
	var searchCount, searchSum int64
	for _, status := range searchStatuses {
		a := search[status]
		fmt.Fprintf(bw, "serpentseek_search_requests_total{%s} %d\n", label("status", status), a.count)
		searchCount += a.count
		searchSum += a.sumMS
	}

	header(bw, "serpentseek_search_duration_seconds", "Search execution time.", "summary")
	fmt.Fprintf(bw, "serpentseek_search_duration_seconds_sum %s\n", seconds(searchSum))
	fmt.Fprintf(bw, "serpentseek_search_duration_seconds_count %d\n", searchCount)

	header(bw, "serpentseek_provider_requests_total", "Provider attempts by outcome.", "counter")
	providerNames := sortedKeys(providers)
	for _, name := range providerNames {
		for _, status := range sortedKeys(providers[name]) {
			fmt.Fprintf(bw, "serpentseek_provider_requests_total{%s,%s} %d\n",
				label("provider", name), label("status", status), providers[name][status].count)
		}
	}

	header(bw, "serpentseek_provider_duration_seconds", "Provider attempt execution time.", "summary")
	for _, name := range providerNames {
		var sum, count int64
		for _, a := range providers[name] {
			sum += a.sumMS
			count += a.count
		}
		fmt.Fprintf(bw, "serpentseek_provider_duration_seconds_sum{%s} %s\n", label("provider", name), seconds(sum))
		fmt.Fprintf(bw, "serpentseek_provider_duration_seconds_count{%s} %d\n", label("provider", name), count)
	}

	return bw.Flush()
}

// snapshot copies the counters under the lock so rendering never blocks writers.
func (m *Metrics) snapshot() (map[string]agg, map[string]map[string]agg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	search := make(map[string]agg, len(m.search))
	for k, v := range m.search {
		search[k] = *v
	}
	providers := make(map[string]map[string]agg, len(m.providers))
	for name, byStatus := range m.providers {
		cp := make(map[string]agg, len(byStatus))
		for status, v := range byStatus {
			cp[status] = *v
		}
		providers[name] = cp
	}
	return search, providers
}

func header(w io.Writer, name, help, typ string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func label(name, value string) string {
	return name + `="` + escapeLabel(value) + `"`
}

// escapeLabel escapes a Prometheus label value.
func escapeLabel(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return r.Replace(s)
}

func seconds(ms int64) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', 6, 64)
}

func sortedKeys[M ~map[string]V, V any](m M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
