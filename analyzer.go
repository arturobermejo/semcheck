package semcheck

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/token"
	"os"
	"slices"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// stderr is the standard error as it was when the program started. While
// linters run, golangci-lint points os.Stderr at /dev/null to keep them quiet:
// a warning written there is lost, and "nothing could be checked" would look
// exactly like "nothing was found".
var stderr = os.Stderr

// judgeTimeout bounds the questions of one package. An analysis.Pass brings no
// context of its own: go/analysis was designed for analyzers that do not wait.
const judgeTimeout = 2 * time.Minute

// NewAnalyzer returns the analysis that enforces the rules of cfg, asking judge
// about the code each rule selects. It honors //nolint directives, and writes
// its warnings to the standard error.
func NewAnalyzer(cfg *Config, judge Judge) (*analysis.Analyzer, error) {
	return newAnalyzer(cfg, judge, options{honorNolint: true})
}

type options struct {
	// honorNolint is off in the plugin, which leaves //nolint to golangci-lint.
	// There, a directive counts as used only if it silences a finding that
	// exists: skipping the question would make nolintlint report every one of
	// them as unused.
	honorNolint bool

	// warn, if set, receives the warnings instead of the standard error.
	warn func(string)

	// staleHint is added to the warning about unanswered questions by drivers
	// that cache results: they will keep serving the empty one.
	staleHint string
}

func newAnalyzer(cfg *Config, judge Judge, opts options) (*analysis.Analyzer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("semcheck: invalid configuration:\n%w", err)
	}

	if opts.warn == nil {
		opts.warn = func(msg string) { fmt.Fprintln(stderr, "semcheck: warning: "+msg) }
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
			return nil, check(pass, cfg, matchers, judge, opts)
		},
	}, nil
}

// An inquiry is a question along with where it comes from.
type inquiry struct {
	rule Rule
	pos  token.Pos
}

func check(pass *analysis.Pass, cfg *Config, matchers []*Matcher, judge Judge, opts options) error {
	var (
		inquiries []inquiry
		questions []Question
		tooLarge  []token.Pos
	)

	var silenced nolint
	if opts.honorNolint {
		silenced = newNolint(pass)
	}

	for i, r := range cfg.Rules {
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
				tooLarge = append(tooLarge, match.Pos)

				continue
			}

			if err != nil {
				return err
			}

			inquiries = append(inquiries, inquiry{r, match.Pos})
			questions = append(questions, Question{Rule: r.Name, Ask: r.Ask, Fragment: text, Types: typeNotes(pass, match)})
		}
	}

	if n := len(tooLarge); n > 0 {
		opts.warn(fmt.Sprintf("%s: %d fragments too large to ask about, the first at %s", pass.Pkg.Path(), n, pass.Fset.Position(tooLarge[0])))
	}

	// One batch for the whole package: round trips are what a model costs.
	ctx, cancel := context.WithTimeout(context.Background(), judgeTimeout)
	defer cancel()

	decisions, err := consult(ctx, judge, questions)
	if err != nil {
		return unanswered(pass, cfg, opts, len(questions), len(questions), err)
	}

	type finding struct {
		inquiry
		confidence float64
	}

	var (
		findings []finding
		failed   int
		firstErr error
	)

	for i, d := range decisions {
		if d.Err != nil {
			failed++
			firstErr = cmp.Or(firstErr, d.Err)

			continue
		}

		if c := d.confidence(inquiries[i].rule.ReportIf); c >= inquiries[i].rule.MinConfidence {
			findings = append(findings, finding{inquiries[i], c})
		}
	}

	if failed > 0 {
		if err := unanswered(pass, cfg, opts, failed, len(questions), firstErr); err != nil {
			return err
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

// unanswered decides what becomes of the questions the judge left without an
// answer: an error if the configuration says so, a warning otherwise. A model
// that is down or slow must not, by itself, turn every build red.
func unanswered(pass *analysis.Pass, cfg *Config, opts options, failed, total int, cause error) error {
	summary := fmt.Sprintf("%d of %d questions were not answered", failed, total)
	if total == 1 {
		summary = "the only question was not answered"
	}

	if cfg.FailOnJudgeError {
		return fmt.Errorf("%s: %w", summary, cause)
	}

	opts.warn(fmt.Sprintf("%s: %s: %v%s", pass.Pkg.Path(), summary, cause, opts.staleHint))

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
