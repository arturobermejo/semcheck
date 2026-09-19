package semcheck

import "testing"

func TestMatcherRegistry(t *testing.T) {
	// Arguments that each matcher accepts.
	args := map[string][]string{
		"call":        {"log.*"},
		"func-prefix": {"Get"},
	}

	for name := range matcherRegistry {
		t.Run(name, func(t *testing.T) {
			m, err := MatchSpec{Matcher: name, Args: args[name]}.matcher()
			if err != nil {
				t.Fatal(err)
			}

			// Diagnostics and configuration must agree on what a matcher is called.
			if m.Name != name {
				t.Errorf("the matcher registered as %q calls itself %q", name, m.Name)
			}

			if len(m.Types) == 0 || m.Match == nil {
				t.Errorf("matcher %q is incomplete: %+v", name, m)
			}
		})
	}
}

func TestMatcherRegistryErrors(t *testing.T) {
	tests := []struct {
		name string
		spec MatchSpec
	}{
		{"unknown matcher", MatchSpec{Matcher: "http-handler"}},
		{"no matcher", MatchSpec{}},
		{"arguments for exported-func-doc", MatchSpec{"exported-func-doc", []string{"x"}}},
		{"arguments for test-func", MatchSpec{"test-func", []string{"x"}}},
		{"no arguments for call", MatchSpec{Matcher: "call"}},
		{"no arguments for func-prefix", MatchSpec{Matcher: "func-prefix"}},
		{"bad argument for call", MatchSpec{"call", []string{"log"}}},
		{"bad argument for func-prefix", MatchSpec{"func-prefix", []string{"get-user"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if m, err := tt.spec.matcher(); err == nil || m != nil {
				t.Errorf("matcher() = %v, %v; want nil and an error", m, err)
			}
		})
	}
}
