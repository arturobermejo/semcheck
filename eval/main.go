// Command eval reads what eval/run.sh collected and prints it as tables.
//
//	go run ./eval [directory of results]
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/arturobermejo/semcheck"
)

// The price of Jev, as in tally.go.
const dollarsPerMillionTokens = 0.042

// The upper ends of the ranges that the answers are counted in.
var ranges = []float64{0.1, 0.5, 0.8, 1}

// A count is what is known of a set of questions.
type count struct {
	questions, findings, errors, tokens int

	// answers has how many fall in each of the ranges.
	answers [4]int
}

func (c *count) add(r semcheck.Record) {
	c.questions++
	c.tokens += r.Tokens

	if r.Finding {
		c.findings++
	}

	if r.Error != "" {
		c.errors++
	}

	if r.Yes != nil {
		c.answers[slices.IndexFunc(ranges, func(end float64) bool { return *r.Yes < end || end == 1 })]++
	}
}

func (c *count) cells() string {
	return fmt.Sprintf("%d | %d | %d | %d | %d | %d | %d | %d | $%.3f",
		c.questions, c.findings, c.answers[0], c.answers[1], c.answers[2], c.answers[3], c.errors,
		c.tokens, float64(c.tokens)/1e6*dollarsPerMillionTokens)
}

// header returns the first two lines of a table: the columns of cells, after
// one with the given name and before any others.
func header(first string, others ...string) string {
	columns := append([]string{first, "Questions", "Findings", "<0.1", "<0.5", "<0.8", "≤1", "Errors", "~Tokens", "~Cost"}, others...)

	return "| " + strings.Join(columns, " | ") + " |\n|" + strings.Repeat("---|", len(columns))
}

func readRecords(path string) ([]semcheck.Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var (
		records []semcheck.Record
		dec     = json.NewDecoder(bufio.NewReader(file))
	)

	for {
		var r semcheck.Record

		switch err := dec.Decode(&r); err {
		case nil:
			records = append(records, r)
		case io.EOF:
			return records, nil
		default:
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
}

// summarize writes a table of the projects and another of the rules.
func summarize(w io.Writer, projects map[string][]semcheck.Record, runs map[string]string) {
	var (
		total  count
		byRule = map[string]*count{}
	)

	fmt.Fprintln(w, header("Project", "Run"))

	for _, name := range slices.Sorted(maps.Keys(projects)) {
		var c count

		for _, r := range projects[name] {
			c.add(r)
			total.add(r)

			if byRule[r.Rule] == nil {
				byRule[r.Rule] = &count{}
			}

			byRule[r.Rule].add(r)
		}

		fmt.Fprintf(w, "| %s | %s | %s |\n", name, c.cells(), runs[name])
	}

	fmt.Fprintf(w, "| **all** | %s | |\n\n", total.cells())
	fmt.Fprintln(w, header("Rule"))

	for _, rule := range slices.Sorted(maps.Keys(byRule)) {
		fmt.Fprintf(w, "| %s | %s |\n", rule, byRule[rule].cells())
	}
}

func main() {
	dir := "tmp/eval/results"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	paths, _ := filepath.Glob(filepath.Join(dir, "*", "records.jsonl"))
	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "eval: no results in %s: run eval/run.sh first\n", dir)
		os.Exit(1)
	}

	projects, runs := map[string][]semcheck.Record{}, map[string]string{}

	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path))

		records, err := readRecords(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "eval:", err)
			os.Exit(1)
		}

		run, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "run.txt"))

		projects[name], runs[name] = records, strings.TrimSpace(string(run))
	}

	summarize(os.Stdout, projects, runs)
}
