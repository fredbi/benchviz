package config

type MetricName string

const (
	MetricNsPerOp     MetricName = "nsPerOp"
	MetricAllocsPerOp MetricName = "allocsPerOp"
	MetricBytesPerOp  MetricName = "bytesPerOp"
	MetricMBPerS      MetricName = "MBytesPerS"
)

func (m MetricName) String() string {
	return string(m)
}

func (m MetricName) IsValid() bool {
	switch m {
	case MetricNsPerOp, MetricAllocsPerOp, MetricBytesPerOp, MetricMBPerS:
		return true
	default:
		return false
	}
}

func AllMetricNames() []MetricName {
	return []MetricName{
		MetricNsPerOp,
		MetricAllocsPerOp,
		MetricBytesPerOp,
		MetricMBPerS,
	}
}
