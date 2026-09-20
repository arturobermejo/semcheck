package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strings"
)

// A label is what a person answers to the question of an item.
type label struct {
	ID     string `json:"id"`
	Answer string `json:"answer"` // yes, no or unsure

	// By tells a label given at the prompt of this program, "hand", from one
	// that comes from somewhere else, such as a language model.
	By  string `json:"by"`
	Why string `json:"why,omitempty"`
}

var answers = map[string]string{"y": "yes", "n": "no", "u": "unsure"}

// labelAll asks for the labels that are missing, and keeps each one as soon as
// it is given: the work can be left and taken up again.
func labelAll(in io.Reader, out io.Writer) error {
	items, err := readJSONLines[item](samplePath)
	if err != nil {
		return fmt.Errorf("%w: run \"go run ./eval sample\" first", err)
	}

	labels, err := readJSONLines[label](labelsPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	file, err := os.OpenFile(labelsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer file.Close()

	return askLabels(in, out, file, items, labels)
}

func askLabels(in io.Reader, out, keep io.Writer, items []item, labels []label) error {
	done := map[string]bool{}
	for _, l := range labels {
		done[l.ID] = true
	}

	// One rule after the other, to answer the same question many times in a
	// row. Shuffled within: the order must not tell what the model answered.
	rng := rand.New(rand.NewPCG(2, 2))
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })

	var pending []item

	for _, rule := range rulesOf(items) {
		for _, it := range items {
			if it.Rule == rule && !done[it.ID] {
				pending = append(pending, it)
			}
		}
	}

	lines := bufio.NewScanner(in)

	for i, it := range pending {
		fmt.Fprintf(out, "\n──── %d of %d ── %s\n\n%s\n", i+1, len(pending), it.ID, it.Fragment)

		if len(it.Types) > 0 {
			fmt.Fprintf(out, "\n%s\n", strings.Join(it.Types, "\n"))
		}

		for {
			fmt.Fprintf(out, "\n%s\n(y)es, (n)o, (u)nsure or (q)uit > ", it.Ask)

			if !lines.Scan() {
				return lines.Err()
			}

			key := strings.ToLower(strings.TrimSpace(lines.Text()))
			if key == "q" {
				return nil
			}

			if answer, ok := answers[key]; ok {
				line, _ := json.Marshal(label{ID: it.ID, Answer: answer, By: "hand"})
				if _, err := keep.Write(append(line, '\n')); err != nil {
					return err
				}

				break
			}
		}
	}

	fmt.Fprintln(out, "\nNothing left to label.")

	return nil
}
