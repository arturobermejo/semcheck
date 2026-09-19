// Package semcheck is a semantic linter for Go.
//
// Static analysis decides where to look and a decision model decides whether
// the code is right: deterministic AST matchers select exact nodes (an
// exported function with its doc comment, a log call, an error message sent
// to a client), and a judge answers a closed natural-language question about
// each one with a calibrated probability. Only findings above the rule's
// confidence threshold are reported.
//
// The model is used only where meaning lives in human text (names, strings,
// comments, messages). Whatever static analysis can already decide is left to
// deterministic linters.
package semcheck
