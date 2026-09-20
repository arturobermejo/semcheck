package semcheck

import (
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

// JudgeEnv replaces the judge of DefaultJudge, for trying semcheck out without
// a model: "fake:<probability>" gives that answer to every question, and
// "broken" fails every time.
const JudgeEnv = "SEMCHECK_JUDGE"

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

	a.Flags.BoolVar(&dryRun, "dry-run", false, "count the questions, and estimate their cost, instead of asking them")
	a.Flags.BoolVar(&stats, "stats", false, "tell how many decisions of every package came from the cache")
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

// DefaultJudge returns the judge of the drivers: Jev, with the key in
// APIKeyEnv and its decisions kept in the directory of CacheEnv, unless
// JudgeEnv says otherwise.
func DefaultJudge() (Judge, error) {
	value := os.Getenv(JudgeEnv)

	if p, ok := strings.CutPrefix(value, "fake:"); ok {
		yes, err := strconv.ParseFloat(p, 64)
		if err != nil || !validProbability(yes) {
			return nil, fmt.Errorf("semcheck: %s=%s: the probability must be a number from 0 to 1", JudgeEnv, value)
		}

		return &FakeJudge{Answer: func(Question) float64 { return yes }}, nil
	}

	if value == "broken" {
		return &FakeJudge{Err: errors.New("broken on purpose")}, nil
	}

	if value != "" {
		return nil, fmt.Errorf("semcheck: %s=%s: unknown judge", JudgeEnv, value)
	}

	if key := os.Getenv(APIKeyEnv); key != "" {
		return withCache(&JevJudge{APIKey: key}), nil
	}

	// Not a judge that fails when asked: that would be a warning, and a run
	// that cannot reach any model must not pass for a clean one.
	return nil, fmt.Errorf("semcheck: there is no API key: set %s, or %s=fake:0.95 to try semcheck out without a model", APIKeyEnv, JudgeEnv)
}

// withCache returns judge behind the cache of decisions of CacheEnv. A cache
// on disk that cannot be used is worth a warning, not a failure: the judge
// still works, with a cache that lasts for the run.
func withCache(judge cacheableJudge) Judge {
	dir, err := cacheDir()
	if err == nil && dir == "" {
		return newCachedJudge(judge, &memoryCache{})
	}

	var cache *diskCache
	if err == nil {
		cache, err = openDiskCache(dir)
	}

	if err != nil {
		warn("decisions will not be kept for the next run: " + err.Error())

		return newCachedJudge(judge, &memoryCache{})
	}

	return newCachedJudge(judge, cache)
}

// unprefixed is an error of the package API on its way through a driver, which
// puts the name of the analyzer in front of it: "semcheck: semcheck: …".
type unprefixed struct{ error }

func (e unprefixed) Error() string { return strings.TrimPrefix(e.error.Error(), "semcheck: ") }

func (e unprefixed) Unwrap() error { return e.error }
