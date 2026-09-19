package semcheck

import (
	"errors"
	"fmt"
	"go/ast"
	"regexp"
	"strings"
)

// Rule names appear in diagnostics as "name: message".
var ruleName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// Validate reports everything that is wrong with the configuration, not only
// the first problem: fixing a file one error at a time is a waste of runs.
func (c *Config) Validate() error {
	if len(c.Rules) == 0 {
		// Zero rules means zero findings, which looks just like clean code.
		return errors.New("there are no rules")
	}

	var (
		problems []error
		seen     = map[string]bool{}
	)

	for i, r := range c.Rules {
		// The number tells apart two rules with the same name.
		id := fmt.Sprintf("rule #%d", i+1)
		if r.Name != "" {
			id += fmt.Sprintf(" %q", r.Name)
		}

		if seen[r.Name] && r.Name != "" {
			problems = append(problems, fmt.Errorf("%s: there is another rule with that name", id))
		}

		seen[r.Name] = true

		for _, err := range r.problems() {
			problems = append(problems, fmt.Errorf("%s: %w", id, err))
		}
	}

	return errors.Join(problems...)
}

func (r Rule) problems() []error {
	var problems []error

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	switch {
	case r.Name == "":
		add("name is required")
	case !ruleName.MatchString(r.Name):
		add("name must start with a letter and have only letters, digits, - and _")
	}

	if strings.TrimSpace(r.Ask) == "" {
		add("ask is required")
	}

	if r.ReportIf != AnswerYes && r.ReportIf != AnswerNo {
		add("report_if must be yes or no, not %q", r.ReportIf)
	}

	if r.MinConfidence <= 0 || r.MinConfidence > 1 {
		add("min_confidence must be greater than 0 and at most 1, not %v", r.MinConfidence)
	}

	if r.Context != ContextStatement && r.Context != ContextFunction {
		add("context must be statement or function, not %q", r.Context)
	}

	switch m, err := r.Match.matcher(); {
	case r.Match.Matcher == "":
		add("match is required")
	case err != nil:
		add("match: %w", err)
	case r.Context == ContextStatement && selectsFunctions(m):
		add("context: statement has no effect with the matcher %s, which selects whole functions", m.Name)
	}

	return problems
}

func selectsFunctions(m *Matcher) bool {
	for _, t := range m.Types {
		if _, ok := t.(*ast.FuncDecl); !ok {
			return false
		}
	}

	return true
}
