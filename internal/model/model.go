package model

import (
	"strings"

	"github.com/fredbi/benchviz/internal/config"
)

// Scenario defines a complete configuration for benchmark visualization on a single page.
//
// A [Scenario] exposes several categories, each to be rendered in a separate chart on the page.
type Scenario struct {
	Name       string
	Categories []Category
}

// Category defines all the series for one or two metrics, regrouped on a single chart.
//
// Multiple versions correspond to several bar series represented side by side.
//
// Each point of a series corresponds to several contexts for a given function.
//
// Notice that dual metric visualization implies a double scale.
type Category struct {
	ID          string
	Title       string
	Environment string

	// XLabels is the ordered list of workload axis labels (one per (function, context) column).
	//
	// Every series in the category holds exactly one point per label, in that same order:
	// the chart aligns series data to the axis by index, so a series may never skip a column.
	// Columns without any measurement carry a point flagged [MetricPoint.Missing].
	XLabels []string

	Data []CategoryData
}

// Metrics returns the deduplicated list of metrics present in the category data.
func (c Category) Metrics() (metrics []config.Metric) {
	seenMetric := make(map[config.Metric]struct{})

	for _, data := range c.Data {
		_, seen := seenMetric[data.Metric]
		if seen {
			continue
		}

		seenMetric[data.Metric] = struct{}{}
		metrics = append(metrics, data.Metric)
	}

	return metrics
}

// TitleWithPlaceHolders replaces the "{metric}" placeholder in the title of the chart.
func (c Category) TitleWithPlaceHolders(metric config.Metric) string {
	return strings.ReplaceAll(c.Title, "{metric}", metric.Title)
}

// CategoryData holds the data series for one metric and one version.
//
// Each series represented by a [CategoryData] is represented as one single data series on the chart.
//
// Each point of the data series corresponds to a context for the measurement.
type CategoryData struct {
	Version config.Version
	Metric  config.Metric
	Series  []MetricSeries
}

// SeriesKey uniquely identify a benchmark series.
//
// The keys to identify a series are: function, version, context and metric.
type SeriesKey struct {
	Function string
	Version  string
	Context  string
	Metric   config.MetricName
}

// MetricSeries correspond to a single series composed of points.
//
// The Title is used to display in a legend and corresponds to the version.
type MetricSeries struct {
	SeriesKey

	Title  string
	Points []MetricPoint
}

// Labels returns the data point labels of the data series.
func (s MetricSeries) Labels() []string {
	labels := make([]string, 0, len(s.Points))

	for _, point := range s.Points {
		labels = append(labels, point.Name)
	}

	return labels
}

// MetricPoint is a single data point. Each data point has a label and a float64 value.
//
// The label is composed like "{function} - {context} - {version}" and may be used by tooltips
// when hovering over a data point.
//
// A point holding no measurement is flagged Missing: it still occupies its column so the
// series stays aligned with the workload axis, but renders as a gap rather than as a zero.
type MetricPoint struct {
	SeriesKey

	Name    string
	Label   string // x-axis label: context title (optionally prefixed by function title)
	Value   float64
	Missing bool // no measurement for this (function, context, version, metric)
}
