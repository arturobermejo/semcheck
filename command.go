package semcheck

import (
	"fmt"
	"os"
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
		recordPath string
	)

	load := sync.OnceValues(func() (*analysis.Analyzer, error) {
		cfg, err := loadConfigOrNearest(configPath)
		if err != nil {
			return nil, err
		}

		opts := options{honorNolint: true, dryRun: dryRun, stats: stats}

		if recordPath != "" {
			// Never closed: it is written to until the program ends. Appended
			// to, because "go vet" runs a program for each package.
			if opts.record, err = os.OpenFile(recordPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666); err != nil {
				return nil, fmt.Errorf("semcheck: record: %w", err)
			}
		}

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
	a.Flags.StringVar(&recordPath, "record", "", "add to `file` every question, as a line of JSON, with its answer")
	a.Flags.BoolVar(&stats, "stats", false, "tell how many decisions of every package came from the cache")
	a.Flags.StringVar(&configPath, "config", "", "rules `file` (default: "+ConfigFile+" in the current directory or the closest parent that has one)")

	return a
}

// unprefixed is an error of the package API on its way through a driver, which
// puts the name of the analyzer in front of it: "semcheck: semcheck: …".
type unprefixed struct{ error }

func (e unprefixed) Error() string { return strings.TrimPrefix(e.error.Error(), "semcheck: ") }

func (e unprefixed) Unwrap() error { return e.error }
