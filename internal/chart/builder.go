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

// defaultPageTitle is the HTML <title> used when neither render.title nor the
// scenario name is configured (avoids go-echarts' "Awesome go-echarts" default).
const defaultPageTitle = "Benchmark results"

// BuildPage creates a page with all charts for all metrics and categories.
func (b *Builder) BuildPage() *Page {
	page := NewPage(b.pageTitle())

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

// pageTitle resolves the HTML page title: the configured render.title takes
// precedence, then the scenario name, then a benchviz default.
func (b *Builder) pageTitle() string {
	if b.cfg.Render.Title != "" {
		return b.cfg.Render.Title
	}

	if b.scenario.Name != "" {
		return b.scenario.Name
	}

	return defaultPageTitle
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

	if b.cfg.Render.Theme != "" {
		opts = append(opts, WithTheme(b.cfg.Render.Theme))
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

// Nominal page dimensions used to derive per-chart canvas sizes from the layout config.
//
// They are picked so that the common horizontal:2 case yields the go-echarts default
// canvas (900px × 500px), preserving the historical, known-good layout.
const (
	nominalPageWidth  = 1800
	nominalPageHeight = 1000
)

// chartSize computes the chart canvas dimensions from the layout config.
//
// The dimensions must be concrete pixel values: the page uses a flex-wrap layout where
// each chart renders into an .item nested inside a content-sized .container. A percentage
// width would resolve against that indefinite container width and collapse to nothing,
// cramming every chart onto a single row. Pixel widths always resolve, and flex-wrap then
// packs as many charts per row as the viewport allows.
//
// When Layout.Horizontal > 1, the nominal page width is divided among that many charts, so
// a wider column count produces proportionally narrower charts that fit more per row.
// Layout.Vertical divides the nominal page height likewise.
func (b *Builder) chartSize() (width, height string) {
	cols := b.cfg.Render.Layout.Horizontal
	if cols <= 1 {
		return "", "" // use go-echarts defaults (900px × 500px)
	}

	width = fmt.Sprintf("%dpx", nominalPageWidth/cols)

	rows := b.cfg.Render.Layout.Vertical
	if rows > 1 {
		height = fmt.Sprintf("%dpx", nominalPageHeight/rows)
	}

	return width, height
}
