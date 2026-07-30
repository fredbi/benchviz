package organizer

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/fredbi/benchviz/internal/config"
	"github.com/fredbi/benchviz/internal/model"
	"github.com/fredbi/benchviz/internal/parser"
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

	return newBenchmarkSet(benchmarks), nil
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

// resolveLabels fills display strings from config Titles (overriding the ids):
// the series legend is the version Title (else its id), and each point's x-axis
// Label is the one of the column it sits on, as built by the labeller of the category.
func (v *Organizer) resolveLabels(series *model.MetricSeries, version config.Version, columns []column, label labeller) {
	legend := version.Title
	if legend == "" {
		legend = version.ID
	}

	series.Title = legend

	for pi := range series.Points {
		series.Points[pi].Label = label(columns[pi])
	}
}

// labeller builds the workload axis label of a column.
type labeller func(column) string

// contextLabeller labels a column after its context: the context Title (else its id),
// prefixed by the function Title (else its id) only when the chart plots more than one
// function — the prefix is redundant otherwise.
func (v *Organizer) contextLabeller(showFunction bool) labeller {
	return func(col column) string {
		ctxLabel := col.Context
		if ctx, ok := v.cfg.GetContext(col.Context); ok && ctx.Title != "" {
			ctxLabel = ctx.Title
		}

		if !showFunction {
			return ctxLabel
		}

		return v.functionLabel(col.Function) + " - " + ctxLabel
	}
}

// functionLabeller labels a column after its function alone.
//
// A derived category plots a single aggregated measurement per function: the context it
// aggregates has no name of its own to show.
func (v *Organizer) functionLabeller() labeller {
	return func(col column) string {
		return v.functionLabel(col.Function)
	}
}

// functionLabel returns the display name of a function.
func (v *Organizer) functionLabel(function string) string {
	if fn, ok := v.cfg.GetFunction(function); ok && fn.Title != "" {
		return fn.Title
	}

	return function
}

// columnLabels builds the workload axis labels of a category.
func (v *Organizer) columnLabels(columns []column, label labeller) []string {
	labels := make([]string, 0, len(columns))

	for _, col := range columns {
		labels = append(labels, label(col))
	}

	return labels
}

