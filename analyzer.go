package semcheck

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/token"
	"io"
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

func warn(msg string) { fmt.Fprintln(stderr, "semcheck: warning: "+msg) }

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

	// dryRun counts the questions instead of asking them: no judge is needed,
	// and nothing is found.
	dryRun bool

	// stats tells, for every package, how many decisions came from the cache.
	stats bool

	// report, if set, receives what dryRun and stats have to say instead of
	// the standard error.
	report func(string)

	record io.Writer
}

func newAnalyzer(cfg *Config, judge Judge, opts options) (*analysis.Analyzer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("semcheck: invalid configuration:\n%w", err)
	}

	if opts.warn == nil {
		opts.warn = warn
	}

	if opts.report == nil {
		opts.report = func(msg string) { fmt.Fprintln(stderr, "semcheck: "+msg) }
	}

	if judge == nil && !opts.dryRun {
		return nil, errors.New("semcheck: there is no judge")
	}

	c := &checker{cfg: cfg, judge: judge, opts: opts, totals: &tally{}, matchers: make([]*matcher, len(cfg.Rules))}

	if opts.record != nil {
		c.records = &recorder{w: opts.record}
	}

	for i, r := range cfg.Rules {
		// Validate has built it already: it cannot fail.
		c.matchers[i] = must(r.Match.matcher())
	}

	return baseAnalyzer(func(pass *analysis.Pass) (any, error) {
		return nil, c.run(pass)
	}), nil
}

// analyzerName is also what a //nolint directive calls semcheck.
const analyzerName = "semcheck"

func baseAnalyzer(run func(*analysis.Pass) (any, error)) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     analyzerName,
		Doc:      "checks rules written in natural language on the code that AST matchers select",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      run,
	}
}

// A checker has what the analysis of every package shares.
type checker struct {
	cfg      *Config
	matchers []*matcher // one for each rule of cfg
	judge    Judge
	opts     options
	totals   *tally
	records  *recorder // nil if nobody asked for them
}

// An inquiry is a question along with where it comes from.
type inquiry struct {
	rule Rule
	pos  token.Pos
}

type finding struct {
	inquiry
	confidence float64
}

// finding returns what d makes of the inquiry, if the judge is sure enough.
func (i inquiry) finding(d Decision) (finding, bool) {
	confidence := d.confidence(i.rule.ReportIf)

	return finding{i, confidence}, confidence >= i.rule.MinConfidence
}

func (c *checker) run(pass *analysis.Pass) error {
	inquiries, questions, err := c.inquire(pass)
	if err != nil {
		return err
	}

	if c.opts.dryRun {
		if estimate := c.totals.estimate(questions); estimate != "" {
			c.opts.report("dry run: " + pass.Pkg.Path() + ": " + estimate)
		}

		return c.record(pass, inquiries, questions, nil)
	}

	// One batch for the whole package: round trips are what a model costs.
	ctx, cancel := context.WithTimeout(context.Background(), judgeTimeout)
	defer cancel()

	decisions, err := consult(ctx, c.judge, questions)
	if err != nil {
		return c.unanswered(pass.Pkg.Path(), len(questions), len(questions), err)
	}

	if c.opts.stats && len(questions) > 0 {
		c.opts.report("stats: " + pass.Pkg.Path() + ": " + c.totals.record(questions, decisions, c.judge))
	}

	if err := c.record(pass, inquiries, questions, decisions); err != nil {
		return err
	}

	findings, failed, cause := findingsOf(inquiries, decisions)

	if failed > 0 {
		if err := c.unanswered(pass.Pkg.Path(), failed, len(questions), cause); err != nil {
			return err
		}
	}

	report(pass, findings)

	return nil
}

func (c *checker) record(pass *analysis.Pass, inquiries []inquiry, questions []Question, decisions []Decision) error {
	if c.records == nil {
		return nil
	}

	return c.records.add(pass, inquiries, questions, decisions)
}

// inquire returns what the rules ask about the package: the inquiries and
// their questions, one for one.
func (c *checker) inquire(pass *analysis.Pass) ([]inquiry, []Question, error) {
	var (
		inquiries []inquiry
		questions []Question
		tooLarge  []token.Pos
	)

	var silenced nolint
	if c.opts.honorNolint {
		silenced = newNolint(pass)
	}

	for i, r := range c.cfg.Rules {
		for _, m := range c.matchers[i].matches(pass) {
			if inTestFile(pass, m.Node) && !r.Tests && !c.matchers[i].ForTests {
				continue
			}

			// Before asking: a finding nobody will see is not worth a question.
			if silenced.covers(pass.Fset, m.Pos) {
				continue
			}

			text, err := fragment(pass, m, r.Context)
			if errors.Is(err, errFragmentTooLarge) {
				tooLarge = append(tooLarge, m.Pos)

				continue
			}

			if err != nil {
				return nil, nil, err
			}

			inquiries = append(inquiries, inquiry{r, m.Pos})
			questions = append(questions, Question{Rule: r.Name, Ask: r.Ask, Fragment: text, Types: typeNotes(pass, m)})
		}
	}

	if n := len(tooLarge); n > 0 {
		c.opts.warn(fmt.Sprintf("%s: %d fragments too large to ask about, the first at %s", pass.Pkg.Path(), n, pass.Fset.Position(tooLarge[0])))
	}

	return inquiries, questions, nil
}

// findingsOf returns the decisions that make a finding, how many questions
// have no decision, and why the first of those has none.
func findingsOf(inquiries []inquiry, decisions []Decision) (findings []finding, failed int, cause error) {
	for i, d := range decisions {
		if d.Err != nil {
			failed++
			cause = cmp.Or(cause, d.Err)

			continue
		}

		if f, ok := inquiries[i].finding(d); ok {
			findings = append(findings, f)
		}
	}

	return findings, failed, cause
}

func report(pass *analysis.Pass, findings []finding) {
	// Drivers print diagnostics in the order they are reported. By file name
	// first: drivers parse files in parallel, and which one gets the lower
	// positions changes from run to run.
	fileName := func(pos token.Pos) string { return pass.Fset.File(pos).Name() }

	slices.SortStableFunc(findings, func(a, b finding) int {
		return cmp.Or(
			cmp.Compare(fileName(a.pos), fileName(b.pos)),
			cmp.Compare(a.pos, b.pos),
			cmp.Compare(a.rule.Name, b.rule.Name),
		)
	})

	for _, f := range findings {
		pass.Report(analysis.Diagnostic{
			Pos:      f.pos,
			Category: f.rule.Name,
			Message:  fmt.Sprintf("%s: %s (%.2f)", f.rule.Name, f.rule.message(), f.confidence),
		})
	}
}

// unanswered decides what becomes of the questions the judge left without an
// answer: an error if the configuration says so, a warning otherwise. A model
// that is down or slow must not, by itself, turn every build red.
func (c *checker) unanswered(pkg string, failed, total int, cause error) error {
	summary := fmt.Sprintf("%d of %d questions were not answered", failed, total)
	if total == 1 {
		summary = "the only question was not answered"
	}

	if c.cfg.FailOnJudgeError {
		return fmt.Errorf("%s: %w", summary, cause)
	}

	c.opts.warn(fmt.Sprintf("%s: %s: %v%s", pkg, summary, cause, c.opts.staleHint))

	return nil
}
