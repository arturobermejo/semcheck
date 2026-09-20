package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
)

func rulesOf(items []item) []string {
	var rules []string

	for _, it := range items {
		if !slices.Contains(rules, it.Rule) {
			rules = append(rules, it.Rule)
		}
	}

	slices.Sort(rules)

	return rules
}

// A tally is what the labels say of a stratum of a rule.
type tally struct {
	size     int     // questions in the stratum
	labeled  int     // of the ones picked, those with a yes or a no
	yes      int     // labeled yes
	unsure   int     // labeled unsure, and left out
	modelYes float64 // the sum of what the model answered for the labeled ones
}

// estimatedYes is how many questions of the stratum would be labeled yes,
// if the ones labeled are like the rest.
func (t tally) estimatedYes() float64 {
	if t.labeled == 0 {
		return 0
	}

	return float64(t.yes) / float64(t.labeled) * float64(t.size)
}

func ratio(a, b float64) string {
	if b == 0 {
		return "-"
	}

	return fmt.Sprintf("%.0f%%", 100*a/b)
}

// metrics writes, for every rule, how often its findings are right and how
// much of what it should find it finds, and then what the labels say of each
// stratum next to what the model said.
func metrics(w io.Writer, items []item, labels []label) {
	answer := map[string]string{}
	for _, l := range labels {
		answer[l.ID] = l.Answer // the last one wins
	}

	tallies := map[[2]string]*tally{}

	for _, it := range items {
		key := [2]string{it.Rule, it.Stratum}
		if tallies[key] == nil {
			tallies[key] = &tally{size: it.StratumSize}
		}

		t := tallies[key]

		switch answer[it.ID] {
		case "yes":
			t.yes++
			t.labeled++
			t.modelYes += *it.Yes
		case "no":
			t.labeled++
			t.modelYes += *it.Yes
		case "unsure":
			t.unsure++
		}
	}

	get := func(rule, stratum string) tally { return *cmp.Or(tallies[[2]string{rule, stratum}], &tally{}) }

	fmt.Fprintln(w, "| Rule | Findings | Labeled | Right | Precision | Missed (estimate) | Recall (estimate) |\n|---|---|---|---|---|---|---|")

	for _, rule := range rulesOf(items) {
		found := get(rule, "finding")

		missed := 0.0
		for _, stratum := range strata[1:] {
			missed += get(rule, stratum).estimatedYes()
		}

		fmt.Fprintf(w, "| %s | %d | %d | %d | %s | %.0f | %s |\n", rule, found.size, found.labeled, found.yes,
			ratio(float64(found.yes), float64(found.labeled)), missed, ratio(found.estimatedYes(), found.estimatedYes()+missed))
	}

	fmt.Fprintln(w, "\n| Rule | Stratum | Questions | Labeled | Unsure | Model says yes | Labels say yes |\n|---|---|---|---|---|---|---|")

	for _, rule := range rulesOf(items) {
		for _, stratum := range strata {
			t := get(rule, stratum)
			if t.size == 0 {
				continue
			}

			fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %s | %s |\n", rule, stratum, t.size, t.labeled, t.unsure,
				ratio(t.modelYes, float64(t.labeled)), ratio(float64(t.yes), float64(t.labeled)))
		}
	}
}
