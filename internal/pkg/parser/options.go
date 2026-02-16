package parser

type Option func(*options)

type options struct {
	isJSON bool
}

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
