package parser //nolint:revive // it's okay for an internal package to use this name

// Option configures a [BenchmarkParser].
type Option func(*options)

type options struct {
	isJSON bool
}

// WithParseJSON enables JSON input parsing instead of the default text format.
func WithParseJSON(enabled bool) Option {
	return func(o *options) {
		o.isJSON = enabled
	}
}

func optionsWithDefaults(opts []Option) options {
	var o options
	for _, apply := range opts {
		apply(&o)
	}

	return o
}
