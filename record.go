package semcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"golang.org/x/tools/go/analysis"
)

// A Record is a line of the file of the -record flag: a question, and what
// became of it. Findings tell what the model is sure of; the records tell what
// it said about everything else, which is what it takes to choose a
// min_confidence, or to see the code that leaves for the model.
type Record struct {
	Package  string   `json:"package"`
	Rule     string   `json:"rule"`
	Pos      string   `json:"pos"` // file:line:column
	Ask      string   `json:"ask"`
	Fragment string   `json:"fragment"`
	Types    []string `json:"types,omitempty"`

	// Yes is missing in a dry run, and when the judge gave no answer.
	Yes    *float64 `json:"yes,omitempty"`
	Cached bool     `json:"cached,omitempty"`
	Error  string   `json:"error,omitempty"`

	// Finding tells whether the question was reported.
	Finding bool `json:"finding"`
}

// A recorder writes records as JSON, one on each line.
type recorder struct {
	mu sync.Mutex
	w  io.Writer

	// seen has the rule and the position of what is written. Drivers analyze a
	// package and its variant with the tests, which asks the same again.
	seen map[string]bool
}

// add writes the questions of a package. There are no decisions in a dry run.
func (r *recorder) add(pass *analysis.Pass, inquiries []inquiry, questions []Question, decisions []Decision) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.seen == nil {
		r.seen = map[string]bool{}
	}

	for i, q := range questions {
		pos := pass.Fset.Position(inquiries[i].pos).String()

		key := q.Rule + "\x00" + pos
		if r.seen[key] {
			continue
		}

		r.seen[key] = true

		rec := Record{Package: pass.Pkg.Path(), Rule: q.Rule, Pos: pos, Ask: q.Ask, Fragment: q.Fragment, Types: q.Types}

		if decisions != nil {
			d := decisions[i]

			if d.Err != nil {
				rec.Error = d.Err.Error()
			} else {
				rec.Yes, rec.Cached = &d.Yes, d.Cached
				_, rec.Finding = inquiries[i].finding(d)
			}
		}

		line, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("record: %w", err)
		}

		// One Write for each line: processes that share the file, as the ones
		// of "go vet" do, do not cut the lines of one another.
		if _, err := r.w.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("record: %w", err)
		}
	}

	return nil
}
