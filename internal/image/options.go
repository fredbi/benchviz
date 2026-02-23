package image //nolint:revive // it's okay for an internal package to use this name

import "time"

// Option to tune image rendering.
type Option func(*options)

type options struct {
	Height        int64
	Width         int64
	SleepDuration time.Duration
}

const (
	defaultHeight int64 = 1080
	defaultWidth  int64 = 1920
	defaultWait         = time.Second
)

func optionsWithDefaults(opts []Option) options {
	o := options{
		Height:        defaultHeight,
		Width:         defaultWidth,
		SleepDuration: defaultWait,
	}

	for _, apply := range opts {
		apply(&o)
	}

	return o
}

// WithHeight sets the height of the screenshot.
//
// Defaults to 1080.
func WithHeight(height int64) Option {
	return func(o *options) {
		if height <= 0 {
			return
		}

		o.Height = height
	}
}

// WithWidth sets the width of the screenshot.
//
// Defaults to 1920.
func WithWidth(width int64) Option {
	return func(o *options) {
		if width <= 0 {
			return
		}

		o.Width = width
	}
}

// WithSleep sets the time to wait for the chrome headless engine to render the HTML page.
//
// Defaults to 1s.
func WithSleep(sleep time.Duration) Option {
	return func(o *options) {
		if sleep == 0 {
			return
		}

		o.SleepDuration = sleep
	}
}
