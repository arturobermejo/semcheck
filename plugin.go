package semcheck

import (
	"errors"
	"fmt"

	"github.com/golangci/plugin-module-register/register"
	"go.yaml.in/yaml/v3"
	"golang.org/x/tools/go/analysis"
)

// PluginName is the key users write under linters.settings.custom in their
// .golangci.yml.
const PluginName = analyzerName

func init() {
	register.Plugin(PluginName, newPlugin)
}

// pluginSettings is the "settings" block of the linter in .golangci.yml.
//
// Rules written there, instead of in a file of their own, have an advantage:
// golangci-lint knows when they change, and forgets the results it has cached.
type pluginSettings struct {
	File string `yaml:"config"`

	// Stats is the -stats flag of the command.
	Stats bool `yaml:"stats"`

	Config `yaml:",inline"`
}

type plugin struct {
	analyzer *analysis.Analyzer
}

func newPlugin(settings any) (register.LinterPlugin, error) {
	s, err := readSettings(settings)
	if err != nil {
		return nil, err
	}

	cfg, err := s.config()
	if err != nil {
		return nil, err
	}

	judge, err := DefaultJudge()
	if err != nil {
		return nil, err
	}

	analyzer, err := newAnalyzer(cfg, judge, options{
		honorNolint: false,
		stats:       s.Stats,
		// Seen, not assumed: after a judge failure that is only a warning,
		// golangci-lint reports "0 issues" for that package, without asking
		// again, until its code changes.
		staleHint: ". golangci-lint will cache this as a package without findings: run \"golangci-lint cache clean\" once the judge is back, or set fail_on_judge_error",
	})
	if err != nil {
		return nil, err
	}

	return plugin{analyzer}, nil
}

func readSettings(settings any) (*pluginSettings, error) {
	// golangci-lint hands the settings over as maps and slices. Through YAML
	// they get the same strict decoding as a file.
	data, err := yaml.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("semcheck: settings: %w", err)
	}

	var s pluginSettings
	if err = decodeStrict(data, &s); err != nil {
		return nil, fmt.Errorf("semcheck: settings: %w", err)
	}

	return &s, nil
}

// config returns the rules written in the settings or, if there are none, the
// ones of the file they name or of the closest ConfigFile.
func (s *pluginSettings) config() (*Config, error) {
	switch inline := !s.isZero(); {
	case s.File != "" && inline:
		return nil, errors.New("semcheck: settings: give either config or the configuration itself, not both")
	case inline:
		s.setDefaults()

		return &s.Config, nil
	}

	return loadConfigOrNearest(s.File)
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{p.analyzer}, nil
}

// With LoadModeSyntax golangci-lint leaves pass.TypesInfo nil, unlike every
// driver used by the tests: asking for too little would only fail for users.
func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
