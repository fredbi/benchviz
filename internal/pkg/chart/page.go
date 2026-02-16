package chart

import (
	"io"

	"github.com/go-echarts/go-echarts/v2/components"
)

// Page represents a page containing multiple charts.
//
// A [Page] knows how to [Page.Render] as HTML.
//
// TODO: control page layout, e.g. 2x2, 4x3 etc.
type Page struct {
	Title  string
	Charts []*Chart
}

// NewPage creates a new page with the given title.
func NewPage(title string) *Page {
	return &Page{
		Title: title,
	}
}

// AddChart adds a chart to the page.
func (p *Page) AddChart(c *Chart) {
	p.Charts = append(p.Charts, c)
}

// Render writes the page HTML to the given writer.
func (p *Page) Render(w io.Writer) error {
	page := components.NewPage()
	page.SetLayout(components.PageFlexLayout)
	page.SetPageTitle(p.Title)

	for _, c := range p.Charts {
		page.AddCharts(c.Build())
	}

	return page.Render(w)
}
