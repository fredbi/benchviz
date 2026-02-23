package chart

// Theme constants from go-echarts.
//
// TODO: add more supported themes.
const (
	ThemeRoma = "roma"
)

// Option configures a [Chart].
type Option func(*options)

type options struct {
	Title       string
	Subtitle    string
	XAxisLabels []string
	YAxisLabel  string
	Theme       string
	ShowLegend  bool
	Horizontal  bool
}

// WithTitle sets the chart title.
func WithTitle(title string) Option {
	return func(c *options) {
		c.Title = title
	}
}

// WithSubtitle sets the chart subtitle (typically environment info).
func WithSubtitle(subtitle string) Option {
	return func(c *options) {
		c.Subtitle = subtitle
	}
}

// WithTheme sets the color theme.
func WithTheme(theme string) Option {
	return func(c *options) {
		c.Theme = theme
	}
}

// WithLegend enables or disables the legend.
func WithLegend(show bool) Option {
	return func(c *options) {
		c.ShowLegend = show
	}
}

// WithYAxisLabel sets the Y-axis label text.
func WithYAxisLabel(ylabel string) Option {
	return func(c *options) {
		c.YAxisLabel = ylabel
	}
}

// WithXAxisLabels sets the X-axis data point labels.
func WithXAxisLabels(xlabels []string) Option {
	return func(c *options) {
		c.XAxisLabels = xlabels
	}
}

// WithHorizontal enables or disables horizontal bar orientation.
func WithHorizontal(enabled bool) Option {
	return func(c *options) {
		c.Horizontal = enabled
	}
}

func optionsWithDefaults(opts []Option) options {
	o := options{
		Theme:      ThemeRoma,
		ShowLegend: true,
	}

	for _, apply := range opts {
		apply(&o)
	}

	return o
}
