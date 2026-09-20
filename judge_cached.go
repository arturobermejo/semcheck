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

	// asking has the questions that some call to Decide is asking right now.
	// Drivers analyze a package and, at the same time, its variant with the
	// tests, which has the same code: the same questions. Asked twice, they
	// cost twice, and may get two answers: a finding with 0.91 and the same
	// one with 0.92 are not duplicates to a driver.
	mu     sync.Mutex
	asking map[cacheKey]*pending
}

// A pending decision is ready once done is closed.
type pending struct {
	done     chan struct{}
	decision Decision
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
		mine    []*pending

		// What another call is asking already.
		theirs = map[int]*pending{}
	)

	for i, q := range questions {
		keys[i] = cacheKeyOf(identity, q)

		if yes, ok := c.cache.get(keys[i]); ok {
			decisions[i] = Decision{Yes: yes}

			continue
		}

		if p, first := c.claim(keys[i]); first {
			missing = append(missing, q)
			places = append(places, i)
			mine = append(mine, p)
		} else {
			theirs[i] = p
		}
	}

	// Through consult: an answer that makes no sense must not be kept.
	answers, err := consult(ctx, c.judge, missing)

	for n, i := range places {
		switch {
		case err != nil:
			// The decisions of the cache are still good.
			decisions[i].Err = err
		case answers[n].Err != nil:
			decisions[i] = answers[n]
		default:
			decisions[i] = answers[n]

			if putErr := c.cache.put(keys[i], answers[n].Yes); putErr != nil {
				c.warnOnce.Do(func() { c.warn("decisions are not being kept: " + putErr.Error()) })
			}
		}

		c.release(keys[i], mine[n], decisions[i])
	}

	// Last, with nothing of its own left to ask: two calls that wait for each
	// other both get there.
	for i, p := range theirs {
		select {
		case <-p.done:
			decisions[i] = p.decision
		case <-ctx.Done():
			decisions[i].Err = context.Cause(ctx)
		}
	}

	return decisions, nil
}

// claim returns the pending decision for key, and whether the caller is the
// first to want it, and so the one to ask.
func (c *cachedJudge) claim(key cacheKey) (*pending, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if p, ok := c.asking[key]; ok {
		return p, false
	}

	if c.asking == nil {
		c.asking = map[cacheKey]*pending{}
	}

	p := &pending{done: make(chan struct{})}
	c.asking[key] = p

	return p, true
}

func (c *cachedJudge) release(key cacheKey, p *pending, d Decision) {
	c.mu.Lock()
	delete(c.asking, key)
	c.mu.Unlock()

	p.decision = d
	close(p.done)
}
