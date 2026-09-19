// Package semcheck is a semantic linter for Go.
//
// Static analysis decides where to look and a decision model decides whether
// the code is right: deterministic AST matchers select exact nodes (an HTTP
// handler, a log call, a bare "return err"), and a judge answers a closed
// natural-language question about each one with a calibrated probability.
// Only findings above the rule's confidence threshold are reported.
package semcheck
