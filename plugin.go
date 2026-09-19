package semcheck

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/golangci/plugin-module-register/register"
	"go.yaml.in/yaml/v3"
	"golang.org/x/tools/go/analysis"
)

// PluginName is the key users write under linters.settings.custom in their
// .golangci.yml.
const PluginName = "semcheck"

func init() {
	register.Plugin(PluginName, newPlugin)
}

// pluginSettings is the "settings" block of the linter in .golangci.yml.
//
// Rules written there, instead of in a file of their own, have an advantage:
// golangci-lint knows when they change, and forgets the results it has cached.
type pluginSettings struct {
	Config string `yaml:"config"`
	Rules  []Rule `yaml:"rules"`
}

type plugin struct {
	analyzer *analysis.Analyzer
}

func newPlugin(settings any) (register.LinterPlugin, error) {
	cfg, err := pluginConfig(settings)
	if err != nil {
		return nil, err
	}

	judge, err := DefaultJudge()
	if err != nil {
		return nil, err
	}

	analyzer, err := newAnalyzer(cfg, judge, false)
	if err != nil {
		return nil, err
	}

	return plugin{analyzer}, nil
}

func pluginConfig(settings any) (*Config, error) {
	// golangci-lint hands the settings over as maps and slices. Through YAML
	// they get the same strict decoding as a file.
	data, err := yaml.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("semcheck: settings: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var s pluginSettings
	if err = dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("semcheck: settings: %w", err)
	}

	switch {
	case s.Config != "" && len(s.Rules) > 0:
		return nil, errors.New("semcheck: settings: give either config or rules, not both")
	case len(s.Rules) > 0:
		for i := range s.Rules {
			s.Rules[i].setDefaults()
		}

		return &Config{Rules: s.Rules}, nil
	case s.Config != "":
		return LoadConfig(s.Config)
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("semcheck: %w", err)
	}

	path, err := FindConfig(dir)
	if err != nil {
		return nil, err
	}

	return LoadConfig(path)
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{p.analyzer}, nil
}

// With LoadModeSyntax golangci-lint leaves pass.TypesInfo nil, unlike every
// driver used by the tests: asking for too little would only fail for users.
func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
