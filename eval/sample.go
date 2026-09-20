package main

import (
	"cmp"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/arturobermejo/semcheck"
)

// The strata that the questions of a rule are split in, by what the model
// answered, and how many of each are labeled. Findings get the most: they are
// what a user sees. The rest tell what semcheck misses.
var (
	strata     = []string{"finding", "near", "middle", "low"}
	sampleSize = map[string]int{
		"finding": 10,
		"near":    4, // not reported, but more likely yes than no
		"middle":  4, // from 0.1 to 0.5
		"low":     4,
	}
)

func stratumOf(r semcheck.Record) string {
	switch {
	case r.Finding:
		return "finding"
	case *r.Yes >= 0.5:
		return "near"
	case *r.Yes >= 0.1:
		return "middle"
	default:
		return "low"
	}
}

// An item is a question picked to be labeled.
type item struct {
	ID string `json:"id"` // rule, project and position

	semcheck.Record

	Stratum string `json:"stratum"`

	// StratumSize is how many questions of the rule are in the stratum: the
	// ones this item stands for, along with the others picked from it.
	StratumSize int `json:"stratum_size"`
}

// sample picks the same questions every time, so that the labels of one day
// are good for the sample of the next.
func sample(projects map[string][]semcheck.Record) []item {
	byStratum := map[[2]string][]item{}

	for _, name := range slices.Sorted(maps.Keys(projects)) {
		for _, r := range projects[name] {
			if r.Yes == nil {
				continue
			}

			// The path up to the project is the one of whoever ran it.
			_, pos, _ := strings.Cut(r.Pos, "/tmp/eval/repos/")

			key := [2]string{r.Rule, stratumOf(r)}
			byStratum[key] = append(byStratum[key], item{ID: r.Rule + " " + pos, Record: r, Stratum: key[1]})
		}
	}

	var items []item

	for _, key := range slices.SortedFunc(maps.Keys(byStratum), func(a, b [2]string) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) }) {
		all := byStratum[key]
		slices.SortFunc(all, func(a, b item) int { return cmp.Compare(a.ID, b.ID) })

		rng := rand.New(rand.NewPCG(1, 1))
		rng.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })

		for _, it := range all[:min(sampleSize[key[1]], len(all))] {
			it.StratumSize = len(all)
			items = append(items, it)
		}
	}

	return items
}
