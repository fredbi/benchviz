package chart

import (
	"log/slog"

	"github.com/fredbi/benchviz/internal/pkg/config"
	"github.com/fredbi/benchviz/internal/pkg/model"
)

// Builder constructs charts from scenarized benchmark data.
type Builder struct {
	cfg      *config.Config
	scenario *model.Scenario
	l        *slog.Logger
}

// New creates a new chart [Builder], given a [config.Config] and a pre-calculated [model.Scenario].
//
// The builder embeds a [slog.Logger] to croak about warnings and issues.
func New(cfg *config.Config, scenario *model.Scenario) *Builder {
	return &Builder{
		cfg:      cfg,
		scenario: scenario,
		l:        slog.Default().With(slog.String("module", "chart")),
	}
}

// BuildPage creates a page with all charts for all metrics and categories.
func (b *Builder) BuildPage() *Page {
	page := NewPage(b.scenario.Name)

	for _, category := range b.scenario.Categories {
		for _, metric := range category.Metrics() {
			chart := b.buildChartForMetric(category, metric)
			if chart == nil {
				b.l.Warn("empty chart skipped", slog.String("category_id", category.ID))

				continue
			}

			page.AddChart(chart)
			b.l.Info("added chart", slog.String("category_id", category.ID))
		}
	}

	b.l.Info("added charts", slog.Int("charts", len(page.Charts)))

	return page
}

// buildChart creates a single chart for one metric (possibly two) and one category.
func (b *Builder) buildChartForMetric(category model.Category, metric config.Metric) *Chart {
	if len(category.Data) == 0 {
		return nil
	}

	// layoutConfig := b.cfg.Render // TODO
	showLegend := b.cfg.Render.Legend != config.LegendPositionNone
	title := category.TitleWithPlaceHolders(metric)
	yAxis := metric.Title + " (" + metric.Axis + ")"

	chart := NewChart(
		WithTitle(title),
		WithXAxisLabels(category.Labels()),
		WithYAxisLabel(yAxis),
		WithSubtitle(category.Environment),
		WithLegend(showLegend), // TODO: configurable legend position
		WithHorizontal(b.cfg.Render.Orientation == config.OrientationHorizontal),
	)

	for _, data := range category.Data { // iterate the series in a category
		for _, series := range data.Series { // each category, iterate over series
			if series.Metric != metric.ID {
				continue
			}

			chart.AddSeries(series)

			b.l.Info("added series",
				slog.String("category_id", category.ID),
				slog.String("metric_id", data.Metric.ID.String()),
				slog.String("version_id", data.Version.ID),
			)
		}
	}

	return chart
}
