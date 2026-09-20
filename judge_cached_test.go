package semcheck

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeCache is a decisionCache for tests, which can fail to keep.
type fakeCache struct {
	mu      sync.Mutex
	yes     map[cacheKey]float64
	putErr  error
	putKeys int
}

func (m *fakeCache) get(key cacheKey) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	yes, ok := m.yes[key]

	return yes, ok
}

func (m *fakeCache) put(key cacheKey, yes float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.putKeys++

	if m.putErr != nil {
		return m.putErr
	}

	if m.yes == nil {
		m.yes = map[cacheKey]float64{}
	}

	m.yes[key] = yes

	return nil
}

// identifiedFake is a FakeJudge whose decisions can be cached.
type identifiedFake struct {
	*FakeJudge

	id string
}

func (f identifiedFake) identity() string { return f.id }

// byFragment answers 0.1 for "a", 0.2 for "b"...
func byFragment(q Question) float64 {
	return float64(q.Fragment[0]-'a'+1) / 10
}

func asked(f *FakeJudge) string {
	var fragments []string
	for _, q := range f.Questions() {
		fragments = append(fragments, q.Fragment)
	}

	return strings.Join(fragments, "")
}

func answersOf(decisions []Decision) []float64 {
	var yes []float64
	for _, d := range decisions {
		yes = append(yes, d.Yes)
	}

	return yes
}

func TestCachedJudge(t *testing.T) {
	fake := &FakeJudge{Answer: byFragment}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, &fakeCache{})

	decisions, err := judge.Decide(context.Background(), questions("a", "b", "c"))
	if err != nil || !slices.Equal(answersOf(decisions), []float64{0.1, 0.2, 0.3}) {
		t.Fatalf("Decide = %+v, %v", decisions, err)
	}

	// Only what is new gets asked, and every decision lands in its place.
	decisions, err = judge.Decide(context.Background(), questions("d", "a", "e", "c", "b"))
	if err != nil || !slices.Equal(answersOf(decisions), []float64{0.4, 0.1, 0.5, 0.3, 0.2}) {
		t.Fatalf("Decide = %+v, %v", decisions, err)
	}

	if got := asked(fake); got != "abcde" {
		t.Errorf("the judge was asked %q, want every question once: abcde", got)
	}

	if n := len(fake.Batches()); n != 2 {
		t.Errorf("%d batches, want 2: the new questions of a batch go together", n)
	}

	// Nothing new: the judge is not called at all.
	if _, err = judge.Decide(context.Background(), questions("e", "a")); err != nil || len(fake.Batches()) != 2 {
		t.Errorf("err = %v after %d batches, want none for decisions that are all known", err, len(fake.Batches()))
	}
}

// The model answers 0.91 now and 0.92 later. The cache answers the same forever.
func TestCachedJudgeIsReproducible(t *testing.T) {
	calls := 0.0
	fake := &FakeJudge{Answer: func(Question) float64 { calls++; return 0.9 + calls/100 }}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, &fakeCache{})

	for range 3 {
		decisions, err := judge.Decide(context.Background(), questions("a"))
		if err != nil || decisions[0].Yes != 0.91 {
			t.Fatalf("Decide = %+v, %v; want the first answer, 0.91", decisions, err)
		}
	}
}

func TestCachedJudgeKeysOnTheIdentity(t *testing.T) {
	cache := &fakeCache{}
	old := &FakeJudge{Answer: func(Question) float64 { return 0.2 }}
	latest := &FakeJudge{Answer: func(Question) float64 { return 0.8 }}

	for _, tt := range []struct {
		judge identifiedFake
		want  float64
	}{
		{identifiedFake{old, "model 1"}, 0.2},
		{identifiedFake{latest, "model 2"}, 0.8},
		{identifiedFake{latest, "model 1"}, 0.2}, // from the cache
	} {
		decisions, err := newCachedJudge(tt.judge, cache).Decide(context.Background(), questions("a"))
		if err != nil || decisions[0].Yes != tt.want {
			t.Errorf("%s: Decide = %+v, %v; want %v", tt.judge.id, decisions, err, tt.want)
		}
	}
}

