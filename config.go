package semcheck

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ConfigFile is the name of the file FindConfig looks for.
const ConfigFile = ".semcheck.yml"

// Defaults of the optional fields of a Rule.
const (
	DefaultReportIf      = AnswerYes
	DefaultMinConfidence = 0.9
	DefaultContext       = ContextFunction
)

// A Config is the content of a .semcheck.yml file.
type Config struct {
	Rules []Rule `yaml:"rules"`

	// FailOnJudgeError makes the analysis fail when the judge cannot answer.
	// By default the questions are left unanswered, with a warning: a model
	// that is down must not break every build.
	FailOnJudgeError bool `yaml:"fail_on_judge_error"`
}

// isZero reports whether nothing was configured. It has to know every field
// of Config.
func (c *Config) isZero() bool {
	return len(c.Rules) == 0 && !c.FailOnJudgeError
}

// A Rule asks a closed question about the nodes a matcher selects.
type Rule struct {
	Name  string    `yaml:"name"`
	Match MatchSpec `yaml:"match"`
	Ask   string    `yaml:"ask"`

	// ReportIf is the answer that makes a finding.
	ReportIf Answer `yaml:"report_if"`

	// MinConfidence is how sure the model must be of that answer, in (0, 1].
	MinConfidence float64 `yaml:"min_confidence"`

	Context Context `yaml:"context"`

	// Message is what a finding says. If empty, it is made from Ask.
	Message string `yaml:"message"`

	// Tests makes the rule look at _test.go files too. Matchers that are
	// about tests, such as test-func, always do.
	Tests bool `yaml:"tests"`
}

// message is what a finding of the rule says. The model gives a probability,
// not a text: without a Message, the best description is the question and the
// answer that was found.
func (r Rule) message() string {
	if r.Message != "" {
		return r.Message
	}

	return fmt.Sprintf("the answer to %q is %s", r.Ask, r.ReportIf)
}

// A MatchSpec names a matcher and gives its arguments, as written in
//
//	match: { call: ["log.*", "slog.*"] }
//	match: { exported-func-doc: {} }
//	match: exported-func-doc
type MatchSpec struct {
	Matcher string
	Args    []string
}

// An Answer is one side of a closed question.
type Answer string

// The two answers.
const (
	AnswerYes Answer = "yes"
	AnswerNo  Answer = "no"
)

// LoadConfig reads a configuration file. The Config it returns is valid.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("semcheck: %w", err)
	}

	cfg, err := parseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("semcheck: %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("semcheck: %s: invalid configuration:\n%w", path, err)
	}

	return cfg, nil
}

// FindConfig returns the path of the ConfigFile in dir or, failing that, in the
// closest of its parents.
func FindConfig(dir string) (string, error) {
	start := dir

	for {
		path := filepath.Join(dir, ConfigFile)

		switch _, err := os.Stat(path); {
		case err == nil:
			return path, nil
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("semcheck: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Without rules there are no findings, which looks like clean code.
			return "", fmt.Errorf("semcheck: no %s in %s or any of its parents", ConfigFile, start)
		}

		dir = parent
	}
}

// loadConfigOrNearest reads the file at path or, if there is no path, the
// ConfigFile closest to the current directory.
func loadConfigOrNearest(path string) (*Config, error) {
	if path == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("semcheck: %w", err)
		}

		if path, err = FindConfig(dir); err != nil {
			return nil, err
		}
	}

	return LoadConfig(path)
}

func parseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := decodeStrict(data, &cfg); err != nil {
		return nil, err
	}

	cfg.setDefaults()

	return &cfg, nil
}

// decodeStrict reads the one YAML document of data into v.
func decodeStrict(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	// A misspelled field must be an error: an ignored min_confidnce would
	// silently run the rule with the default.
	dec.KnownFields(true)

	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("the file is empty")
		}

		return err
	}

	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return errors.New("more than one YAML document")
	}

	return nil
}

// setDefaults runs after decoding, not in an UnmarshalYAML method: inside one,
// the decoder forgets KnownFields and unknown fields go unnoticed.
func (c *Config) setDefaults() {
	for i := range c.Rules {
		c.Rules[i].setDefaults()
	}
}

func (r *Rule) setDefaults() {
	if r.ReportIf == "" {
		r.ReportIf = DefaultReportIf
	}

	if r.MinConfidence == 0 {
		r.MinConfidence = DefaultMinConfidence
	}

	if r.Context == "" {
		r.Context = DefaultContext
	}
}

// UnmarshalYAML accepts the booleans of YAML 1.1 and 1.2 alike. What "yes"
// decodes to depends on the Go type it lands in, and tools that rewrite YAML
// turn it into "true".
func (a *Answer) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		switch strings.ToLower(node.Value) {
		case "yes", "true", "on":
			*a = AnswerYes

			return nil
		case "no", "false", "off":
			*a = AnswerNo

			return nil
		}
	}

	return fmt.Errorf("line %d: report_if must be yes or no", node.Line)
}

// UnmarshalYAML accepts the three ways to write a match.
func (spec *MatchSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Value != "" {
		*spec = MatchSpec{Matcher: node.Value}

		return nil
	}

	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return fmt.Errorf("line %d: match must name exactly one matcher, as in { call: [\"log.*\"] }", node.Line)
	}

	// A mapping keeps its keys and values one after the other.
	name, value := node.Content[0], node.Content[1]

	args, err := matcherArgs(value)
	if err != nil {
		return fmt.Errorf("line %d: matcher %s: %w", value.Line, name.Value, err)
	}

	*spec = MatchSpec{Matcher: name.Value, Args: args}

	return nil
}

// matcherArgs reads a list of strings. Nothing, written as {} or left empty,
// is an empty list.
func matcherArgs(value *yaml.Node) ([]string, error) {
	var args []string

	switch {
	case value.Kind == yaml.SequenceNode:
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, errors.New("its arguments must be strings")
			}

			args = append(args, item.Value)
		}
	case value.Kind == yaml.MappingNode && len(value.Content) == 0:
	case value.Kind == yaml.ScalarNode && value.Tag == "!!null":
	default:
		return nil, errors.New("its arguments must be a list of strings, or {} if it takes none")
	}

	return args, nil
}