func (v *Organizer) populateCategories(set *BenchmarkSet) (*model.Scenario, error) {
	scenario := &model.Scenario{
		Name:       v.cfg.Name,
		Categories: make([]model.Category, 0, len(v.cfg.Categories)),
	}

	environment := v.cfg.Environment

	for _, categoryConfig := range v.cfg.Categories {
		var category model.Category

		if categoryConfig.IsDerived() {
			category = v.populateDerivedCategory(set, categoryConfig, environment)
		} else {
			category = v.populateCategory(set, categoryConfig, environment)
		}

		if len(category.Data) == 0 || len(category.XLabels) == 0 {
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

// populateCategory builds all the series of a single category.
//
// Every (metric, version) series is materialized against the same ordered column grid,
// so that all of them align with the workload axis labels. Columns that no series could
// fill are then discarded, together with their labels.
func (v *Organizer) populateCategory(set *BenchmarkSet, categoryConfig config.Category, environment string) model.Category {
	category := model.Category{
		ID:          categoryConfig.ID,
		Title:       categoryConfig.Title,
		Environment: stringDefault(environment, set.Environment()),
		Data:        make([]model.CategoryData, 0, len(categoryConfig.Includes.Metrics)),
	}

	columns := columnsFor(v.cfg, categoryConfig)
	label := v.contextLabeller(len(categoryConfig.Includes.Functions) > 1)

	v.buildSeries(&category, set, categoryConfig.Includes, columns, label)

	return category
}

// populateDerivedCategory builds the "bottom line" chart of a derived category: for each
// function, a single measurement aggregating all the contexts of the implied scope.
//
// Versions remain the series, so the chart still compares them — aggregating them away
// would collapse the very comparison the charts exist to make.
func (v *Organizer) populateDerivedCategory(set *BenchmarkSet, categoryConfig config.Category, environment string) model.Category {
	scope := v.scopeFor(categoryConfig)

	category := model.Category{
		ID:          categoryConfig.ID,
		Title:       categoryConfig.Title,
		Environment: stringDefault(environment, set.Environment()),
		Data:        make([]model.CategoryData, 0, len(scope.Metrics)),
	}

	// The measured columns are materialized so that they can be aggregated, then dropped:
	// only the aggregate of each function reaches the chart.
	columns := derivedColumnsFor(scope, categoryConfig.DerivedCategory)

	v.buildSeries(&category, set, scope, columns, v.functionLabeller())

	return category
}

// buildSeries materializes one series per (metric, version) of the scope against the
// column grid, then trims the grid down to the columns worth displaying.
func (v *Organizer) buildSeries(category *model.Category, set *BenchmarkSet, scope config.Includes, columns []column, label labeller) {
	for _, metricID := range scope.Metrics {
		metric, _ := v.cfg.GetMetric(metricID)

		for _, versionID := range scope.Versions {
			version, _ := v.cfg.GetVersion(versionID)

			series := set.SeriesFor(metric.ID, version.ID, columns)
			fillDerivedColumns(&series, columns)
			v.resolveLabels(&series, version, columns, label)

			category.Data = append(category.Data, model.CategoryData{
				Metric:  metric,
				Version: version,
				Series:  []model.MetricSeries{series},
			})
		}
	}

	columns = v.trimColumns(category, columns)
	category.XLabels = v.columnLabels(columns, label)
}

// trimColumns discards the columns that no series could fill, together with their points,
// and returns the surviving columns.
func (v *Organizer) trimColumns(category *model.Category, columns []column) []column {
	keep := keepColumns(columns, allSeries(category.Data))

	for i := range category.Data {
		for j := range category.Data[i].Series {
			prunePoints(&category.Data[i].Series[j], keep)
		}
	}

	return pruneColumns(columns, keep)
}

// scopeFor resolves what a derived category aggregates.
//
// Each dimension it states explicitly is honoured; the rest is implied from the other
// categories. Narrowing a dimension is how workloads that are not really comparable are
// kept out of the same bottom line — restricting the contexts to the small ones, say,
// when the large ones run on a different order of magnitude.
func (v *Organizer) scopeFor(categoryConfig config.Category) config.Includes {
	scope := categoryConfig.Includes
	implied := v.impliedScope()

	if len(scope.Functions) == 0 {
		scope.Functions = implied.Functions
	}

	if len(scope.Contexts) == 0 {
		scope.Contexts = implied.Contexts
	}

	if len(scope.Versions) == 0 {
		scope.Versions = implied.Versions
	}

	if len(scope.Metrics) == 0 {
		scope.Metrics = implied.Metrics
	}

	return scope
}

// impliedScope returns the default scope of the derived categories: the union of the
// includes of all the non derived categories, in first-seen declaration order.
//
// Derived contexts are left out — an aggregate never feeds another aggregate.
func (v *Organizer) impliedScope() config.Includes {
	var scope config.Includes

	seenFunction := make(map[string]struct{})
	seenContext := make(map[string]struct{})
	seenVersion := make(map[string]struct{})
	seenMetric := make(map[config.MetricName]struct{})

	for _, categoryConfig := range v.cfg.Categories {
		if categoryConfig.IsDerived() {
			continue
		}

		includes := categoryConfig.Includes

		for _, function := range includes.Functions {
			if _, seen := seenFunction[function]; seen {
				continue
			}
			seenFunction[function] = struct{}{}
			scope.Functions = append(scope.Functions, function)
		}

		for _, contextID := range includes.Contexts {
			if _, seen := seenContext[contextID]; seen {
				continue
			}
			seenContext[contextID] = struct{}{}

			if context, ok := v.cfg.GetContext(contextID); ok && context.IsDerived() {
				continue
			}

			scope.Contexts = append(scope.Contexts, contextID)
		}

		for _, version := range includes.Versions {
			if _, seen := seenVersion[version]; seen {
				continue
			}
			seenVersion[version] = struct{}{}
			scope.Versions = append(scope.Versions, version)
		}

		for _, metric := range includes.Metrics {
			if _, seen := seenMetric[metric]; seen {
				continue
			}
			seenMetric[metric] = struct{}{}
			scope.Metrics = append(scope.Metrics, metric)
		}
	}

	return scope
}

// allSeries flattens the series of a category.
func allSeries(data []model.CategoryData) []model.MetricSeries {
	all := make([]model.MetricSeries, 0, len(data))

	for _, d := range data {
		all = append(all, d.Series...)
	}

	return all
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

	measures map[model.SeriesKey]measure
}

// measure accumulates the repeated samples of a single measurement point.
//
// The same benchmark is routinely sampled several times (go test -count=N) and may also
// appear in several input files: all the samples of a (function, version, context, metric)
// are folded into their arithmetic mean, so that a measurement occupies exactly one column.
type measure struct {
	total float64
	count int
}

// value returns the mean of the accumulated samples.
func (m measure) value() float64 {
	return m.total / float64(m.count)
}

// newBenchmarkSet indexes parsed benchmarks by their series key.
func newBenchmarkSet(benchmarks []ParsedBenchmark) *BenchmarkSet {
	measures := make(map[model.SeriesKey]measure, len(benchmarks))

	for _, bench := range benchmarks {
		m := measures[bench.SeriesKey]
		m.total += bench.Value
		m.count++
		measures[bench.SeriesKey] = m
	}

	return &BenchmarkSet{
		Set:      benchmarks,
		measures: measures,
	}
}

// Measure returns the measurement recorded for a series key, if any.
func (s BenchmarkSet) Measure(key model.SeriesKey) (float64, bool) {
	m, ok := s.measures[key]
	if !ok {
		return 0, false
	}

	return m.value(), true
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

// SeriesFor extracts a single series for 1 metric and 1 version, materialized over the
// given columns.
//
// The series holds exactly one point per column, in column order: columns without a
// measurement carry a point flagged [model.MetricPoint.Missing], so that the series stays
// aligned with the workload axis.
func (s BenchmarkSet) SeriesFor(metric config.MetricName, version string, columns []column) model.MetricSeries {
	series := model.MetricSeries{
		SeriesKey: model.SeriesKey{
			Version: version,
			Metric:  metric,
		},
		Title:  version, // the version gives the series name (e.g. to display as a legend)
		Points: make([]model.MetricPoint, 0, len(columns)),
	}

	for _, col := range columns {
		key := model.SeriesKey{
			Function: col.Function,
			Version:  version,
			Context:  col.Context,
			Metric:   metric,
		}

		point := model.MetricPoint{
			SeriesKey: key,
			Name:      col.Function + " - " + version + " - " + col.Context, // the point name (e.g. to display as a tooltip)
			Missing:   true,
		}

		// A derived column never holds a measurement of its own: it is filled later, by
		// aggregating the measured columns of the same function.
		if !col.IsDerived() {
			if value, ok := s.Measure(key); ok {
				point.Value = value
				point.Missing = false
			}
		}

		series.Points = append(series.Points, point)
	}

	return series
}

func stringDefault(in, def string) string {
	if in == "" {
		return def
	}
	return in
}
