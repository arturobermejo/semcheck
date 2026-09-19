package exporteddoc

type Public struct{}

// Value has a value receiver.
func (Public) Value() {} // want "exported-func-doc: matched"

// Pointer has a pointer receiver.
func (p *Public) Pointer() {} // want "exported-func-doc: matched"

// hidden is an unexported method of an exported type.
func (p *Public) hidden() {}

func (p *Public) NoDoc() {}

type private struct{}

// Close is exported, but nobody outside the package can name its receiver.
func (p *private) Close() error { return nil }

type Set[T comparable] struct{}

// Add has a generic receiver.
func (s *Set[T]) Add(T) {} // want "exported-func-doc: matched"

type Pair[K comparable, V any] struct{}

// Swap has a receiver with two type parameters.
func (p Pair[K, V]) Swap() {} // want "exported-func-doc: matched"

type hiddenSet[T comparable] struct{}

// Add belongs to an unexported generic type.
func (s *hiddenSet[T]) Add(T) {}
