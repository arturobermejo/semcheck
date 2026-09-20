package semcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func recordsIn(t *testing.T, data []byte) []Record {
	t.Helper()

	var records []Record

	for line := range strings.Lines(string(data)) {
		var r Record

		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&r); err != nil {
			t.Fatalf("%v in the line %q", err, line)
		}

		records = append(records, r)
	}

	return records
}

func ptr(f float64) *float64 { return &f }

func TestRecord(t *testing.T) {
	var file bytes.Buffer

	judge := &FakeJudge{Answer: func(q Question) float64 {
		if strings.Contains(q.Fragment, "%+v") {
			return 0.95
		}

		return 0.02
	}}

	a, _ := testAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge, options{record: &file})

	// Twice, as a driver does with a package and its variant with the tests.
	for range 2 {
		if n := findingsIn(t, a, checkedSource); n != 1 {
			t.Fatalf("%d findings, want 1", n)
		}
	}

	r := logRule()

	want := []Record{
		{
			Package: "p", Rule: r.Name, Pos: "p.go:11:2", Ask: r.Ask,
			Fragment: "logf(\"created %+v\", u)",
			Types:    []string{"u: User{ID int; Email string}"},
			Tokens:   44, Yes: ptr(0.95), Finding: true,
		},
		{
			Package: "p", Rule: r.Name, Pos: "p.go:12:2", Ask: r.Ask,
			Fragment: "logf(\"done\")",
			Tokens:   24, Yes: ptr(0.02),
		},
	}

	if got := recordsIn(t, file.Bytes()); !reflect.DeepEqual(got, want) {
		t.Errorf("records:\n got %s\nwant %s", describeRecords(got), describeRecords(want))
	}
}

func describeRecords(records []Record) string {
	data, _ := json.MarshalIndent(records, "", "  ")

	return string(data)
}

func TestRecordOfADryRun(t *testing.T) {
	var file bytes.Buffer

	a, _ := testAnalyzer(t, &Config{Rules: []Rule{logRule()}}, nil, options{dryRun: true, record: &file})

	findingsIn(t, a, checkedSource)

	records := recordsIn(t, file.Bytes())
	if len(records) != 2 {
		t.Fatalf("%d records, want 2", len(records))
	}

	for _, r := range records {
		if r.Yes != nil || r.Finding || r.Fragment == "" {
			t.Errorf("record = %+v, want a question without an answer", r)
		}
	}
}

func TestRecordOfAQuestionWithoutAnswer(t *testing.T) {
	var file bytes.Buffer

	judge := judgeFunc(func(questions []Question) ([]Decision, error) {
		return []Decision{{Yes: 0.95, Cached: true}, {Err: errors.New("the model refused")}}, nil
	})

	a, _ := testAnalyzer(t, &Config{Rules: []Rule{logRule()}}, judge, options{record: &file})

	findingsIn(t, a, checkedSource)

	records := recordsIn(t, file.Bytes())
	if len(records) != 2 {
		t.Fatalf("%d records, want 2", len(records))
	}

	if r := records[0]; r.Yes == nil || !r.Cached || !r.Finding || r.Error != "" {
		t.Errorf("record = %+v, want a cached finding", r)
	}

	if r := records[1]; r.Yes != nil || r.Finding || r.Error != "the model refused" {
		t.Errorf("record = %+v, want the error and no answer", r)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// Records that were asked for and cannot be written are not a warning: whoever
// reads the file later would take it for complete.
func TestRecordThatCannotBeWritten(t *testing.T) {
	a, _ := testAnalyzer(t, &Config{Rules: []Rule{logRule()}}, &FakeJudge{}, options{record: brokenWriter{}})

	if _, err := runOn(t, a, checkedSource); err == nil || !strings.Contains(err.Error(), "record: disk full") {
		t.Errorf("error = %v, want one about the record", err)
	}
}
