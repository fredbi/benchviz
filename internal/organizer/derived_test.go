package organizer

import (
	"fmt"
	"testing"

	"github.com/fredbi/benchviz/internal/config"
	"github.com/fredbi/benchviz/internal/model"
	"github.com/fredbi/benchviz/internal/parser"
	"golang.org/x/tools/benchmark/parse"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestDerivedContextSeries checks the summary bar produced by a derived context: it
// aggregates the contexts of its own function, within its own version, so that the
// versions stay comparable on the extra tick.
func TestDerivedContextSeries(t *testing.T) {
	tests := []struct {
		formula string
		// the derived context carries no title: it is named after its formula
		wantLabel   string
		wantReflect float64
		wantGeneric float64
	}{
		// reflect measures 1 and 100, generics measures 4 and 9
		{formula: "mean", wantLabel: "Mean", wantReflect: 50.5, wantGeneric: 6.5},
		{formula: "geomean", wantLabel: "Geomean", wantReflect: 10, wantGeneric: 6},
		{formula: "min", wantLabel: "Min", wantReflect: 1, wantGeneric: 4},
		{formula: "max", wantLabel: "Max", wantReflect: 100, wantGeneric: 9},
	}

	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			cfg := mustLoadConfig(t, derivedContextConfig(tt.formula))
			o := New(cfg)

			scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
			require.NoError(t, err)
			require.Len(t, scenario.Categories, 1)

			category := scenario.Categories[0]

			// the summary sits last, where it is declared
			require.Equal(t, []string{"Int", "Float64", tt.wantLabel}, category.XLabels)

			assert.InEpsilon(t, tt.wantReflect, derivedPoint(t, category, "reflect"), 1e-9)
			assert.InEpsilon(t, tt.wantGeneric, derivedPoint(t, category, "generics"), 1e-9)
		})
	}
}

// TestDerivedContextPosition verifies that the declaration order of the contexts decides
// where the summary bar shows up, even though it is computed last.
func TestDerivedContextPosition(t *testing.T) {
	cfg := mustLoadConfig(t, derivedContextConfigWithOrder("geomean", "[summary, int, float64]"))
	o := New(cfg)

	scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
	require.NoError(t, err)
	require.Len(t, scenario.Categories, 1)

	category := scenario.Categories[0]
	assert.Equal(t, []string{"Geomean", "Int", "Float64"}, category.XLabels)

	// still the geomean of the two measured contexts, wherever it sits
	series := seriesOf(t, category, "reflect")
	assert.InEpsilon(t, 10.0, series.Points[0].Value, 1e-9)
}

// TestDerivedContextIgnoresOtherDerived verifies that an aggregate never feeds another
// aggregate: the geomean must not pick up the min sitting next to it.
func TestDerivedContextIgnoresOtherDerived(t *testing.T) {
	cfg := mustLoadConfig(t, `
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    match: 'Greater'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
  - id: lowest
    title: Lowest
    derivedContext:
      formula: min
  - id: summary
    title: Summary
    derivedContext:
      formula: geomean
versions:
  - id: reflect
    match: '/reflect/'
categories:
  - id: comparisons
    includes:
      functions: [greater]
      versions: [reflect]
      contexts: [int, float64, lowest, summary]
      metrics: [nsPerOp]
`)
	o := New(cfg)

	scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
	require.NoError(t, err)

	category := scenario.Categories[0]
	require.Equal(t, []string{"Int", "Float64", "Lowest", "Summary"}, category.XLabels)

	series := seriesOf(t, category, "reflect")
	assert.InEpsilon(t, 1.0, series.Points[2].Value, 1e-9, "min of 1 and 100")
	// geomean of 1 and 100 is 10; had it swallowed the min column it would be ~4.64
	assert.InEpsilon(t, 10.0, series.Points[3].Value, 1e-9, "geomean of 1 and 100, not of 1, 100 and 1")
}

