package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fredbi/benchviz/internal/config"
	"github.com/fredbi/benchviz/internal/parser"
	"golang.org/x/tools/benchmark/parse"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestNew(t *testing.T) {
	cfg := mustLoadConfig(t, minimalConfig())
	o := New(cfg)
	require.NotNil(t, o)
	assert.Equal(t, cfg, o.cfg)
}

func TestParseBenchmarkName(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	tests := []struct {
		name         string
		benchName    string
		file         string
		env          string
		wantOk       bool
		wantFunction string
		wantVersion  string
		wantContext  string
	}{
		{
			name:         "full match from name",
			benchName:    "BenchmarkGreater/reflect/int-16",
			wantOk:       true,
			wantFunction: "greater",
			wantVersion:  "reflect",
			wantContext:  "int",
		},
		{
			name:         "generic version",
			benchName:    "BenchmarkGreater/generic/float64-16",
			wantOk:       true,
			wantFunction: "greater",
			wantVersion:  "generics",
			wantContext:  "float64",
		},
		{
			name:         "less function",
			benchName:    "BenchmarkLess/reflect/int-16",
			wantOk:       true,
			wantFunction: "less",
			wantVersion:  "reflect",
			wantContext:  "int",
		},
		{
			name:         "excluded by NotMatch",
			benchName:    "BenchmarkGreaterOrEqual/reflect/int-16",
			wantOk:       false,
			wantFunction: "",
		},
		{
			name:         "no function match",
			benchName:    "BenchmarkUnknown/reflect/int-16",
			wantOk:       false,
			wantFunction: "",
		},
		{
			name:         "negative function",
			benchName:    "BenchmarkNegative/reflect/int-16",
			wantOk:       true,
			wantFunction: "negative",
			wantVersion:  "reflect",
			wantContext:  "int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, ok := o.parseBenchmarkName(tt.benchName, tt.file, tt.env)
			require.Equal(t, tt.wantOk, ok, "parseBenchmarkName(%q) ok", tt.benchName)
			if !ok {
				return
			}
			assert.Equal(t, tt.wantFunction, parsed.Function)
			assert.Equal(t, tt.wantVersion, parsed.Version)
			assert.Equal(t, tt.wantContext, parsed.Context)
		})
	}
}

// TestParseBenchmarkNameContextFallback verifies that when the context
// is not found in the benchmark name, it falls back to file-based matching.
func TestParseBenchmarkNameContextFallbackBug(t *testing.T) {
	cfg := mustLoadConfig(t, configWithFileFallback())
	o := New(cfg)

	// The benchmark name contains the function but NOT the version or context.
	// Both should fall back to file-based matching.
	parsed, ok := o.parseBenchmarkName(
		"BenchmarkGreater-16",       // no version/context in name
		"bench_reflect_int_test.go", // file should match version=reflect, context=int
		"linux amd64",
	)
	require.True(t, ok, "expected parseBenchmarkName to succeed")
	assert.Equal(t, "reflect", parsed.Version, "version file fallback")
	assert.Equal(t, "int", parsed.Context, "context file fallback")
}

func TestParseBenchmarks(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}

	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)
	require.NotEmpty(t, benchSet.Set)

	// The config has 2 metrics (nsPerOp, allocsPerOp).
	// The generics set has 4 benchmarks (Greater reflect/int, Greater generic/int,
	// Greater reflect/float64, Greater generic/float64).
	// Each benchmark should produce 2 ParsedBenchmarks (one per metric).
	// Total: 4 * 2 = 8
	assert.Len(t, benchSet.Set, 8)

	// Verify we have the right metrics
	metrics := make(map[config.MetricName]int)
	for _, b := range benchSet.Set {
		metrics[b.Metric]++
	}
	assert.Equal(t, 4, metrics[config.MetricNsPerOp])
	assert.Equal(t, 4, metrics[config.MetricAllocsPerOp])
}

func TestParseBenchmarksEmpty(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	benchSet, err := o.parseBenchmarks(nil)
	require.NoError(t, err)
	assert.Empty(t, benchSet.Set)
}

func TestParseBenchmarksSkipsUnmatched(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{{
		Set: parse.Set{
			"BenchmarkUnknown-16": []*parse.Benchmark{
				{Name: "BenchmarkUnknown-16", N: 1000, NsPerOp: 100},
			},
		},
	}}

	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)
	assert.Empty(t, benchSet.Set)
}

func TestSeriesFor(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]

	series := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", columnsFor(cfg, category))

	// For version "reflect", the category includes function "greater"
	// and contexts "int" and "float64" → 1 series with 2 points.
	assert.Equal(t, "reflect", series.Title)
	require.Len(t, series.Points, 2)
	for _, p := range series.Points {
		assert.False(t, p.Missing, "expected a measurement for %q", p.Name)
		assert.Positive(t, p.Value, "expected positive value for %q", p.Name)
	}
}

