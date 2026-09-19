//nolint:semcheck
package nolintfile

import "log"

func f() {
	log.Println("T in a file with the directive above the package clause")
}