// TestDerivedContextDropsZeroAndMissing verifies the drop rules: a zero measurement
// carries no information for a summary, and a hole must not count as a zero.
func TestDerivedContextDropsZeroAndMissing(t *testing.T) {
	cfg := mustLoadConfig(t, derivedContextConfig("mean"))
	o := New(cfg)

	// reflect/int measures 0 allocs, reflect/float64 measures 8: the mean is 8, not 4.
	// generics has no float64 benchmark at all: its mean rests on int only.
	sets := []parser.Set{{
		Set: parse.Set{
			"BenchmarkGreater/reflect/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/int-16", N: 1, NsPerOp: 1, AllocsPerOp: 0},
			},
			"BenchmarkGreater/reflect/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/float64-16", N: 1, NsPerOp: 100, AllocsPerOp: 8},
			},
			"BenchmarkGreater/generic/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/int-16", N: 1, NsPerOp: 4, AllocsPerOp: 3},
			},
		},
		File: "test.json",
	}}

	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)
	category := scenario.Categories[0]

	allocs := seriesOfMetric(t, category, "reflect", "allocsPerOp")
	assert.Zero(t, allocs.Points[0].Value, "the zero measurement is kept on its own column")
	assert.False(t, allocs.Points[0].Missing)
	assert.InEpsilon(t, 8.0, allocs.Points[2].Value, 1e-9, "the zero is dropped from the aggregate")

	timings := seriesOfMetric(t, category, "generics", "nsPerOp")
	assert.True(t, timings.Points[1].Missing, "generics has no float64 measurement")
	assert.InEpsilon(t, 4.0, timings.Points[2].Value, 1e-9, "the hole is dropped from the aggregate")
}

// TestDerivedContextWithoutMeasurement verifies that a summary with nothing to aggregate
// stays missing, and is dropped along with its column when no version can fill it.
func TestDerivedContextWithoutMeasurement(t *testing.T) {
	cfg := mustLoadConfig(t, derivedContextConfig("mean"))
	o := New(cfg)

	scenario, err := o.Scenarize(nil)
	require.NoError(t, err)
	assert.Empty(t, scenario.Categories, "no measurement at all: nothing to chart")
}

// helpers

// derivedPoint returns the value of the derived column of a version, for the first metric.
func derivedPoint(t *testing.T, category model.Category, version string) float64 {
	t.Helper()

	series := seriesOf(t, category, version)
	point := series.Points[len(series.Points)-1]
	require.False(t, point.Missing, "expected a derived measurement for version %q", version)

	return point.Value
}

// seriesOf returns the series of a version, for the first metric of the category.
func seriesOf(t *testing.T, category model.Category, version string) model.MetricSeries {
	t.Helper()

	for _, data := range category.Data {
		if data.Version.ID != version {
			continue
		}

		require.Len(t, data.Series, 1)

		return data.Series[0]
	}

	require.FailNow(t, fmt.Sprintf("no series found for version %q", version))

	return model.MetricSeries{}
}

// seriesOfMetric returns the series of a (version, metric) pair.
func seriesOfMetric(t *testing.T, category model.Category, version, metric string) model.MetricSeries {
	t.Helper()

	for _, data := range category.Data {
		if data.Version.ID != version || data.Metric.ID.String() != metric {
			continue
		}

		require.Len(t, data.Series, 1)

		return data.Series[0]
	}

	require.FailNow(t, fmt.Sprintf("no series found for version %q, metric %q", version, metric))

	return model.MetricSeries{}
}

// buildDerivedSet measures 1 and 100 ns/op for reflect, 4 and 9 for generics.
func buildDerivedSet() parser.Set {
	return parser.Set{
		Set: parse.Set{
			"BenchmarkGreater/reflect/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/int-16", N: 1, NsPerOp: 1},
			},
			"BenchmarkGreater/reflect/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/float64-16", N: 1, NsPerOp: 100},
			},
			"BenchmarkGreater/generic/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/int-16", N: 1, NsPerOp: 4},
			},
			"BenchmarkGreater/generic/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/float64-16", N: 1, NsPerOp: 9},
			},
		},
		File: "test.json",
	}
}

