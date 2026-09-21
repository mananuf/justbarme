package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolStatter is the one method DBPoolCollector needs from *pgxpool.Pool --
// narrowed to make the collector trivially testable with a fake.
type poolStatter interface {
	Stat() *pgxpool.Stat
}

// DBPoolCollector reports the connection pool's current occupancy at scrape
// time -- docs/ARCHITECTURE.md §18's "database pool usage". A live
// snapshot via pgxpool.Pool.Stat(), not accumulated counters, so it always
// reflects reality even if the process has been running a long time.
type DBPoolCollector struct {
	pool poolStatter

	acquiredConns *prometheus.Desc
	idleConns     *prometheus.Desc
	maxConns      *prometheus.Desc
	totalConns    *prometheus.Desc
}

func NewDBPoolCollector(pool poolStatter) *DBPoolCollector {
	return &DBPoolCollector{
		pool:          pool,
		acquiredConns: prometheus.NewDesc("jbm_db_pool_acquired_conns", "Connections currently acquired (in use).", nil, nil),
		idleConns:     prometheus.NewDesc("jbm_db_pool_idle_conns", "Connections currently idle.", nil, nil),
		maxConns:      prometheus.NewDesc("jbm_db_pool_max_conns", "Configured maximum pool size.", nil, nil),
		totalConns:    prometheus.NewDesc("jbm_db_pool_total_conns", "Total connections currently held by the pool (acquired + idle + constructing).", nil, nil),
	}
}

func (c *DBPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquiredConns
	ch <- c.idleConns
	ch <- c.maxConns
	ch <- c.totalConns
}

func (c *DBPoolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(stat.TotalConns()))
}
