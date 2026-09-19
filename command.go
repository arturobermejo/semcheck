package semcheck

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// ConfigFile is the name of the file FindConfig looks for.
const ConfigFile = ".semcheck.yml"

// JudgeEnv selects the judge of DefaultJudge. The only value so far is
// "fake:<probability>", a judge that gives that answer to every question: for
// trying semcheck out without a model.
const JudgeEnv = "SEMCHECK_JUDGE"

// Analyzer is the semcheck analysis for command-line drivers. Those parse the
// flags after they are given the analyzer, so it cannot be built from a
// configuration: it finds and loads one the first time it runs. To build an
// analysis from a Config, use NewAnalyzer.
var Analyzer = newCommandAnalyzer()

func newCommandAnalyzer() *analysis.Analyzer {
	var configPath string

	load := sync.OnceValues(func() (*analysis.Analyzer, error) {
		path := configPath

		if path == "" {
			dir, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("semcheck: %w", err)
			}

			if path, err = FindConfig(dir); err != nil {
				return nil, err
			}
		}

		cfg, err := LoadConfig(path)
		if err != nil {
			return nil, err
		}

		judge, err := DefaultJudge()
		if err != nil {
			return nil, err
		}

		return NewAnalyzer(cfg, judge)
	})

	a := &analysis.Analyzer{
		Name:     "semcheck",
		Doc:      "checks rules written in natural language on the code that AST matchers select",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			loaded, err := load()
			if err != nil {
				return nil, unprefixed{err}
			}

			return loaded.Run(pass)
		},
	}

	a.Flags.StringVar(&configPath, "config", "", "rules `file` (default: "+ConfigFile+" in the current directory or the closest parent that has one)")

	return a
}

// FindConfig returns the path of the ConfigFile in dir or, failing that, in the
// closest of its parents.
func FindConfig(dir string) (string, error) {
	start := dir

	for {
		path := filepath.Join(dir, ConfigFile)

		switch _, err := os.Stat(path); {
		case err == nil:
			return path, nil
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("semcheck: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Without rules there are no findings, which looks like clean code.
			return "", fmt.Errorf("semcheck: no %s in %s or any of its parents", ConfigFile, start)
		}

		dir = parent
	}
}

// DefaultJudge returns the judge of the drivers.
func DefaultJudge() (Judge, error) {
	value := os.Getenv(JudgeEnv)

	if p, ok := strings.CutPrefix(value, "fake:"); ok {
		yes, err := strconv.ParseFloat(p, 64)
		if err != nil || yes < 0 || yes > 1 {
			return nil, fmt.Errorf("semcheck: %s=%s: the probability must be a number from 0 to 1", JudgeEnv, value)
		}

		return &FakeJudge{Answer: func(Question) float64 { return yes }}, nil
	}

	if value != "" {
		return nil, fmt.Errorf("semcheck: %s=%s: unknown judge", JudgeEnv, value)
	}

	return unavailableJudge{}, nil
}

// unavailableJudge stands where the model will be.
type unavailableJudge struct{}

func (unavailableJudge) Decide(context.Context, []Question) ([]Decision, error) {
	return nil, fmt.Errorf("there is no model yet: set %s=fake:0.95 to try semcheck out", JudgeEnv)
}

// unprefixed is an error of the package API on its way through a driver, which
// puts the name of the analyzer in front of it: "semcheck: semcheck: …".
type unprefixed struct{ error }

func (e unprefixed) Error() string { return strings.TrimPrefix(e.error.Error(), "semcheck: ") }

func (e unprefixed) Unwrap() error { return e.error }
