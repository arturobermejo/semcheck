package semcheck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// RecordEnv makes TestExamples ask the model and keep its answers, instead of
// using the ones that were kept the last time. It needs APIKeyEnv.
const RecordEnv = "SEMCHECK_RECORD"

// recordedDir has the answers of the model to the questions about the
// examples, as a cache of decisions.
const recordedDir = "testdata/examples/decisions"

// TestExamples checks the rules of .semcheck.yml, the ones people start from,
// and the two more of eval/rules.yml, on code that says what they must report
// ("// want") and, by saying nothing, what they must not. The answers are real
// ones, recorded: the test needs no network, and fails if the rules or the
// examples change until the answers are recorded again, with "make examples".
func TestExamples(t *testing.T) {
	judge := examplesJudge(t)

	tests := []struct {
		config   string
		packages []string
	}{
		{ConfigFile, []string{"examples/pii", "examples/level", "examples/name"}},

		// The rules that are not good enough to ship keep their examples, for
		// the day their questions are written again.
		{"eval/rules.yml", []string{"examples/doc", "examples/testname"}},
	}

	for _, tt := range tests {
		t.Run(tt.config, func(t *testing.T) {
			cfg, err := LoadConfig(tt.config)
			if err != nil {
				t.Fatal(err)
			}

			log := &decisionLog{}

			a, err := newAnalyzer(cfg, &loggingJudge{judge, log}, options{
				honorNolint: true,
				warn:        func(msg string) { t.Log("warning: " + msg) },
			})
			if err != nil {
				t.Fatal(err)
			}

			analysistest.Run(t, analysistest.TestData(), a, tt.packages...)

			for _, fragment := range log.leaks {
				t.Errorf("a question shows its expected answer to the model:\n%s", fragment)
			}

			t.Log("\n" + log.String())
		})
	}
}

func examplesJudge(t *testing.T) Judge {
	t.Helper()

	if os.Getenv(RecordEnv) == "" {
		if _, err := os.Stat(recordedDir); os.IsNotExist(err) {
			t.Skipf("there are no recorded answers: record them with \"make examples\", which needs %s", APIKeyEnv)
		}

		// A copy: a cache tidies itself up, and these files are not its own.
		dir := t.TempDir()
		if err := os.CopyFS(dir, os.DirFS(recordedDir)); err != nil {
			t.Fatal(err)
		}

		return newCachedJudge(unrecorded{t}, openTestCache(t, dir))
	}

	key := os.Getenv(APIKeyEnv)
	if key == "" {
		t.Fatalf("%s needs %s", RecordEnv, APIKeyEnv)
	}

	// From scratch: no answers to questions that are gone.
	if err := os.RemoveAll(recordedDir); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Remove(filepath.Join(recordedDir, trimMarker)) })

	return newCachedJudge(&JevJudge{APIKey: key}, openTestCache(t, recordedDir))
}

// unrecorded stands for Jev when there is no asking it.
type unrecorded struct{ t *testing.T }

func (u unrecorded) identity() string { return (&JevJudge{}).identity() }

func (u unrecorded) Decide(_ context.Context, questions []Question) ([]Decision, error) {
	u.t.Errorf("%d questions have no recorded answer, the first one about:\n%s\nRecord them with \"make examples\", which needs %s.",
		len(questions), questions[0].Fragment, APIKeyEnv)

	return nil, fmt.Errorf("no recorded answer")
}

// A loggingJudge keeps what its judge answers, to show it.
type loggingJudge struct {
	judge Judge
	log   *decisionLog
}

func (l *loggingJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	for _, q := range questions {
		// The model must not get to read what it is expected to answer.
		if strings.Contains(q.Fragment, "// want") {
			l.log.leaks = append(l.log.leaks, q.Fragment)
		}
	}

	decisions, err := l.judge.Decide(ctx, questions)

	for i, d := range decisions {
		if d.Err == nil {
			l.log.add(questions[i], d.Yes)
		}
	}

	return decisions, err
}

type decisionLog struct {
	mu    sync.Mutex
	lines []string
	leaks []string
}

func (l *decisionLog) add(q Question, yes float64) {
	firstLine, _, _ := strings.Cut(strings.TrimLeft(q.Fragment, "/ "), "\n")

	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, fmt.Sprintf("%-22s %.2f  %s", q.Rule, yes, firstLine))
}

// String has a line for every decision: by rule, the surest yes first.
func (l *decisionLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	lines := slices.Clone(l.lines)
	slices.Sort(lines)

	// Within a rule, from yes to no.
	slices.SortStableFunc(lines, func(a, b string) int {
		if a[:22] != b[:22] {
			return 0
		}

		return strings.Compare(b[23:27], a[23:27])
	})

	return strings.Join(slices.Compact(lines), "\n")
}
