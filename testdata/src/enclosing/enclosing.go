// Package enclosing exercises Match.Func and Match.Stmt: the innermost
// function and statement around a selected node.
package enclosing

import "log"

var _ = log.Prefix() // want "package level, no statement"

func declared() {
	log.Println("a") // want `in declared, \*ast.ExprStmt`

	func() {
		log.Println("b") // want `in the literal of line 12, \*ast.ExprStmt`
	}()

	go func() {
		defer func() {
			log.Println("c") // want `in the literal of line 17, \*ast.ExprStmt`
		}()
	}()
}

type T struct{}

func (T) method() {
	defer log.Println("d") // want `in method, \*ast.DeferStmt`

	prefix := log.Prefix() // want `in method, \*ast.AssignStmt`

	if log.Flags() != 0 { // want `in method, \*ast.IfStmt`
		_ = prefix
	}
}

// A literal is a boundary: the statement that holds it is not the statement of
// what is inside.
var handler = func() { log.Println("e") } // want `in the literal of line 37, \*ast.ExprStmt`

var flags = func() int { return 0 }() + log.Flags() // want "package level, no statement"

// Exported is selected as a whole: the function around it is itself.
func Exported() {} // want "in Exported, no statement"
