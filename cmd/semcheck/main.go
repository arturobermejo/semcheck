// Command semcheck runs the semcheck analyzer on Go packages.
//
// Standalone:
//
//	semcheck ./...
//
// As a go vet tool:
//
//	go vet -vettool=$(which semcheck) ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/arturobermejo/semcheck"
)

func main() {
	singlechecker.Main(semcheck.Analyzer)
}
