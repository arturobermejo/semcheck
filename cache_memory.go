package semcheck

import "sync"

// A memoryCache keeps decisions for as long as the program runs. Even without
// a cache on disk, a run must agree with itself: the code that a package
// shares with its test variant gets one decision, not two.
type memoryCache struct {
	mu  sync.Mutex
	yes map[cacheKey]float64
}

var _ decisionCache = (*memoryCache)(nil)

func (m *memoryCache) get(key cacheKey) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	yes, ok := m.yes[key]

	return yes, ok
}

func (m *memoryCache) put(key cacheKey, yes float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.yes == nil {
		m.yes = map[cacheKey]float64{}
	}

	m.yes[key] = yes

	return nil
}
