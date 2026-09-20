package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/arturobermejo/semcheck"
)

func yes(f float64) *float64 { return &f }

func TestSample(t *testing.T) {
	var records []semcheck.Record

	// Thirty findings, and a question in each of the other strata.
	for i := range 30 {
		records = append(records, semcheck.Record{Rule: "pii", Pos: fmt.Sprintf("/home/me/tmp/eval/repos/caddy/a.go:%d:1", i+1), Yes: yes(0.9), Finding: true})
	}

	records = append(records,
		semcheck.Record{Rule: "pii", Pos: "/home/me/tmp/eval/repos/caddy/b.go:1:1", Yes: yes(0.7)},
		semcheck.Record{Rule: "pii", Pos: "/home/me/tmp/eval/repos/caddy/b.go:2:1", Yes: yes(0.3)},
		semcheck.Record{Rule: "pii", Pos: "/home/me/tmp/eval/repos/caddy/b.go:3:1", Yes: yes(0.01)},
		semcheck.Record{Rule: "pii", Pos: "/home/me/tmp/eval/repos/caddy/b.go:4:1", Error: "overloaded"},
	)

	items := sample(map[string][]semcheck.Record{"caddy": records})

	var got []string
	for _, it := range items[10:] {
		got = append(got, fmt.Sprintf("%s|%s|%d", it.ID, it.Stratum, it.StratumSize))
	}

	want := []string{"pii caddy/b.go:3:1|low|1", "pii caddy/b.go:2:1|middle|1", "pii caddy/b.go:1:1|near|1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after the findings: %q, want %q", got, want)
	}

	if len(items) != 13 || items[9].Stratum != "finding" || items[9].StratumSize != 30 {
		t.Errorf("%d items, want ten of the thirty findings and the other three", len(items))
	}

	if again := sample(map[string][]semcheck.Record{"caddy": records}); !reflect.DeepEqual(again, items) {
		t.Error("the sample changes from one time to the next")
	}
}

func TestAskLabels(t *testing.T) {
	items := []item{
		{ID: "pii a", Record: semcheck.Record{Rule: "pii", Ask: "Personal data?", Fragment: "log(u)", Types: []string{"u: User"}, Yes: yes(0.97)}},
		{ID: "pii b", Record: semcheck.Record{Rule: "pii", Ask: "Personal data?", Fragment: "log(n)", Yes: yes(0.02)}},
		{ID: "doc c", Record: semcheck.Record{Rule: "doc", Ask: "Contradiction?", Fragment: "func F() {}", Yes: yes(0.5)}},
	}

	var out, kept strings.Builder

	// "pii b" has its label, and "doc c" one that was not given by hand, which
	// does not count. An answer that is none is asked again; then the work is
	// left, with one question to go.
	labels := []label{{ID: "pii b", Answer: "no", By: "hand"}, {ID: "doc c", Answer: "no", By: "claude"}}

	if err := askLabels(strings.NewReader("maybe\nY\nq\n"), &out, &kept, items, labels); err != nil {
		t.Fatal(err)
	}

	if want := `{"id":"doc c","answer":"yes","by":"hand"}` + "\n"; kept.String() != want {
		t.Errorf("kept %q, want %q", kept.String(), want)
	}

	if shown := out.String(); !strings.Contains(shown, "1 of 2 ── doc c") || !strings.Contains(shown, "u: User") {
		t.Errorf("shown:\n%s", shown)
	}

	// What the model answered must not be seen by whoever labels.
	if shown := out.String(); strings.Contains(shown, "0.97") || strings.Contains(shown, "0.5") {
		t.Errorf("the answer of the model is shown:\n%s", shown)
	}
}

func TestMetrics(t *testing.T) {
	it := func(id, stratum string, size int, modelYes float64) item {
		return item{ID: id, Stratum: stratum, StratumSize: size, Record: semcheck.Record{Rule: "pii", Yes: yes(modelYes)}}
	}

	items := []item{
		it("f1", "finding", 20, 0.9), it("f2", "finding", 20, 0.9), it("f3", "finding", 20, 0.9), it("f4", "finding", 20, 0.9),
		it("n1", "near", 10, 0.6), it("n2", "near", 10, 0.6),
		it("l1", "low", 100, 0.02), it("l2", "low", 100, 0.02),
	}

	labels := []label{
		{ID: "f1", Answer: "yes"},
		{ID: "f2", Answer: "yes"},
		{ID: "f3", Answer: "yes"},
		{ID: "f4", Answer: "no"},
		{ID: "n1", Answer: "yes"},
		{ID: "n2", Answer: "no"},
		{ID: "l1", Answer: "no"},
		{ID: "l2", Answer: "unsure"},
		{ID: "f4", Answer: "unsure"},
		{ID: "f4", Answer: "no"}, // changed twice: the last one counts
	}

	var out strings.Builder

	metrics(&out, items, labels)

	// 3 of 4 findings are right: 15 of the 20. Half of the 10 near ones are
	// missed: 5. Recall is 15 of 20.
	const want = `| Rule | Findings | Labeled | Right | Precision | Missed (estimate) | Recall (estimate) |
|---|---|---|---|---|---|---|
| pii | 20 | 4 | 3 | 75% | 5 | 75% |

| Rule | Stratum | Questions | Labeled | Unsure | Model says yes | Labels say yes |
|---|---|---|---|---|---|---|
| pii | finding | 20 | 4 | 0 | 90% | 75% |
| pii | near | 10 | 2 | 0 | 60% | 50% |
| pii | low | 100 | 1 | 1 | 2% | 0% |
`

	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
