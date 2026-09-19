package semcheck

import (
	"encoding/hex"
	"testing"
)

const testJudge = "jev 1 jev-latest"

func TestCacheKeyIsStable(t *testing.T) {
	q := Question{
		Rule:     "no-pii-in-logs",
		Ask:      "Does this log include personal data?",
		Fragment: `log.Printf("%+v", u)`,
		Types:    []string{"u: *User{Email string}"},
	}

	// The keys of the decisions people have on disk. If this fails and the
	// change is meant, cacheKeyVersion has to change too.
	const want = "b74d80abb7590e3ffdf6e93038e89e20c4667b6250b41c1f3e30c28a77347f2c"

	key := cacheKeyOf(testJudge, q)
	if got := hex.EncodeToString(key[:]); got != want {
		t.Errorf("key = %s, want %s", got, want)
	}

	if cacheKeyOf(testJudge, q) != key {
		t.Error("two keys for the same question")
	}
}

func TestCacheKeyIgnoresTheRule(t *testing.T) {
	a := Question{Rule: "no-pii-in-logs", Ask: "Personal data?", Fragment: "log.Print(u)"}
	b := Question{Rule: "privacy", Ask: "Personal data?", Fragment: "log.Print(u)"}

	if cacheKeyOf(testJudge, a) != cacheKeyOf(testJudge, b) {
		t.Error("the name of the rule changes the key")
	}
}

func TestCacheKeyTellsQuestionsApart(t *testing.T) {
	type keyed struct {
		judge string
		q     Question
	}

	base := Question{Ask: "ask", Fragment: "code", Types: []string{"u: User", "o: Order"}}

	with := func(change func(*Question)) keyed {
		q := base
		q.Types = append([]string(nil), base.Types...)
		change(&q)

		return keyed{testJudge, q}
	}

	different := map[string]keyed{
		"base":               {testJudge, base},
		"another model":      {"jev 1 jev-1.14.0", base},
		"another prompt":     {"jev 2 jev-latest", base},
		"another ask":        with(func(q *Question) { q.Ask = "ask?" }),
		"another fragment":   with(func(q *Question) { q.Fragment = "code " }),
		"another note":       with(func(q *Question) { q.Types[0] = "u: Admin" }),
		"notes swapped":      with(func(q *Question) { q.Types[0], q.Types[1] = q.Types[1], q.Types[0] }),
		"one note less":      with(func(q *Question) { q.Types = q.Types[:1] }),
		"no notes":           with(func(q *Question) { q.Types = nil }),
		"notes joined":       with(func(q *Question) { q.Types = []string{"u: Usero: Order"} }),
		"notes split":        with(func(q *Question) { q.Types = []string{"u: ", "User", "o: Order"} }),
		"ask into fragment":  with(func(q *Question) { q.Ask, q.Fragment = "as", "kcode" }),
		"fragment into note": with(func(q *Question) { q.Fragment, q.Types[0] = "cod", "eu: User" }),
		"note as fragment":   with(func(q *Question) { q.Fragment, q.Types = "codeu: User", q.Types[1:] }),
		"judge into ask":     {testJudge[:len(testJudge)-1], with(func(q *Question) { q.Ask = "task" }).q},
		"lengths as text":    with(func(q *Question) { q.Ask, q.Fragment = "ask4:code", "" }),
		"empty ask":          with(func(q *Question) { q.Ask = "" }),
		"empty fragment":     with(func(q *Question) { q.Fragment = "" }),
		"an empty note":      with(func(q *Question) { q.Types = append(q.Types, "") }),
	}

	seen := map[cacheKey]string{}

	for name, k := range different {
		key := cacheKeyOf(k.judge, k.q)
		if other, ok := seen[key]; ok {
			t.Errorf("%q and %q have the same key", name, other)
		}

		seen[key] = name
	}
}

// Pairs that a careless way of putting the fields together would mix up.
func TestCacheKeyEncodingIsNotAmbiguous(t *testing.T) {
	pairs := map[string][2]Question{
		"a separator inside a field": {
			{Ask: "a\x00b", Fragment: "c"},
			{Ask: "a", Fragment: "b\x00c"},
		},
		"a length that reads on into the field": {
			{Ask: "10abcdefghij", Fragment: "x"},
			{Ask: "2", Fragment: "abcdefghij", Types: []string{"x"}},
		},
	}

	for name, pair := range pairs {
		if cacheKeyOf(testJudge, pair[0]) == cacheKeyOf(testJudge, pair[1]) {
			t.Errorf("%s: the same key for %+v and %+v", name, pair[0], pair[1])
		}
	}
}
