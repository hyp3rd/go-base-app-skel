package pg

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyp3rd/base/internal/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// MaxMetricsToStore is the maximum number of metrics to store in the database.
	MaxMetricsToStore = 10000 // Keep last 10k metrics
	// HealthStatusMaxErrors is the maximum number of errors to keep in the health status.
	HealthStatusMaxErrors = 100
	// MonitorInterval is the interval at which the monitor will check the health of the database.
	MonitorInterval = 10 * time.Second
)

// PoolStats represents statistics about the connection pool.
// It includes information about active queries, queued queries, slow queries,
// failed queries, average query time, the number of prepared statements,
// and the last error that occurred.
type PoolStats struct {
	*pgxpool.Stat
	// Connection metrics
	ActiveQueries int64 // Currently executing queries
	QueuedQueries int64 // Queries waiting for execution
	SlowQueries   int64 // Queries exceeding threshold
	FailedQueries int64 // Queries that resulted in errors

	// Connection timing
	AcquireCount       int64         // Total number of connection acquisitions
	AcquireDuration    time.Duration // Average time to acquire a connection
	WaitingConnections int64         // Number of goroutines waiting for a connection
	IdleConnections    int64         // Current number of idle connections

	// Connection lifecycle
	MaxLifetimeDropped int64 // Connections dropped due to max lifetime
	MaxIdleTimeDropped int64 // Connections dropped due to idle timeout
	ConnectionRefusals int64 // Connection requests that were refused
	// PendingConnections represents connections that exist in the pool
	// but are neither idle nor acquired. These may be connections
	// in the process of being established or closed.
	PendingConnections int64
	// Performance metrics
	AverageQueryTime  time.Duration // Average query execution time
	PreparedStmtCount int           // Number of prepared statements

	// Error tracking
	LastError     error     // Last error that occurred
	LastErrorTime time.Time // When the last error occurred
	ErrorCount    int64     // Total number of errors
}

// PreparedStatement represents a prepared SQL statement in the database.
// It includes information about the statement, such as the query text,
// a unique statement ID, when the statement was created, when it was
// last used, how many times it has been used, the average execution
// time, and the total execution time. The struct is protected by a
// read-write mutex to allow concurrent access.
type PreparedStatement struct {
	Query           string
	StatementID     string
	CreatedAt       time.Time
	LastUsed        time.Time
	UsageCount      int64
	AverageExecTime time.Duration
	TotalExecTime   time.Duration
	mu              sync.RWMutex
}

// HealthStatus represents the health status of a database connection.
// It includes information about the connection status, connection pool statistics,
// latency, last checked time, replication lag (for replicas), and recent errors.
// The MaxErrors field specifies the maximum number of errors to keep in the Errors slice.
type HealthStatus struct {
	Connected      bool
	PoolStats      *PoolStats
	Latency        time.Duration
	LastChecked    time.Time
	ReplicationLag *time.Duration // Only for replicas
	Errors         []error        // Recent errors
	MaxErrors      int            // Maximum number of errors to keep
}

// Monitor is a struct that manages the monitoring of a database connection pool.
// It includes information about the health status of the connection, prepared statements,
// slow query threshold, and metrics collected from the database.
type Monitor struct {
	manager            *Manager
	healthStatus       *HealthStatus
	preparedStmts      map[string]*PreparedStatement
	slowQueryThreshold time.Duration
	mu                 sync.RWMutex
	stopChan           chan struct{}
	metrics            []QueryMetric
	maxMetrics         int
}

// QueryMetric represents a metric collected for a database query, including the
// query text, the duration of the query, the number of rows affected, the
// timestamp when the query was executed, and any error that occurred during
// the query execution.
type QueryMetric struct {
	Query        string
	Duration     time.Duration
	RowsAffected int64
	Timestamp    time.Time
	Error        error
}

// NewMonitor creates a new Monitor instance with the given slow query threshold.
// The Monitor is responsible for managing the monitoring of a database connection pool,
// including collecting health status, prepared statements, and query metrics.
func (m *Manager) NewMonitor(slowQueryThreshold time.Duration) *Monitor {
	return &Monitor{
		manager: m,
		healthStatus: &HealthStatus{
			MaxErrors: HealthStatusMaxErrors,
			PoolStats: &PoolStats{}, // Initialize PoolStats
		},
		preparedStmts:      make(map[string]*PreparedStatement),
		slowQueryThreshold: slowQueryThreshold,
		stopChan:           make(chan struct{}),
		maxMetrics:         MaxMetricsToStore,
	}
}