func TestCachedJudgeWhenTheJudgeFails(t *testing.T) {
	cache := &fakeCache{}
	fake := &FakeJudge{Answer: byFragment}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, cache)

	if _, err := judge.Decide(context.Background(), questions("a")); err != nil {
		t.Fatal(err)
	}

	fake.Err = errors.New("no network")

	decisions, err := judge.Decide(context.Background(), questions("a", "b"))
	if err != nil || len(decisions) != 2 {
		t.Fatalf("Decide = %+v, %v; want two decisions", decisions, err)
	}

	if decisions[0].Yes != 0.1 || decisions[0].Err != nil {
		t.Errorf("decision 0 = %+v, want the one of the cache", decisions[0])
	}

	if decisions[1].Err == nil || !errors.Is(decisions[1].Err, fake.Err) {
		t.Errorf("decision 1 = %+v, want the failure of the judge", decisions[1])
	}

	// A failure is not a decision: once the judge is back, it gets asked.
	fake.Err = nil

	decisions, err = judge.Decide(context.Background(), questions("b"))
	if err != nil || decisions[0].Yes != 0.2 || decisions[0].Err != nil {
		t.Errorf("Decide = %+v, %v; want 0.2", decisions, err)
	}
}

// partialJudge leaves the questions about "b" without an answer.
type partialJudge struct{ identifiedFake }

func (p partialJudge) Decide(ctx context.Context, questions []Question) ([]Decision, error) {
	decisions, err := p.identifiedFake.Decide(ctx, questions)

	for i, q := range questions {
		if q.Fragment == "b" {
			decisions[i] = Decision{Err: errors.New("overloaded")}
		}
	}

	return decisions, err
}

func TestCachedJudgeKeepsOnlyAnswers(t *testing.T) {
	cache := &fakeCache{}
	fake := &FakeJudge{Answer: byFragment}
	judge := newCachedJudge(partialJudge{identifiedFake{fake, "fake 1"}}, cache)

	decisions, err := judge.Decide(context.Background(), questions("a", "b", "c"))
	if err != nil || decisions[0].Yes != 0.1 || decisions[1].Err == nil || decisions[2].Yes != 0.3 {
		t.Fatalf("Decide = %+v, %v", decisions, err)
	}

	if len(cache.yes) != 2 {
		t.Errorf("%d decisions in the cache, want the 2 that were answered", len(cache.yes))
	}
}

func TestCachedJudgeDoesNotKeepNonsense(t *testing.T) {
	cache := &fakeCache{}
	fake := &FakeJudge{Answer: func(Question) float64 { return 1.5 }}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, cache)

	decisions, err := judge.Decide(context.Background(), questions("a"))
	if err != nil || decisions[0].Err == nil {
		t.Errorf("Decide = %+v, %v; want a decision with an error", decisions, err)
	}

	if cache.putKeys != 0 {
		t.Error("a probability of 1.5 was kept")
	}
}

func TestCachedJudgeWarnsOnceWhenItCannotKeep(t *testing.T) {
	cache := &fakeCache{putErr: errors.New("disk full")}
	judge := newCachedJudge(identifiedFake{&FakeJudge{Answer: byFragment}, "fake 1"}, cache)

	var warnings []string

	judge.warn = func(msg string) { warnings = append(warnings, msg) }

	for range 2 {
		decisions, err := judge.Decide(context.Background(), questions("a", "b"))
		if err != nil || !slices.Equal(answersOf(decisions), []float64{0.1, 0.2}) {
			t.Fatalf("Decide = %+v, %v; want the answers all the same", decisions, err)
		}
	}

	if len(warnings) != 1 || !strings.Contains(warnings[0], "disk full") {
		t.Errorf("warnings = %q, want one about the disk", warnings)
	}
}

// blockingJudge answers with the number of the question, 0.01 for the first
// one it ever got, and only once it is released.
type blockingJudge struct {
	entered chan struct{} // gets a value for every call to Decide
	release chan struct{} // closed to let every call go on
	err     error

	mu    sync.Mutex
	asked []string
}

func newBlockingJudge() *blockingJudge {
	return &blockingJudge{entered: make(chan struct{}, 10), release: make(chan struct{})}
}

func (b *blockingJudge) identity() string { return "blocking 1" }

func (b *blockingJudge) Decide(_ context.Context, questions []Question) ([]Decision, error) {
	b.mu.Lock()

	decisions := make([]Decision, len(questions))

	for i, q := range questions {
		b.asked = append(b.asked, q.Fragment)
		decisions[i].Yes = float64(len(b.asked)) / 100
	}

	b.mu.Unlock()

	b.entered <- struct{}{}

	<-b.release

	return decisions, b.err
}

