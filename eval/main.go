// Command eval reads what eval/run.sh collected.
//
//	go run ./eval           tables of what the model answered
//	go run ./eval sample    picks the questions to label by hand
//	go run ./eval label     asks for those labels, one question at a time
//	go run ./eval metrics   compares the labels with what semcheck reported
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

// readJSONLines reads a file with a JSON value of type T on each line.
func readJSONLines[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var (
		values []T
		dec    = json.NewDecoder(bufio.NewReader(file))
	)

	for {
		var v T

		switch err := dec.Decode(&v); err {
		case nil:
			values = append(values, v)
		case io.EOF:
			return values, nil
		default:
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
}

func writeJSONLines[T any](path string, values []T) error {
	var data []byte

	for _, v := range values {
		line, err := json.Marshal(v)
		if err != nil {
			return err
		}

		data = append(append(data, line...), '\n')
	}

	return os.WriteFile(path, data, 0o666)
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

const (
	resultsDir = "tmp/eval/results"
	samplePath = "tmp/eval/sample.jsonl"
	labelsPath = "eval/labels.jsonl"
)

// loadProjects reads what eval/run.sh left of every project.
func loadProjects() (projects map[string][]semcheck.Record, runs map[string]string, err error) {
	paths, _ := filepath.Glob(filepath.Join(resultsDir, "*", "records.jsonl"))
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("no results in %s: run eval/run.sh first", resultsDir)
	}

	projects, runs = map[string][]semcheck.Record{}, map[string]string{}

	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path))

		if projects[name], err = readJSONLines[semcheck.Record](path); err != nil {
			return nil, nil, err
		}

		run, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "run.txt"))
		runs[name] = strings.TrimSpace(string(run))
	}

	return projects, runs, nil
}

func run(command string) error {
	switch command {
	case "summary":
		projects, runs, err := loadProjects()
		if err != nil {
			return err
		}

		summarize(os.Stdout, projects, runs)

		return nil
	case "sample":
		projects, _, err := loadProjects()
		if err != nil {
			return err
		}

		items := sample(projects)

		if err := writeJSONLines(samplePath, items); err != nil {
			return err
		}

		fmt.Printf("%d questions to label in %s\n", len(items), samplePath)

		return nil
	case "label":
		return labelAll(os.Stdin, os.Stdout)
	case "metrics":
		items, err := readJSONLines[item](samplePath)
		if err != nil {
			return err
		}

		labels, err := readJSONLines[label](labelsPath)
		if err != nil {
			return err
		}

		metrics(os.Stdout, items, labels)

		return nil
	default:
		return fmt.Errorf("unknown command %q: the commands are summary, sample, label and metrics", command)
	}
}

func main() {
	command := "summary"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := run(command); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}
