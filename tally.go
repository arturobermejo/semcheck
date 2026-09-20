package semcheck

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// The price of Jev, in dollars for a million tokens of input. Its output is
// free.
const dollarsPerMillionTokens = 0.042

// A tally adds up the questions of the packages of a run. An analysis never
// learns that it has seen the last package, so there is no place for a summary:
// every package reports the totals so far instead, and the last line to show
// up has the ones of the run.
type tally struct {
	mu        sync.Mutex
	questions int
	tokens    int
	cached    int

	// seen has the questions of a dry run. The ones a package shares with its
	// test variant, which drivers analyze too, would be asked once.
	seen map[cacheKey]bool
}

// estimate describes what asking the questions of a package would take, or
// returns "" if they have all been counted before.
func (t *tally) estimate(questions []Question) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.seen == nil {
		t.seen = map[cacheKey]bool{}
	}

	var (
		count, tokens int
		rules         = map[string]int{}
	)

	for _, q := range questions {
		key := cacheKeyOf("", q)
		if t.seen[key] {
			continue
		}

		t.seen[key] = true
		count++
		tokens += estimateTokens(q)
		rules[q.Rule]++
	}

	if count == 0 {
		return ""
	}

	byRule := make([]string, 0, len(rules))
	for _, name := range slices.Sorted(maps.Keys(rules)) {
		byRule = append(byRule, fmt.Sprintf("%s %d", name, rules[name]))
	}

	t.questions += count
	t.tokens += tokens

	return fmt.Sprintf("%s (%s), ~%d tokens; so far %s, ~%d tokens, %s",
		plural(count, "question"), strings.Join(byRule, ", "), tokens,
		plural(t.questions, "question"), t.tokens, dollars(t.tokens))
}

// record adds the decisions of a package to the totals, and describes where
// they came from.
func (t *tally) record(decisions []Decision) string {
	cached := 0

	for _, d := range decisions {
		if d.Cached && d.Err == nil {
			cached++
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.questions += len(decisions)
	t.cached += cached

	return fmt.Sprintf("%s, %d from the cache; so far %s, %d from the cache",
		plural(len(decisions), "question"), cached, plural(t.questions, "question"), t.cached)
}

// estimateTokens is a rough guess: about four bytes of code or English for a
// token. It counts all that the model reads to answer q.
func estimateTokens(q Question) int {
	bytes := len(q.Ask) + len(q.Fragment)
	for _, note := range q.Types {
		bytes += len(note)
	}

	return (bytes + 3) / 4
}

func dollars(tokens int) string {
	cost := float64(tokens) / 1e6 * dollarsPerMillionTokens
	if cost < 0.01 {
		return "under $0.01"
	}

	return fmt.Sprintf("~$%.2f", cost)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", n, noun)
}
