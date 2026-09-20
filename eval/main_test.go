package main

import (
	"strings"
	"testing"

	"github.com/arturobermejo/semcheck"
)

func TestSummarize(t *testing.T) {
	yes := func(f float64) *float64 { return &f }

	projects := map[string][]semcheck.Record{
		"b": {
			{Rule: "pii", Tokens: 1_000_000, Yes: yes(0.95), Finding: true},
			{Rule: "pii", Tokens: 500, Yes: yes(0.05)},
			{Rule: "doc", Tokens: 500, Error: "overloaded"},
		},
		"a": {
			{Rule: "doc", Tokens: 100, Yes: yes(0.5)},
			{Rule: "doc", Tokens: 100, Yes: yes(1)},
			{Rule: "doc", Tokens: 100, Yes: yes(0.1)},
		},
	}

	var out strings.Builder

	summarize(&out, projects, map[string]string{"a": "exit=0 seconds=3"})

	const want = `| Project | Questions | Findings | <0.1 | <0.5 | <0.8 | ≤1 | Errors | ~Tokens | ~Cost | Run |
|---|---|---|---|---|---|---|---|---|---|---|
| a | 3 | 0 | 0 | 1 | 1 | 1 | 0 | 300 | $0.000 | exit=0 seconds=3 |
| b | 3 | 1 | 1 | 0 | 0 | 1 | 1 | 1001000 | $0.042 |  |
| **all** | 6 | 1 | 1 | 1 | 1 | 2 | 1 | 1001300 | $0.042 | |

| Rule | Questions | Findings | <0.1 | <0.5 | <0.8 | ≤1 | Errors | ~Tokens | ~Cost |
|---|---|---|---|---|---|---|---|---|---|
| doc | 4 | 0 | 0 | 1 | 1 | 1 | 1 | 800 | $0.000 |
| pii | 2 | 1 | 1 | 0 | 0 | 1 | 0 | 1000500 | $0.042 |
`

	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