func derivedContextConfig(formula string) string {
	return derivedContextConfigWithOrder(formula, "[int, float64, summary]")
}

func derivedContextConfigWithOrder(formula, contexts string) string {
	return fmt.Sprintf(`
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
  - id: allocsPerOp
    title: Allocations
    axis: 'allocs/op'
functions:
  - id: greater
    match: 'Greater'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
  - id: summary
    derivedContext:
      formula: %s
versions:
  - id: reflect
    match: '/reflect/'
  - id: generics
    match: '/generic/'
categories:
  - id: comparisons
    includes:
      functions: [greater]
      versions: [reflect, generics]
      contexts: %s
      metrics: [nsPerOp, allocsPerOp]
`, formula, contexts)
}

// TestDerivedCategory checks the "bottom line" chart: one tick per function, holding the
// aggregate over all the contexts of the implied scope, with the versions kept as series
// so that they stay comparable.
func TestDerivedCategory(t *testing.T) {
	cfg := mustLoadConfig(t, derivedCategoryConfig("geomean"))
	o := New(cfg)

	scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
	require.NoError(t, err)
	require.Len(t, scenario.Categories, 2, "the measured category, then the derived one")

	bottomLine := scenario.Categories[1]
	assert.Equal(t, "bottom-line", bottomLine.ID)
	assert.Equal(t, "{metric} - Geomean", bottomLine.Title)

	// one tick per function: the contexts it aggregates are gone
	require.Equal(t, []string{"Greater"}, bottomLine.XLabels)

	// versions are still the series
	reflect := seriesOf(t, bottomLine, "reflect")
	generics := seriesOf(t, bottomLine, "generics")
	require.Len(t, reflect.Points, 1)
	require.Len(t, generics.Points, 1)

	assert.InEpsilon(t, 10.0, reflect.Points[0].Value, 1e-9, "geomean of 1 and 100")
	assert.InEpsilon(t, 6.0, generics.Points[0].Value, 1e-9, "geomean of 4 and 9")

	// the point label is the function, on both the axis and the point
	assert.Equal(t, "Greater", reflect.Points[0].Label)
}

// TestDerivedCategoryScope verifies that the scope of a derived category is implied from
// the other categories: their functions, contexts, versions and metrics, deduplicated in
// declaration order, minus the derived contexts.
func TestDerivedCategoryScope(t *testing.T) {
	cfg := mustLoadConfig(t, `
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
  - id: allocsPerOp
    title: Allocations
    axis: 'allocs/op'
functions:
  - id: greater
    match: 'Greater'
  - id: less
    match: 'Less'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
  - id: summary
    derivedContext:
      formula: min
versions:
  - id: reflect
    match: '/reflect/'
  - id: generics
    match: '/generic/'
categories:
  - id: greater-only
    includes:
      functions: [greater]
      versions: [reflect, generics]
      contexts: [int, summary]
      metrics: [nsPerOp]
  - id: less-only
    includes:
      functions: [less]
      versions: [reflect]
      contexts: [float64]
      metrics: [allocsPerOp]
  - id: bottom-line
    derivedCategory:
      formula: max
`)
	o := New(cfg)

	scope := o.impliedScope()
	assert.Equal(t, []string{"greater", "less"}, scope.Functions)
	assert.Equal(t, []string{"int", "float64"}, scope.Contexts, "the derived context is left out")
	assert.Equal(t, []string{"reflect", "generics"}, scope.Versions)
	assert.Equal(t, []config.MetricName{config.MetricNsPerOp, config.MetricAllocsPerOp}, scope.Metrics)
}

