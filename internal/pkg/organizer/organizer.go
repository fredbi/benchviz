package organizer

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/fredbi/benchviz/internal/pkg/config"
	"github.com/fredbi/benchviz/internal/pkg/model"
	"github.com/fredbi/benchviz/internal/pkg/parser"
)

// Organizer rearranges parsed benchmark data into a configured visualization scenario.
type Organizer struct {
	options //nolint:unused // reserved for future extensions

	cfg *config.Config
	l   *slog.Logger
}

// New builds an [Organizer] ready to reshuffle parsed benchmark data.
func New(cfg *config.Config, _ ...Option) *Organizer {
	return &Organizer{
		cfg: cfg,
		l:   slog.Default().With(slog.String("module", "organizer")),
	}
}

// Scenarize a set of parsed benchmark data into a visualization [model.Scenario].
func (v *Organizer) Scenarize(sets []parser.Set) (*model.Scenario, error) {
	newSet, err := v.parseBenchmarks(sets)
	if err != nil {
		return nil, err
	}

	scenario, err := v.populateCategories(newSet)
	if err != nil {
		return nil, err
	}

	return scenario, nil
}

// parseBenchmarks extracts structured data from raw benchmark results.
func (v *Organizer) parseBenchmarks(sets []parser.Set) (*BenchmarkSet, error) {
	var benchmarks []ParsedBenchmark

	for _, set := range sets {
		file := set.File
		env := set.Environment

		for _, benchs := range set.Set {
			for _, bench := range benchs {
				parsed, ok := v.parseBenchmarkName(bench.Name, file, env)
				if !ok {
					v.l.Warn("benchmark not ingested", slog.String("file", file), slog.String("benchmark_name", bench.Name))
					if v.cfg.IsStrict {
						err := fmt.Errorf("strict requirement not met for benchmark %q: not ingested. Stopping here", bench.Name)
						v.l.Error("strict requirement not met", slog.String("error", err.Error()))

						return nil, err
					}

					continue
				}

				var resolved bool
				benchmarks, ok = v.resolveMetric(config.MetricNsPerOp, parsed, bench.NsPerOp, benchmarks)
				resolved = resolved || ok
				benchmarks, ok = v.resolveMetric(config.MetricAllocsPerOp, parsed, float64(bench.AllocsPerOp), benchmarks)
				resolved = resolved || ok
				benchmarks, ok = v.resolveMetric(config.MetricBytesPerOp, parsed, float64(bench.AllocedBytesPerOp), benchmarks)
				resolved = resolved || ok
				benchmarks, ok = v.resolveMetric(config.MetricMBPerS, parsed, bench.MBPerS, benchmarks)
				resolved = resolved || ok

				if !resolved {
					v.l.Warn("no benchmark metric ingested", slog.String("file", file), slog.String("benchmark_name", bench.Name))
					if v.cfg.IsStrict {
						err := fmt.Errorf("strict requirement not met for benchmark %q: empty series. Stopping here", bench.Name)
						v.l.Error("strict requirement not met", slog.String("error", err.Error()))

						return nil, err
					}
				}
			}
		}
	}

	if len(benchmarks) == 0 {
		v.l.Warn("benchmark set is empty")
		if v.cfg.IsStrict {
			err := errors.New("strict requirement not met for benchmark %q: empty benchmark set. Stopping here")
			v.l.Error("strict requirement not met", slog.String("error", err.Error()))

			return nil, err
		}
	}

	return &BenchmarkSet{
		Set: benchmarks,
	}, nil
}

func (v *Organizer) resolveMetric(search config.MetricName, parsed ParsedBenchmark, value float64, benchmarks []ParsedBenchmark) ([]ParsedBenchmark, bool) {
	if metric, ok := v.cfg.GetMetric(search); ok {
		parsed.Metric = metric.ID
		parsed.Name = metric.Title
		parsed.Value = value
		benchmarks = append(benchmarks, parsed)

		return benchmarks, true
	}

	return benchmarks, false
}

/*
	if metric, ok := v.cfg.GetMetric(config.MetricAllocsPerOp); ok {
		parsed.Metric = metric.ID
		parsed.Name = metric.Title
		parsed.Value = float64(bench.AllocsPerOp)
		benchmarks = append(benchmarks, parsed)
		resolved = true
	}

	if metric, ok := v.cfg.GetMetric(config.MetricBytesPerOp); ok {
		parsed.Metric = metric.ID
		parsed.Name = metric.Title
		parsed.Value = float64(bench.AllocedBytesPerOp)
		benchmarks = append(benchmarks, parsed)
		resolved = true
	}

	if metric, ok := v.cfg.GetMetric(config.MetricMBPerS); ok {
		parsed.Metric = metric.ID
		parsed.Name = metric.Title
		parsed.Value = float64(bench.MBPerS)
		benchmarks = append(benchmarks, parsed)
		resolved = true
	}
*/