// Drivers analyze a package and its variant with the tests at once: the same
// questions, from two calls.
func TestCachedJudgeAsksOnceForCallsAtTheSameTime(t *testing.T) {
	inner := newBlockingJudge()
	judge := newCachedJudge(inner, &fakeCache{})

	var (
		wg            sync.WaitGroup
		first, second []Decision
	)

	wg.Go(func() { first, _ = judge.Decide(context.Background(), questions("a", "b")) })

	<-inner.entered // the first call is waiting for its answers

	wg.Go(func() { second, _ = judge.Decide(context.Background(), questions("b", "z", "a")) })

	<-inner.entered // the second one has seen that only z is left to ask

	close(inner.release)
	wg.Wait()

	if got := strings.Join(inner.asked, ""); got != "abz" {
		t.Errorf("the judge was asked %q, want every question once: abz", got)
	}

	if !slices.Equal(answersOf(first), []float64{0.01, 0.02}) || !slices.Equal(answersOf(second), []float64{0.02, 0.03, 0.01}) {
		t.Errorf("decisions = %v and %v, want the same ones for a and b", answersOf(first), answersOf(second))
	}
}

func TestCachedJudgeSharesAFailure(t *testing.T) {
	inner := newBlockingJudge()
	inner.err = errors.New("no network")

	cache := &fakeCache{}
	judge := newCachedJudge(inner, cache)

	var (
		wg            sync.WaitGroup
		first, second []Decision
	)

	wg.Go(func() { first, _ = judge.Decide(context.Background(), questions("a")) })

	<-inner.entered

	wg.Go(func() { second, _ = judge.Decide(context.Background(), questions("z", "a")) })

	<-inner.entered

	close(inner.release)
	wg.Wait()

	for _, d := range slices.Concat(first, second) {
		if !errors.Is(d.Err, inner.err) {
			t.Errorf("decision = %+v, want the failure of the judge", d)
		}
	}

	// Nobody is left waiting for it: the next call asks again.
	inner.err = nil

	if decisions, _ := judge.Decide(context.Background(), questions("a")); decisions[0].Err != nil {
		t.Errorf("decision = %+v, want an answer", decisions[0])
	}
}

func TestCachedJudgeStopsWaitingWhenCanceled(t *testing.T) {
	inner := newBlockingJudge()
	judge := newCachedJudge(inner, &fakeCache{})

	var wg sync.WaitGroup

	wg.Go(func() { _, _ = judge.Decide(context.Background(), questions("a")) })

	<-inner.entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decisions, err := judge.Decide(ctx, questions("a"))
	if err != nil || !errors.Is(decisions[0].Err, context.Canceled) {
		t.Errorf("Decide = %+v, %v; want a decision that was canceled", decisions, err)
	}

	close(inner.release)
	wg.Wait()
}

func TestCachedJudgeAsksOnceForTheSameQuestionInABatch(t *testing.T) {
	fake := &FakeJudge{Answer: byFragment}
	judge := newCachedJudge(identifiedFake{fake, "fake 1"}, &fakeCache{})

	decisions, err := judge.Decide(context.Background(), questions("a", "b", "a"))
	if err != nil || !slices.Equal(answersOf(decisions), []float64{0.1, 0.2, 0.1}) {
		t.Fatalf("Decide = %+v, %v", decisions, err)
	}

	if got := asked(fake); got != "ab" {
		t.Errorf("the judge was asked %q, want ab", got)
	}
}

// The whole thing: the second run of an analysis asks nothing and reports the
// same, from a cache on disk.
func TestAnalysisIsReproducibleFromTheCache(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Rules: []Rule{logRule()}}

	run := func(answer float64) ([]string, int) {
		fake := &FakeJudge{Answer: func(Question) float64 { return answer }}

		a, err := NewAnalyzer(cfg, newCachedJudge(identifiedFake{fake, "fake 1"}, openTestCache(t, dir)))
		if err != nil {
			t.Fatal(err)
		}

		diagnostics, err := runOn(t, a, checkedSource)
		if err != nil {
			t.Fatal(err)
		}

		var messages []string
		for _, d := range diagnostics {
			messages = append(messages, d.Message)
		}

		return messages, len(fake.Questions())
	}

	first, asked := run(0.95)
	if len(first) != 2 || asked != 2 {
		t.Fatalf("first run: %q after %d questions, want 2 findings and 2 questions", first, asked)
	}

	// A judge that has changed its mind does not get to say so.
	second, asked := run(0.05)
	if !slices.Equal(first, second) || asked != 0 {
		t.Errorf("second run: %q after %d questions, want %q and no questions", second, asked, first)
	}
}