// TestDerivedCategoryAggregatesAllContexts verifies that the bottom line spans every
// context of the scope, including those the other categories split across charts.
func TestDerivedCategoryAggregatesAllContexts(t *testing.T) {
	cfg := mustLoadConfig(t, `
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    match: 'Greater'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
versions:
  - id: reflect
    match: '/reflect/'
categories:
  - id: ints
    includes:
      functions: [greater]
      versions: [reflect]
      contexts: [int]
      metrics: [nsPerOp]
  - id: floats
    includes:
      functions: [greater]
      versions: [reflect]
      contexts: [float64]
      metrics: [nsPerOp]
  - id: bottom-line
    derivedCategory:
      formula: max
`)
	o := New(cfg)

	scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
	require.NoError(t, err)
	require.Len(t, scenario.Categories, 3)

	bottomLine := scenario.Categories[2]
	series := seriesOf(t, bottomLine, "reflect")
	require.Len(t, series.Points, 1)
	assert.InEpsilon(t, 100.0, series.Points[0].Value, 1e-9, "max over both charts, not just one")
}

// TestDerivedCategoryDropsUnmeasuredFunction verifies that a function without any
// measurement does not leave an empty tick on the bottom line.
func TestDerivedCategoryDropsUnmeasuredFunction(t *testing.T) {
	cfg := mustLoadConfig(t, `
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    match: 'Greater'
  - id: never
    match: 'NeverBenchmarked'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
versions:
  - id: reflect
    match: '/reflect/'
categories:
  - id: comparisons
    includes:
      functions: [greater, never]
      versions: [reflect]
      contexts: [int, float64]
      metrics: [nsPerOp]
  - id: bottom-line
    derivedCategory:
      formula: mean
`)
	o := New(cfg)

	scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
	require.NoError(t, err)

	bottomLine := scenario.Categories[len(scenario.Categories)-1]
	assert.Equal(t, []string{"Greater"}, bottomLine.XLabels, "the unmeasured function is dropped")
}

// TestDerivedCategoryNarrowedScope verifies that an explicit includes clause narrows what
// the bottom line aggregates, one dimension at a time: workloads that are not really
// comparable can be kept out, while the dimensions left unstated stay implied.
func TestDerivedCategoryNarrowedScope(t *testing.T) {
	// the measured set spans int (1 ns/op) and float64 (100 ns/op) for reflect
	narrowed := `
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    title: Greater
    match: 'Greater'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
versions:
  - id: reflect
    match: '/reflect/'
  - id: generics
    match: '/generic/'
categories:
  - id: comparisons
    includes:
      functions: [greater]
      versions: [reflect, generics]
      contexts: [int, float64]
      metrics: [nsPerOp]
  - id: bottom-line
    derivedCategory:
      formula: max
    includes:
      contexts: [int]
`
	cfg := mustLoadConfig(t, narrowed)
	o := New(cfg)

	// the contexts are narrowed down to int, everything else stays implied
	scope := o.scopeFor(cfg.Categories[1])
	assert.Equal(t, []string{"int"}, scope.Contexts)
	assert.Equal(t, []string{"greater"}, scope.Functions)
	assert.Equal(t, []string{"reflect", "generics"}, scope.Versions)
	assert.Equal(t, []config.MetricName{config.MetricNsPerOp}, scope.Metrics)

	scenario, err := o.Scenarize([]parser.Set{buildDerivedSet()})
	require.NoError(t, err)

	bottomLine := scenario.Categories[1]
	series := seriesOf(t, bottomLine, "reflect")
	require.Len(t, series.Points, 1)
	assert.InEpsilon(t, 1.0, series.Points[0].Value, 1e-9,
		"max over int alone, not over int and float64")
}

func derivedCategoryConfig(formula string) string {
	return fmt.Sprintf(`
name: derived
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    title: Greater
    match: 'Greater'
contexts:
  - id: int
    match: '/int'
  - id: float64
    match: '/float64'
versions:
  - id: reflect
    match: '/reflect/'
  - id: generics
    match: '/generic/'
categories:
  - id: comparisons
    includes:
      functions: [greater]
      versions: [reflect, generics]
      contexts: [int, float64]
      metrics: [nsPerOp]
  - id: bottom-line
    derivedCategory:
      formula: %s
`, formula)
}
