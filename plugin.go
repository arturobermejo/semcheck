package semcheck

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// PluginName is the name under which semcheck registers itself as a
// golangci-lint module plugin. It is the key users write under
// linters.settings.custom in their .golangci.yml.
const PluginName = "semcheck"

// init registers the plugin. golangci-lint never calls semcheck directly: a
// custom build imports this package for its side effects only, and looks the
// plugin up by name when the configuration enables it.
func init() {
	register.Plugin(PluginName, newPlugin)
}

// plugin adapts Analyzer to the contract golangci-lint expects.
type plugin struct{}

// newPlugin is the constructor golangci-lint calls with the "settings" block of
// the linter's configuration. There is nothing to configure yet.
func newPlugin(any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

// BuildAnalyzers returns the analyzers golangci-lint must run.
func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}

// GetLoadMode tells golangci-lint how much of each package to load. Syntax
// trees are enough today, but matchers will need type information, and a plugin
// that asks for too little fails inside golangci-lint only, where no test of
// this repository would notice.
func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
