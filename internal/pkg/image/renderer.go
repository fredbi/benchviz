// Package image converts a HTML into a PNG screenshot.
package image

import (
	"context"
	"fmt"
	"io"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
)

// Renderer knows how to take a screenshot from a HTML input and writes it as PNG.
type Renderer struct {
	options
}

// New builds an image [Renderer] from HTML.
func New(opts ...Option) *Renderer {
	return &Renderer{
		options: optionsWithDefaults(opts),
	}
}

// Render a PNG image as a screenshot from a HTML input [io.Reader].
func (r *Renderer) Render(dest io.Writer, source io.Reader) error {
	screenshot, err := r.screenshot(source)
	if err != nil {
		return fmt.Errorf("taking screenshot: %w", err)
	}

	_, err = dest.Write(screenshot)
	if err != nil {
		return fmt.Errorf("writing screenshot: %w", err)
	}

	return nil
}

func (r *Renderer) screenshot(reader io.Reader) ([]byte, error) {
	ctx, cancel := chromedp.NewContext(
		context.Background(),
		// chromedp.WithDebugf(log.Printf),
		// chromedp.WithBrowserOption(opts ...chromedp.BrowserOption)
	)
	defer cancel()

	var screenshot []byte
	// capture entire browser viewport, returning png with quality=90
	// localURL := fmt.Sprintf(`file://./%s`, file)
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}
	const qualityPNG = 100 // 100 to force PNG

	err = chromedp.Run(ctx,
		chromedp.Emulate(device.Info{
			Height:    r.Height,
			Width:     r.Width,
			Landscape: true,
		}),
		chromedp.Navigate("data:text/html,"+string(content)),
		// chromedp.WaitVisible(`canvas`, chromedp.ByQueryAll),
		// chromedp.WaitReady(`script  _, opts ...chromedp.QueryOption),
		chromedp.Sleep(r.SleepDuration), // we need to wait some time to get the rendering done
		chromedp.FullScreenshot(&screenshot, qualityPNG),
	)
	if err != nil {
		return nil, err
	}

	return screenshot, nil
}
