// Package enclosing exercises Match.Func: the innermost function around a
// selected node.
package enclosing

import "log"

var _ = log.Prefix() // want "package level"

func declared() {
	log.Println("a") // want "in declared"

	func() {
		log.Println("b") // want "in the literal of line 12"
	}()

	go func() {
		defer func() {
			log.Println("c") // want "in the literal of line 17"
		}()
	}()
}

type T struct{}

func (T) method() {
	log.Println("d") // want "in method"
}

// Exported is selected as a whole: the function around it is itself.
func Exported() {} // want "in Exported"
