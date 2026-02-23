package config

// MetricName identifies a benchmark metric (e.g. "nsPerOp", "allocsPerOp").
type MetricName string

// Standard benchmark metric names.
const (
	MetricNsPerOp     MetricName = "nsPerOp"
	MetricAllocsPerOp MetricName = "allocsPerOp"
	MetricBytesPerOp  MetricName = "bytesPerOp"
	MetricMBPerS      MetricName = "MBytesPerS"
)

// String returns the metric name as a plain string.
func (m MetricName) String() string {
	return string(m)
}

// IsValid reports whether the metric name is one of the known benchmark metrics.
func (m MetricName) IsValid() bool {
	switch m {
	case MetricNsPerOp, MetricAllocsPerOp, MetricBytesPerOp, MetricMBPerS:
		return true
	default:
		return false
	}
}

// AllMetricNames returns all known benchmark metric names.
func AllMetricNames() []MetricName {
	return []MetricName{
		MetricNsPerOp,
		MetricAllocsPerOp,
		MetricBytesPerOp,
		MetricMBPerS,
	}
}
