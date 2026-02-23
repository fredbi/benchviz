package chart

import (
	"fmt"
	"log/slog"

	"github.com/fredbi/benchviz/internal/config"
	"github.com/fredbi/benchviz/internal/model"
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

	showLegend := b.cfg.Render.Legend != config.LegendPositionNone
	title := category.TitleWithPlaceHolders(metric)
	yAxis := metric.Title + " (" + metric.Axis + ")"

	opts := []Option{
		WithTitle(title),
		WithXAxisLabels(category.Labels()),
		WithYAxisLabel(yAxis),
		WithSubtitle(category.Environment),
		WithLegend(showLegend),
		WithLegendPosition(string(b.cfg.Render.Legend)),
		WithHorizontal(b.cfg.Render.Orientation == config.OrientationHorizontal),
	}

	if w, h := b.chartSize(); w != "" {
		opts = append(opts, WithSize(w, h))
	}

	chart := NewChart(opts...)

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

// chartSize computes the chart canvas dimensions from the layout config.
//
// When Layout.Horizontal > 1, the width is divided among that many charts per row.
// A small gap is subtracted so charts don't touch.
func (b *Builder) chartSize() (width, height string) {
	cols := b.cfg.Render.Layout.Horizontal
	if cols <= 1 {
		return "", "" // use go-echarts defaults (900px × 500px)
	}

	pct := 100 / cols
	width = fmt.Sprintf("%d%%", pct)

	rows := b.cfg.Render.Layout.Vertical
	if rows > 1 {
		// Divide viewport height among rows, capped at a reasonable minimum.
		vpct := 100 / rows
		height = fmt.Sprintf("%dvh", vpct)
	}

	return width, height
}