func TestSeriesForNoMatch(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]

	// Query a version that doesn't exist in the data: the series still covers every
	// column, with all its points flagged missing.
	series := benchSet.SeriesFor(config.MetricNsPerOp, "nonexistent", columnsFor(cfg, category))
	require.Len(t, series.Points, 2)
	for _, p := range series.Points {
		assert.True(t, p.Missing, "expected no measurement for %q", p.Name)
	}
}

// TestPopulateCategories verifies that populateCategories produces
// exactly the right number of categories.
func TestPopulateCategoriesBug(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	scenario, err := o.populateCategories(benchSet)
	require.NoError(t, err)

	// Config has 1 category. With the bug, scenario.Categories has
	// 1 empty + 1 real = 2 entries. Without the bug, just 1.
	assert.Len(t, scenario.Categories, 1)
}

func TestScenarize(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)

	require.NotNil(t, scenario)
	assert.Equal(t, "test-scenario", scenario.Name)

	// Filter out empty categories (due to the prepend bug)
	var nonEmpty int
	for _, cat := range scenario.Categories {
		if cat.ID != "" {
			nonEmpty++
		}
	}
	assert.Equal(t, 1, nonEmpty)
}

func TestScenarizeEnvironment(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	cfg.Environment = "test-env"
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)

	for _, cat := range scenario.Categories {
		if cat.ID == "" {
			continue
		}
		assert.Equal(t, "test-env", cat.Environment)
	}
}

func TestScenarizeEmptySets(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	scenario, err := o.Scenarize(nil)
	require.NoError(t, err)
	require.NotNil(t, scenario)
}

// TestSeriesStayAlignedOnHole guards the workload axis alignment: series data is mapped
// to the axis by index, so a version missing one measurement must still emit a point on
// that column. Skipping it used to shift every following point by one tick.
func TestSeriesStayAlignedOnHole(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	// "generics" has no float64 measurement, "reflect" has both contexts.
	sets := []parser.Set{buildGenericsSetWithHole()}
	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)
	require.Len(t, scenario.Categories, 1)

	category := scenario.Categories[0]
	require.Len(t, category.XLabels, 2, "the float64 column is kept: reflect fills it")

	for _, data := range category.Data {
		for _, series := range data.Series {
			require.Len(t, series.Points, len(category.XLabels),
				"series %q (%s) must hold one point per axis label", series.Title, data.Metric.ID)

			for i, point := range series.Points {
				assert.Equal(t, category.XLabels[i], point.Label,
					"point %d of series %q sits on the wrong column", i, series.Title)
			}
		}
	}

	// The hole is flagged, on the float64 column only, for the generics version.
	for _, data := range category.Data {
		if data.Version.ID != "generics" {
			continue
		}

		require.Len(t, data.Series, 1)
		points := data.Series[0].Points
		assert.False(t, points[0].Missing, "int is measured for generics")
		assert.True(t, points[1].Missing, "float64 is not measured for generics")
		assert.Zero(t, points[1].Value)
	}
}

// TestEmptyColumnsArePruned verifies that a column no version could fill is dropped
// altogether, rather than showing as a labelled tick with no bar.
func TestEmptyColumnsArePruned(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSetIntOnly()}
	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)
	require.Len(t, scenario.Categories, 1)

	category := scenario.Categories[0]
	require.Len(t, category.XLabels, 1, "float64 has no measurement at all: column dropped")
	assert.Equal(t, "Int", category.XLabels[0])

	for _, data := range category.Data {
		for _, series := range data.Series {
			assert.Len(t, series.Points, 1)
		}
	}
}

// TestRepeatedSamplesAreAveraged verifies that the repeated samples of a benchmark
// (go test -count=N, or the same benchmark across input files) collapse into a single
// measurement instead of piling up extra points on the series.
func TestRepeatedSamplesAreAveraged(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{{
		Set: parse.Set{
			"BenchmarkGreater/reflect/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/int-16", N: 1000, NsPerOp: 100},
				{Name: "BenchmarkGreater/reflect/int-16", N: 1000, NsPerOp: 300},
			},
		},
		File: "test.json",
	}}

	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	series := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", columnsFor(cfg, cfg.Categories[0]))
	require.Len(t, series.Points, 2)
	assert.InDelta(t, 200.0, series.Points[0].Value, 1e-9, "the two int samples average to 200")
	assert.True(t, series.Points[1].Missing)
}

func TestDefaultString(t *testing.T) {
	tests := []struct {
		in, def, want string
	}{
		{"value", "default", "value"},
		{"", "default", "default"},
		{"", "", ""},
		{"value", "", "value"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, defaultString(tt.in, tt.def))
	}
}

func TestParseBenchmarkNameEnvironment(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	parsed, ok := o.parseBenchmarkName("BenchmarkGreater/reflect/int-16", "file.txt", "linux amd64")
	require.True(t, ok)
	assert.Equal(t, "linux amd64", parsed.Environment)

	// Config environment takes precedence
	cfg.Environment = "override-env"
	parsed, ok = o.parseBenchmarkName("BenchmarkGreater/reflect/int-16", "file.txt", "linux amd64")
	require.True(t, ok)
	assert.Equal(t, "override-env", parsed.Environment)
}