// Start runs a background goroutine that periodically collects metrics for the
// database connection pool managed by the Monitor. It uses a ticker to trigger
// the collection of metrics at a fixed interval, and stops the ticker when the
// stopChan is closed or the context is canceled.
func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(MonitorInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				m.collectMetrics(ctx)
			case <-m.stopChan:
				ticker.Stop()

				return
			case <-ctx.Done():
				ticker.Stop()

				return
			}
		}
	}()
}

// Stop stops the background goroutine that periodically collects metrics for the database connection pool.
func (m *Monitor) Stop() {
	close(m.stopChan)
}

// collectMetrics gathers current pool statistics and health information. It collects
// the pool statistics using collectPoolStats, updates the health status by pinging
// the database, logs the pool statistics, and cleans up old prepared statements.
// This method is called periodically by the Start method to collect and maintain
// the monitoring data for the database connection pool.
func (m *Monitor) collectMetrics(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Collect pool statistics
	stats := m.collectPoolStats()
	if stats == nil {
		return
	}

	// Update health status
	start := time.Now()
	err := m.manager.Ping(ctx)
	latency := time.Since(start)

	m.healthStatus.Connected = err == nil
	m.healthStatus.Latency = latency
	m.healthStatus.LastChecked = time.Now()
	m.healthStatus.PoolStats = stats

	if err != nil {
		stats.LastError = err
		stats.LastErrorTime = time.Now()
		atomic.AddInt64(&stats.ErrorCount, 1)
		m.addError(err)
	}

	// Log the statistics
	m.logPoolStats()

	// Clean up old prepared statements
	m.cleanupPreparedStatements()
}

// updatePoolStats atomically updates the pool statistics. It takes a *PoolStats
// argument and updates the various statistics fields using atomic operations.
// This ensures the statistics are updated in a thread-safe manner.
func (m *Monitor) updatePoolStats(stats *PoolStats) {
	if stats == nil {
		return
	}

	// Atomic updates for all counters
	atomic.StoreInt64(&stats.ActiveQueries, int64(stats.Stat.AcquiredConns()))
	atomic.StoreInt64(&stats.IdleConnections, int64(stats.Stat.IdleConns()))
	atomic.StoreInt64(&stats.PendingConnections, int64(stats.Stat.TotalConns()-stats.Stat.IdleConns()-stats.Stat.AcquiredConns()))

	// Update acquisition metrics
	atomic.StoreInt64(&stats.AcquireCount, stats.Stat.AcquireCount())

	// Calculate average acquire duration if we have acquisitions
	if stats.AcquireCount > 0 {
		avgDuration := stats.Stat.AcquireDuration().Nanoseconds() / stats.AcquireCount
		atomic.StoreInt64((*int64)(&stats.AcquireDuration), avgDuration)
	}
}

// collectPoolStats gathers comprehensive pool statistics for the database connection pool.
// It retrieves the current pool statistics from the manager, copies relevant values from the
// existing health status, and then updates the pool statistics using updatePoolStats.
// The resulting PoolStats struct is returned, or nil if the manager's Stats() method returns nil.
func (m *Monitor) collectPoolStats() *PoolStats {
	poolStat := m.manager.Stats()
	if poolStat == nil {
		return nil
	}

	stats := &PoolStats{
		Stat: poolStat,
		// Copy existing atomic values
		ActiveQueries: atomic.LoadInt64(&m.healthStatus.PoolStats.ActiveQueries),
		SlowQueries:   atomic.LoadInt64(&m.healthStatus.PoolStats.SlowQueries),
		FailedQueries: atomic.LoadInt64(&m.healthStatus.PoolStats.FailedQueries),
		ErrorCount:    atomic.LoadInt64(&m.healthStatus.PoolStats.ErrorCount),

		// Copy non-atomic values under lock
		LastError:         m.healthStatus.PoolStats.LastError,
		LastErrorTime:     m.healthStatus.PoolStats.LastErrorTime,
		PreparedStmtCount: len(m.preparedStmts),
	}

	// Update the statistics
	m.updatePoolStats(stats)

	return stats
}