func (v *Organizer) populateCategories(set *BenchmarkSet) (*model.Scenario, error) {
	scenario := &model.Scenario{
		Name:       v.cfg.Name,
		Categories: make([]model.Category, 0, len(v.cfg.Categories)),
	}

	environment := v.cfg.Environment

	for _, categoryConfig := range v.cfg.Categories {
		category := model.Category{
			ID:    categoryConfig.ID,
			Title: categoryConfig.Title,
			Data:  make([]model.CategoryData, 0, len(categoryConfig.Includes.Metrics)),
		}

		var data model.CategoryData
		for _, metricID := range categoryConfig.Includes.Metrics {
			metric, _ := v.cfg.GetMetric(metricID)
			for _, versionID := range categoryConfig.Includes.Versions {
				version, _ := v.cfg.GetVersion(versionID)
				data.Metric = metric
				data.Version = version
				data.Series = set.SeriesFor(metric.ID, version.ID, categoryConfig)
				category.Data = append(category.Data, data)
				category.Environment = stringDefault(environment, set.Environment())
			}
		}

		if len(category.Data) == 0 {
			v.l.Warn("no data resolved for category", slog.String("category", category.ID))
			if v.cfg.IsStrict {
				err := fmt.Errorf("strict requirement not met for category %q: no data for category. Stopping here", category.ID)
				v.l.Error("strict requirement not met", slog.String("error", err.Error()))

				return nil, err
			}

			continue
		}

		scenario.Categories = append(scenario.Categories, category)
	}

	v.l.Info("resolved categories", slog.Int("categories", len(scenario.Categories)))

	return scenario, nil
}

// parseBenchmarkName extracts function, version, and context from a benchmark name.
//
// Supports multiple formats:
//
// Examples:
//
//   - Generics: "BenchmarkPositive/reflect/int-16" → (Positive, reflect, int)
//   - EasyJSON: "BenchmarkReadJSON_small" → (ReadJSON, stdlib, small)
//   - EasyJSON: "BenchmarkReadJSON_easyjson_large" → (ReadJSON, easyjson, large)
func (v *Organizer) parseBenchmarkName(name, file, env string) (ParsedBenchmark, bool) {
	function, ok := v.cfg.FindFunction(name)
	if !ok {
		v.l.Warn("no function matched", slog.String("function", name))

		return ParsedBenchmark{}, false // exclude benchmarks with non-identified functions
	}

	version, ok := v.cfg.FindVersion(name)
	if !ok {
		// fall back on file-based rule
		version, _ = v.cfg.FindVersionFromFile(file)
	}

	context, ok := v.cfg.FindContext(name)
	if !ok {
		// fall back on file-based rule
		context, _ = v.cfg.FindContextFromFile(file)
	}

	if version == "" && context == "" {
		v.l.Warn("no version, no context matched", slog.String("function", name))
	}

	return ParsedBenchmark{
		SeriesKey: model.SeriesKey{
			Function: function,
			Version:  version,
			Context:  context,
		},
		Environment: defaultString(v.cfg.Environment, env),
	}, true
}

func defaultString(in, def string) string {
	if in == "" {
		return def
	}

	return in
}

// ParsedBenchmark represents a benchmark result with extracted components.
type ParsedBenchmark struct {
	model.SeriesKey
	model.MetricPoint

	Environment string // benchmark-specific environment // TODO: we may have 1 or several values for environment - rendering to be figured out
}

// BenchmarkSet holds parsed benchmarks organized for chart generation.
type BenchmarkSet struct {
	Set []ParsedBenchmark
}

// Environment returns the first non-empty environment string found in the benchmark set.
func (s BenchmarkSet) Environment() string {
	for _, set := range s.Set {
		if env := set.Environment; env != "" {
			return env
		}
	}

	return ""
}

// SeriesFor extracts a single series for 1 metric, 1 version for the filtered category.
//
// The points of the series correspond to different context values.
func (s BenchmarkSet) SeriesFor(metric config.MetricName, version string, filter config.Category) []model.MetricSeries {
	series := []model.MetricSeries{
		{
			SeriesKey: model.SeriesKey{
				Version: version,
				Metric:  metric,
			},
			Title: version, // the version gives the series name (e.g. to display as a legend)
		},
	}
	var points []model.MetricPoint

	for _, wantFunction := range filter.Includes.Functions {
		for _, wantContext := range filter.Includes.Contexts {
			for _, bench := range s.Set {
				if bench.Metric != metric || bench.Function != wantFunction || bench.Version != version || bench.Context != wantContext {
					continue
				}

				points = append(points, model.MetricPoint{
					SeriesKey: model.SeriesKey{
						Function: bench.Function,
						Version:  bench.Version,
						Context:  bench.Context,
						Metric:   bench.Metric,
					},
					Name:  bench.Function + " - " + bench.Version + " - " + bench.Context, // the point name (e.g. to display as a tooltip)
					Value: bench.Value,
				})
			}
		}
	}
	series[0].Points = points

	return series
}

func stringDefault(in, def string) string {
	if in == "" {
		return def
	}
	return in
}
