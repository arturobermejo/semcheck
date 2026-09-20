package semcheck

import (
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the semcheck analysis for command-line drivers. Those parse the
// flags after they are given the analyzer, so it cannot be built from a
// configuration: it finds and loads one the first time it runs. To build an
// analysis from a Config, use NewAnalyzer.
var Analyzer = newCommandAnalyzer()

func newCommandAnalyzer() *analysis.Analyzer {
	var (
		configPath string
		dryRun     bool
		stats      bool
	)

	load := sync.OnceValues(func() (*analysis.Analyzer, error) {
		cfg, err := loadConfigOrNearest(configPath)
		if err != nil {
			return nil, err
		}

		opts := options{honorNolint: true, dryRun: dryRun, stats: stats}

		// A dry run asks nobody: it needs no judge, and so no API key.
		if dryRun {
			return newAnalyzer(cfg, nil, opts)
		}

		judge, err := DefaultJudge()
		if err != nil {
			return nil, err
		}

		return newAnalyzer(cfg, judge, opts)
	})

	a := baseAnalyzer(func(pass *analysis.Pass) (any, error) {
		loaded, err := load()
		if err != nil {
			return nil, unprefixed{err}
		}

		return loaded.Run(pass)
	})

	a.Flags.BoolVar(&dryRun, "dry-run", false, "count the questions, and estimate their cost, instead of asking them")
	a.Flags.BoolVar(&stats, "stats", false, "tell how many decisions of every package came from the cache")
	a.Flags.StringVar(&configPath, "config", "", "rules `file` (default: "+ConfigFile+" in the current directory or the closest parent that has one)")

	return a
}

// unprefixed is an error of the package API on its way through a driver, which
// puts the name of the analyzer in front of it: "semcheck: semcheck: …".
type unprefixed struct{ error }

func (e unprefixed) Error() string { return strings.TrimPrefix(e.error.Error(), "semcheck: ") }

func (e unprefixed) Unwrap() error { return e.error }
