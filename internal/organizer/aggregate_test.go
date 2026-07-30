package organizer

import (
	"testing"

	"github.com/fredbi/benchviz/internal/config"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestAggregate(t *testing.T) {
	tests := []struct {
		name    string
		formula config.AggregationFunction
		values  []float64
		want    float64
		wantOK  bool
	}{
		{
			name:    "mean",
			formula: config.AggregationFunctionMean,
			values:  []float64{10, 20, 60},
			want:    30,
			wantOK:  true,
		},
		{
			name:    "geomean",
			formula: config.AggregationFunctionGeoMean,
			values:  []float64{1, 10, 100},
			want:    10,
			wantOK:  true,
		},
		{
			name:    "geomean over a wide range stays stable",
			formula: config.AggregationFunctionGeoMean,
			values:  []float64{1e-3, 1e300},
			want:    3.1622776601683795e148, // sqrt(1e297): a plain product would overflow
			wantOK:  true,
		},
		{
			name:    "min",
			formula: config.AggregationFunctionMin,
			values:  []float64{10, 20, 60},
			want:    10,
			wantOK:  true,
		},
		{
			name:    "max",
			formula: config.AggregationFunctionMax,
			values:  []float64{10, 20, 60},
			want:    60,
			wantOK:  true,
		},
		{
			name:    "single value",
			formula: config.AggregationFunctionGeoMean,
			values:  []float64{42},
			want:    42,
			wantOK:  true,
		},
		{
			name:    "no value at all",
			formula: config.AggregationFunctionMean,
			values:  nil,
			wantOK:  false,
		},
		{
			name:    "geomean of non positive values is undefined",
			formula: config.AggregationFunctionGeoMean,
			values:  []float64{-1, -2},
			wantOK:  false,
		},
		{
			name:    "geomean skips non positive values",
			formula: config.AggregationFunctionGeoMean,
			values:  []float64{-1, 4, 9},
			want:    6,
			wantOK:  true,
		},
		{
			name:    "no formula",
			formula: config.AggregationFunctionNone,
			values:  []float64{10, 20},
			wantOK:  false,
		},
		{
			name:    "unknown formula",
			formula: config.AggregationFunction("median"),
			values:  []float64{10, 20},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := aggregate(tt.formula, tt.values)
			require.Equal(t, tt.wantOK, ok)

			if !tt.wantOK {
				assert.Zero(t, got)

				return
			}

			assert.InEpsilon(t, tt.want, got, 1e-9)
		})
	}
}