// logPoolStats outputs detailed pool statistics. It collects comprehensive pool statistics using collectPoolStats,
// and then logs the statistics using the logger. It also logs warnings for concerning metrics, such as waiting
// connections and connection refusals.
func (m *Monitor) logPoolStats() {
	stats := m.collectPoolStats()
	if stats == nil {
		return
	}

	// Create fields correctly
	m.manager.logger.WithFields(
		logger.Field{Key: "active_queries", Value: stats.ActiveQueries},
		logger.Field{Key: "idle_connections", Value: stats.IdleConnections},
		logger.Field{Key: "waiting_connections", Value: stats.WaitingConnections},
		logger.Field{Key: "acquire_count", Value: stats.AcquireCount},
		logger.Field{Key: "acquire_duration_ms", Value: stats.AcquireDuration.Milliseconds()},
		logger.Field{Key: "slow_queries", Value: stats.SlowQueries},
		logger.Field{Key: "failed_queries", Value: stats.FailedQueries},
		logger.Field{Key: "prepared_statements", Value: stats.PreparedStmtCount},
		logger.Field{Key: "error_count", Value: stats.ErrorCount},
	).Info("Pool Statistics")

	// Log warnings for concerning metrics
	if stats.WaitingConnections > 0 {
		m.manager.logger.WithFields(
			logger.Field{Key: "waiting_count", Value: stats.WaitingConnections},
		).Warn("Connections waiting in pool")
	}

	if stats.ConnectionRefusals > 0 {
		m.manager.logger.WithFields(
			logger.Field{Key: "refusal_count", Value: stats.ConnectionRefusals},
		).Warn("Connection refusals detected")
	}
}

// TrackQuery records query execution metrics. It logs the query, duration, rows affected, and any errors that occurred during the query execution. It also tracks slow queries and failed queries in the health status.
func (m *Monitor) TrackQuery(query string, duration time.Duration, rowsAffected int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metric := QueryMetric{
		Query:        query,
		Duration:     duration,
		RowsAffected: rowsAffected,
		Timestamp:    time.Now(),
		Error:        err,
	}

	// Update metrics
	m.metrics = append(m.metrics, metric)
	if len(m.metrics) > m.maxMetrics {
		m.metrics = m.metrics[1:]
	}

	// Track slow queries
	if duration > m.slowQueryThreshold {
		atomic.AddInt64(&m.healthStatus.PoolStats.SlowQueries, 1)
	}

	if err != nil {
		atomic.AddInt64(&m.healthStatus.PoolStats.FailedQueries, 1)
	}
}

// TrackPreparedStatement records metrics for a prepared SQL statement, including the usage count, last used time, total execution time, and average execution time.
// This function is used to track the usage and performance of prepared statements in the database connection pool.
func (m *Monitor) TrackPreparedStatement(query string, stmtID string, execTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stmt, exists := m.preparedStmts[query]
	if !exists {
		stmt = &PreparedStatement{
			Query:       query,
			StatementID: stmtID,
			CreatedAt:   time.Now(),
		}
		m.preparedStmts[query] = stmt
	}

	stmt.mu.Lock()
	stmt.UsageCount++
	stmt.LastUsed = time.Now()
	stmt.TotalExecTime += execTime
	stmt.AverageExecTime = stmt.TotalExecTime / time.Duration(stmt.UsageCount)
	stmt.mu.Unlock()
}

// cleanupPreparedStatements removes unused prepared statements.
func (m *Monitor) cleanupPreparedStatements() {
	threshold := time.Now().Add(-1 * time.Hour)

	for query, stmt := range m.preparedStmts {
		stmt.mu.RLock()
		if stmt.LastUsed.Before(threshold) && stmt.UsageCount < 100 {
			delete(m.preparedStmts, query)
		}
		stmt.mu.RUnlock()
	}
}

// addError adds an error to the health status.
func (m *Monitor) addError(err error) {
	m.healthStatus.Errors = append(m.healthStatus.Errors, err)
	if len(m.healthStatus.Errors) > m.healthStatus.MaxErrors {
		m.healthStatus.Errors = m.healthStatus.Errors[1:]
	}
}

// GetHealthStatus returns a copy of the current health status of the database connection pool.
// The returned HealthStatus object is a snapshot of the current state and is safe to access
// without race conditions.
func (m *Monitor) GetHealthStatus() *HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent races
	status := *m.healthStatus

	return &status
}

// GetPoolMetrics returns a copy of the current query metrics for the database connection pool.
// The returned slice of QueryMetric objects is a snapshot of the current state and is safe to access
// without race conditions.
func (m *Monitor) GetPoolMetrics() []QueryMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy of metrics
	metrics := make([]QueryMetric, len(m.metrics))
	copy(metrics, m.metrics)

	return metrics
}
