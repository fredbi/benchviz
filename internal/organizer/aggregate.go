package organizer

import (
	"math"
	"slices"

	"github.com/fredbi/benchviz/internal/config"
)

// aggregate applies an aggregation formula to a set of measurements.
//
// It reports false when there is nothing left to aggregate: the derived measurement is
// then left missing, and shows as a gap rather than as a misleading zero.
func aggregate(formula config.AggregationFunction, values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}

	switch formula {
	case config.AggregationFunctionMean:
		return mean(values), true

	case config.AggregationFunctionGeoMean:
		// the geometric mean is only defined over positive values
		positive := positiveOnly(values)
		if len(positive) == 0 {
			return 0, false
		}

		return geoMean(positive), true

	case config.AggregationFunctionMin:
		return slices.Min(values), true

	case config.AggregationFunctionMax:
		return slices.Max(values), true

	case config.AggregationFunctionNone:
		return 0, false

	default:
		return 0, false
	}
}

// mean computes the arithmetic mean of the measurements.
func mean(values []float64) float64 {
	var total float64

	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

// geoMean computes the geometric mean of the measurements.
//
// It works in the log domain: benchmark timings span several orders of magnitude and a
// plain product of the measurements would overflow long before the root is taken.
func geoMean(values []float64) float64 {
	var sum float64

	for _, value := range values {
		sum += math.Log(value)
	}

	return math.Exp(sum / float64(len(values)))
}

// positiveOnly keeps the strictly positive measurements.
func positiveOnly(values []float64) []float64 {
	positive := make([]float64, 0, len(values))

	for _, value := range values {
		if value > 0 {
			positive = append(positive, value)
		}
	}

	return positive
}