func TestSeriesForPointNames(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]
	series := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", columnsFor(cfg, category))

	require.NotEmpty(t, series.Points)

	// Verify point names follow the pattern "function - version - context"
	for _, point := range series.Points {
		assert.NotEmpty(t, point.Name)
		// Name should contain function, version and context
		for _, part := range []string{"greater", "reflect"} {
			assert.Contains(t, point.Name, part)
		}
	}
}

func TestMultipleVersionSeries(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]
	columns := columnsFor(cfg, category)

	// Get series for both versions
	reflectSeries := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", columns)
	genericsSeries := benchSet.SeriesFor(config.MetricNsPerOp, "generics", columns)

	require.NotEmpty(t, reflectSeries.Points)
	require.NotEmpty(t, genericsSeries.Points)

	// Both series cover the same columns, in the same order.
	assert.Len(t, genericsSeries.Points, len(reflectSeries.Points))

	// Generic benchmarks should have lower ns/op values in our test data
	if genericsSeries.Points[0].Value >= reflectSeries.Points[0].Value {
		t.Logf("Note: generic ns/op (%f) >= reflect ns/op (%f) - unexpected for test data",
			genericsSeries.Points[0].Value, reflectSeries.Points[0].Value)
	}
}

// helpers

func mustLoadConfig(t *testing.T, yamlContent string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yamlContent), 0o600))
	cfg, err := config.Load(file)
	require.NoError(t, err)
	return cfg
}

func buildGenericsSet() parser.Set {
	return parser.Set{
		Set: parse.Set{
			"BenchmarkGreater/reflect/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/int-16", N: 5000000, NsPerOp: 245.3, AllocedBytesPerOp: 64, AllocsPerOp: 2},
			},
			"BenchmarkGreater/generic/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/int-16", N: 150000000, NsPerOp: 7.89, AllocedBytesPerOp: 0, AllocsPerOp: 0},
			},
			"BenchmarkGreater/reflect/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/float64-16", N: 4500000, NsPerOp: 267.8, AllocedBytesPerOp: 64, AllocsPerOp: 2},
			},
			"BenchmarkGreater/generic/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/float64-16", N: 140000000, NsPerOp: 8.12, AllocedBytesPerOp: 0, AllocsPerOp: 0},
			},
		},
		File:        "test.json",
		Environment: "linux amd64 cpu: Test CPU",
	}
}

// buildGenericsSetWithHole omits the generics/float64 benchmark: the float64 column is
// measured by "reflect" only.
func buildGenericsSetWithHole() parser.Set {
	set := buildGenericsSet()
	delete(set.Set, "BenchmarkGreater/generic/float64-16")

	return set
}

// buildGenericsSetIntOnly omits every float64 benchmark: the float64 column is empty.
func buildGenericsSetIntOnly() parser.Set {
	set := buildGenericsSet()
	delete(set.Set, "BenchmarkGreater/generic/float64-16")
	delete(set.Set, "BenchmarkGreater/reflect/float64-16")

	return set
}

func genericsConfig() string {
	return `
name: test-scenario
metrics:
  - id: nsPerOp
    title: Benchmark Timings
    axis: 'ns/op'
  - id: allocsPerOp
    title: Benchmark Allocations
    axis: 'allocs/op'
functions:
  - id: greater
    title: Greater
    Match: 'GreaterT?'
    NotMatch: 'GreaterOr'
  - id: less
    title: Less
    Match: 'LessT?'
    NotMatch: 'LessOr'
  - id: negative
    title: Negative
    Match: 'NegativeT?'
contexts:
  - id: int
    Match: '/int'
  - id: float64
    Match: '/float64'
versions:
  - id: reflect
    Match: '/reflect/'
  - id: generics
    Match: '/generic/'
categories:
  - id: comparisons
    title: Comparisons
    includes:
      functions: [greater]
      versions: [reflect, generics]
      contexts: [int, float64]
      metrics: [nsPerOp, allocsPerOp]
`
}

func minimalConfig() string {
	return `
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: fn1
    Match: 'Bench'
categories:
  - id: cat1
    includes:
      functions: [fn1]
      metrics: [nsPerOp]
`
}

func configWithFileFallback() string {
	return `
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    title: Greater
    Match: 'Greater'
    NotMatch: 'GreaterOr'
contexts:
  - id: int
  - id: float64
versions:
  - id: reflect
  - id: generics
categories:
  - id: cat1
    includes:
      functions: [greater]
      metrics: [nsPerOp]
files:
  - id: benchfile
    MatchFile: 'bench_.*_test'
    contexts:
      - id: int
        Match: '_int_'
    versions:
      - id: reflect
        Match: '_reflect_'
      - id: generics
        Match: '_generics_'
`
}
