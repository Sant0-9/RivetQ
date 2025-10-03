package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// JobsEnqueuedTotal counts total jobs enqueued
	JobsEnqueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rivetq_jobs_enqueued_total",
			Help: "Total number of jobs enqueued",
		},
		[]string{"queue"},
	)

	// JobsLeasedTotal counts total jobs leased
	JobsLeasedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rivetq_jobs_leased_total",
			Help: "Total number of jobs leased",
		},
		[]string{"queue"},
	)

	// JobsAckedTotal counts total jobs acknowledged
	JobsAckedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rivetq_jobs_acked_total",
			Help: "Total number of jobs acknowledged",
		},
		[]string{"queue"},
	)

	// JobsNackedTotal counts total jobs negatively acknowledged
	JobsNackedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rivetq_jobs_nacked_total",
			Help: "Total number of jobs negatively acknowledged",
		},
		[]string{"queue"},
	)

	// JobsReady gauge for ready jobs
	JobsReady = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rivetq_jobs_ready",
			Help: "Number of jobs ready to be leased",
		},
		[]string{"queue"},
	)

	// JobsInflight gauge for inflight jobs
	JobsInflight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rivetq_jobs_inflight",
			Help: "Number of jobs currently leased",
		},
		[]string{"queue"},
	)

	// JobsDLQ gauge for dead letter queue
	JobsDLQ = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rivetq_jobs_dlq",
			Help: "Number of jobs in dead letter queue",
		},
		[]string{"queue"},
	)

	// WALSegments gauge for WAL segment count
	WALSegments = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rivetq_wal_segments",
			Help: "Number of WAL segments",
		},
	)

	// WALSize gauge for total WAL size
	WALSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rivetq_wal_size_bytes",
			Help: "Total size of WAL in bytes",
		},
	)

	// RateLimitRejections counts rate limit rejections
	RateLimitRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rivetq_rate_limit_rejections_total",
			Help: "Total number of jobs rejected due to rate limiting",
		},
		[]string{"queue"},
	)

	// Cluster metrics
	ClusterNodes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rivetq_cluster_nodes",
			Help: "Number of nodes in the cluster",
		},
	)

	ClusterLeader = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rivetq_cluster_is_leader",
			Help: "Whether this node is the Raft leader (1) or not (0)",
		},
	)

	RaftCommittedIndex = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rivetq_raft_committed_index",
			Help: "Raft committed log index",
		},
	)

	RaftAppliedIndex = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rivetq_raft_applied_index",
			Help: "Raft applied log index",
		},
	)

	ProxyForwardedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rivetq_proxy_forwarded_total",
			Help: "Total number of requests forwarded to other nodes",
		},
		[]string{"target_node"},
	)

	ProxyForwardErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rivetq_proxy_forward_errors_total",
			Help: "Total number of failed request forwards",
		},
	)
)
