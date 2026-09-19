package semcheck

import (
	"crypto/sha256"
	"fmt"
)

// A cacheKey identifies a decision: questions with the same key get the same
// answer, so that it has to be asked for only once.
type cacheKey [sha256.Size]byte

// cacheKeyVersion changes when the way of making keys does, which turns every
// decision stored before into one that is never found.
const cacheKeyVersion = "semcheck decision 1"

// cacheKeyOf hashes everything the answer to q depends on: who answers (judge
// names the model and how questions are put to it) and what it gets to read.
//
// The name of the rule is not part of it, since the model never sees it, and
// neither is what the rule makes of the answer: renaming a rule or moving its
// threshold costs no new questions.
func cacheKeyOf(judge string, q Question) cacheKey {
	h := sha256.New()

	// Every field goes with its length in front. Just one after the other,
	// "ab"+"c" and "a"+"bc" would be the same key for two different questions.
	for _, field := range append([]string{cacheKeyVersion, judge, q.Ask, q.Fragment}, q.Types...) {
		fmt.Fprintf(h, "%d:%s", len(field), field)
	}

	return cacheKey(h.Sum(nil))
}
