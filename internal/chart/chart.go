package chart

import (
	"github.com/fredbi/benchviz/internal/model"
	"github.com/go-echarts/go-echarts/v2/charts"
	echartsopts "github.com/go-echarts/go-echarts/v2/opts"
)

const (
	defaultFontSize = 12
	xAxisLabelAngle = 30
	axisNameGap     = 32
)

// Series represents a named data series in a chart.
type Series struct {
	Name string
	Data []echartsopts.BarData
}

// Chart represents a benchmark bar chart.
type Chart struct {
	options

	Series []Series
}

// NewChart creates a new chart with the given title and y-axis label.
func NewChart(opts ...Option) *Chart {
	return &Chart{
		options: optionsWithDefaults(opts),
	}
}

// AddSeries adds a named data series to the chart.
func (c *Chart) AddSeries(series model.MetricSeries) {
	data := make([]echartsopts.BarData, 0, len(series.Points))
	for _, point := range series.Points {
		data = append(data, echartsopts.BarData{
			Name:  point.Label,
			Value: point.Value,
			/*
				Tooltip: &echartsopts.Tooltip{
					Show:    echartsopts.Bool(true),
					Trigger: "item",
				},
			*/
		})
	}
	c.Series = append(c.Series, Series{Name: series.Title, Data: data})
}

// Build creates the ECharts bar chart from the accumulated configuration.
func (c *Chart) Build() *charts.Bar {
	bar := charts.NewBar()

	// Title options
	titleOpts := echartsopts.Title{
		Title: c.Title,
	}
	if c.Subtitle != "" {
		titleOpts.Subtitle = c.Subtitle
		titleOpts.SubtitleStyle = &echartsopts.TextStyle{
			FontStyle: "italic",
			FontSize:  defaultFontSize,
		}
	}

	// Legend options
	legendOpts := echartsopts.Legend{
		Show: echartsopts.Bool(c.ShowLegend),
	}
	if c.ShowLegend {
		legendOpts.X, legendOpts.Y = legendXY(c.LegendPosition)
	}

	xAxisOpts, yAxisOpts := c.setAxes()

	// Grid options
	gridOpts := echartsopts.Grid{
		Bottom: "100",
		Top:    "100",
	}

	// Toolbox options
	toolboxOpts := echartsopts.Toolbox{
		Left: "right",
		Feature: &echartsopts.ToolBoxFeature{
			SaveAsImage: &echartsopts.ToolBoxFeatureSaveAsImage{
				Title: "Save as image",
			},
		},
	}

	// Apply global options
	bar.SetGlobalOptions(
		charts.WithInitializationOpts(echartsopts.Initialization{
			Theme:  c.Theme,
			Width:  c.Width,
			Height: c.Height,
		}),
		charts.WithToolboxOpts(toolboxOpts),
		charts.WithTitleOpts(titleOpts),
		charts.WithLegendOpts(legendOpts),
		charts.WithGridOpts(gridOpts),
		charts.WithXAxisOpts(xAxisOpts),
		charts.WithYAxisOpts(yAxisOpts),
		charts.WithTooltipOpts(echartsopts.Tooltip{
			Show:    echartsopts.Bool(true),
			Trigger: "axis",
			AxisPointer: &echartsopts.AxisPointer{
				Type: "shadow",
			},
		}),
	)

	// Set categories
	bar.SetXAxis(c.XAxisLabels)

	// Add all series
	for _, s := range c.Series {
		bar.AddSeries(s.Name, s.Data)
	}

	if c.Horizontal {
		return bar.XYReversal()
	}

	return bar
}

// legendXY maps a legend position string to echarts X and Y alignment values.
func legendXY(pos string) (string, string) {
	switch pos {
	case "top":
		return "center", "top"
	case "left":
		return "left", "center"
	case "right":
		return "right", "center"
	default: // "bottom" or any unknown value
		return "center", "bottom"
	}
}

// workloadAxisLabel builds the tick-label options for the workload (category) axis.
//
// The workload names are rotated to fit and, when configured, rendered at a smaller
// font size so long names don't overflow (notably on horizontal bar charts).
func (c *Chart) workloadAxisLabel() *echartsopts.AxisLabel {
	label := &echartsopts.AxisLabel{
		Rotate:       xAxisLabelAngle,
		Interval:     "0",
		ShowMinLabel: echartsopts.Bool(true),
		ShowMaxLabel: echartsopts.Bool(true),
		HideOverlap:  echartsopts.Bool(false),
	}

	if c.LabelFontSize > 0 {
		label.FontSize = c.LabelFontSize
	}

	return label
}

func (c *Chart) setAxes() (echartsopts.XAxis, echartsopts.YAxis) {
	const (
		workload     = "Workload"
		xType        = "category"
		yType        = "value"
		axisPosition = "bottom"
	)
	valueFormatter := echartsopts.FuncOpts("function (value,index) { return value.toFixed(0).toString();}")

	if !c.Horizontal {
		// X-axis options
		xAxisOpts := echartsopts.XAxis{
			Name:         workload,
			Type:         xType,
			Position:     axisPosition,
			NameLocation: "end",
			AxisTick: &echartsopts.AxisTick{
				AlignWithLabel: echartsopts.Bool(true),
			},
			AxisLabel: c.workloadAxisLabel(),
		}

		// Y-axis options
		yAxisOpts := echartsopts.YAxis{
			Name:  c.YAxisLabel,
			Type:  yType,
			Scale: echartsopts.Bool(true),
			AxisLabel: &echartsopts.AxisLabel{
				Formatter: valueFormatter,
			},
		}

		return xAxisOpts, yAxisOpts
	}

	// horizontal bar layout
	yAxisOpts := echartsopts.YAxis{
		Name:         workload,
		Type:         xType,
		Position:     axisPosition,
		NameLocation: "end",
		AxisLabel:    c.workloadAxisLabel(),
	}

	xAxisOpts := echartsopts.XAxis{
		Name:         c.YAxisLabel,
		NameLocation: "center",
		NameGap:      axisNameGap,
		Type:         yType,
		Scale:        echartsopts.Bool(true),
		AxisTick: &echartsopts.AxisTick{
			AlignWithLabel: echartsopts.Bool(true),
		},
		AxisLabel: &echartsopts.AxisLabel{
			Formatter: valueFormatter,
		},
	}

	return xAxisOpts, yAxisOpts
}
