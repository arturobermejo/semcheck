package exporteddoc

//go:noinline
func OnlyDirective() {}

//nolint:unused
func OnlyNolint() {}

// WithDirective has text besides the directive.
//
//go:noinline
func WithDirective() {} // want "exported-func-doc: matched"

//
func EmptyComment() {}

// Deprecated: use Documented.
func Deprecated() {} // want "exported-func-doc: matched"
