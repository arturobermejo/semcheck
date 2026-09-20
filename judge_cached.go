package semcheck

import (
	"context"
	"sync"
)

// A cacheableJudge says what its answers depend on besides the questions, such
// as the model behind it: decisions are only good for the judge that made them.
type cacheableJudge interface {
	Judge
	identity() string
}

// A cachedJudge asks its judge only what the cache cannot answer, and keeps
// what it learns. The same code gets the same decision run after run, which a
// model alone does not promise, and costs one question, not one per run.
type cachedJudge struct {
	judge cacheableJudge
	cache decisionCache

	// warn gets the first failure to keep a decision, and no other: a disk
	// that is full fails for every one of them.
	warn     func(string)
	warnOnce sync.Once
}

var _ Judge = (*cachedJudge)(nil)

func newCachedJudge(judge cacheableJudge, cache decisionCache) *cachedJudge {
	return &cachedJudge{judge: judge, cache: cache, warn: warn}
}

func (c *cachedJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	var (
		identity  = c.judge.identity()
		decisions = make([]Decision, len(questions))
		keys      = make([]cacheKey, len(questions))

		// What the cache does not have, and where in the batch it belongs.
		missing []Question
		places  []int
	)

	for i, q := range questions {
		keys[i] = cacheKeyOf(identity, q)

		if yes, ok := c.cache.get(keys[i]); ok {
			decisions[i].Yes = yes

			continue
		}

		missing = append(missing, q)
		places = append(places, i)
	}

	// Through consult: an answer that makes no sense must not be kept.
	answers, err := consult(ctx, c.judge, missing)

	for n, i := range places {
		if err != nil {
			// The decisions of the cache are still good.
			decisions[i].Err = err

			continue
		}

		decisions[i] = answers[n]

		if answers[n].Err != nil {
			continue
		}

		if putErr := c.cache.put(keys[i], answers[n].Yes); putErr != nil {
			c.warnOnce.Do(func() { c.warn("decisions are not being kept: " + putErr.Error()) })
		}
	}

	return decisions, nil
}
