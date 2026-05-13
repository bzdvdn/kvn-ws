// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task production-hardening#T3.2: prometheus metrics (AC-007)
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// @sk-task production-hardening#T3.2: metrics collectors (AC-007)
type Collectors struct {
	ActiveSessions prometheus.Gauge
	Throughput     *prometheus.CounterVec
	Errors         *prometheus.CounterVec
}

func NewCollectors() *Collectors {
	return &Collectors{
		ActiveSessions: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kvn_active_sessions",
			Help: "Current number of active VPN sessions",
		}),
		Throughput: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "kvn_throughput_bytes_total",
			Help: "Total bytes transferred through tunnel",
		}, []string{"type"}),
		Errors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "kvn_errors_total",
			Help: "Total errors by type",
		}, []string{"type"}),
	}
}

// @sk-task production-hardening#T3.2: inc errors by type (AC-007)
func (c *Collectors) IncError(errType string) {
	c.Errors.WithLabelValues(errType).Inc()
}

// @sk-task production-hardening#T3.2: add bytes by direction (AC-007)
func (c *Collectors) AddThroughput(dir string, bytes float64) {
	c.Throughput.WithLabelValues(dir).Add(bytes)
}
