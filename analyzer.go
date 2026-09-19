package semcheck

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/token"
	"slices"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// judgeTimeout bounds the questions of one package. An analysis.Pass brings no
// context of its own: go/analysis was designed for analyzers that do not wait.
const judgeTimeout = 2 * time.Minute

// NewAnalyzer returns the analysis that enforces the rules of cfg, asking judge
// about the code each rule selects. It honors //nolint directives.
func NewAnalyzer(cfg *Config, judge Judge) (*analysis.Analyzer, error) {
	return newAnalyzer(cfg, judge, true)
}

// newAnalyzer lets the plugin leave //nolint to golangci-lint. There, a
// directive counts as used only if it silences a finding that exists: skipping
// the question would make nolintlint report every one of them as unused.
func newAnalyzer(cfg *Config, judge Judge, honorNolint bool) (*analysis.Analyzer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("semcheck: invalid configuration:\n%w", err)
	}

	matchers := make([]*Matcher, len(cfg.Rules))

	for i, r := range cfg.Rules {
		// Validate has built it already: it cannot fail.
		matchers[i] = must(r.Match.matcher())
	}

	return &analysis.Analyzer{
		Name:     "semcheck",
		Doc:      "checks rules written in natural language on the code that AST matchers select",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			return nil, check(pass, cfg.Rules, matchers, judge, honorNolint)
		},
	}, nil
}

// An inquiry is a question along with where it comes from.
type inquiry struct {
	rule Rule
	pos  token.Pos
}

func check(pass *analysis.Pass, rules []Rule, matchers []*Matcher, judge Judge, honorNolint bool) error {
	var (
		inquiries []inquiry
		questions []Question
	)

	var silenced nolint
	if honorNolint {
		silenced = newNolint(pass)
	}

	for i, r := range rules {
		for _, match := range matchers[i].matches(pass) {
			if inTestFile(pass, match.Node) && !r.Tests && !matchers[i].ForTests {
				continue
			}

			// Before asking: a finding nobody will see is not worth a question.
			if silenced.covers(pass.Fset, match.Pos) {
				continue
			}

			text, err := fragment(pass, match, r.Context)
			if errors.Is(err, errFragmentTooLarge) {
				continue
			}

			if err != nil {
				return err
			}

			inquiries = append(inquiries, inquiry{r, match.Pos})
			questions = append(questions, Question{Rule: r.Name, Ask: r.Ask, Fragment: text, Types: typeNotes(pass, match)})
		}
	}

	// One batch for the whole package: round trips are what a model costs.
	ctx, cancel := context.WithTimeout(context.Background(), judgeTimeout)
	defer cancel()

	decisions, err := consult(ctx, judge, questions)
	if err != nil {
		return err
	}

	type finding struct {
		inquiry
		confidence float64
	}

	var findings []finding

	for i, d := range decisions {
		if c := d.confidence(inquiries[i].rule.ReportIf); c >= inquiries[i].rule.MinConfidence {
			findings = append(findings, finding{inquiries[i], c})
		}
	}

	// Drivers print diagnostics in the order they are reported.
	slices.SortStableFunc(findings, func(a, b finding) int {
		return cmp.Or(cmp.Compare(a.pos, b.pos), cmp.Compare(a.rule.Name, b.rule.Name))
	})

	for _, f := range findings {
		pass.Report(analysis.Diagnostic{
			Pos:      f.pos,
			Category: f.rule.Name,
			Message:  fmt.Sprintf("%s: %s (%.2f)", f.rule.Name, f.rule.message(), f.confidence),
		})
	}

	return nil
}

// confidence is how sure the judge is that the answer is a.
func (d Decision) confidence(a Answer) float64 {
	if a == AnswerNo {
		return 1 - d.Yes
	}

	return d.Yes
}

// message is what a finding of the rule says. The model gives a probability,
// not a text: without a Message, the best description is the question and the
// answer that was found.
func (r Rule) message() string {
	if r.Message != "" {
		return r.Message
	}

	return fmt.Sprintf("the answer to %q is %s", r.Ask, r.ReportIf)
}
