package semcheck

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// PluginName is the key users write under linters.settings.custom in their
// .golangci.yml.
const PluginName = "semcheck"

func init() {
	register.Plugin(PluginName, newPlugin)
}

type plugin struct{}

func newPlugin(any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}

// With LoadModeSyntax golangci-lint leaves pass.TypesInfo nil, unlike every
// driver used by the tests: asking for too little would only fail for users.
func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
