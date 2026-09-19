package semcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func keyOf(fragment string) cacheKey {
	return cacheKeyOf(testJudge, Question{Ask: "ask", Fragment: fragment})
}

func openTestCache(t *testing.T, dir string) *diskCache {
	t.Helper()

	c, err := openDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

// age makes a file of the cache look as if it was last touched some time ago.
func age(t *testing.T, path string, by time.Duration) {
	t.Helper()

	then := time.Now().Add(-by)
	if err := os.Chtimes(path, then, then); err != nil {
		t.Fatal(err)
	}
}

func TestDiskCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "there", "yet")
	c := openTestCache(t, dir)

	if yes, ok := c.get(keyOf("a")); ok {
		t.Errorf("get = %v from an empty cache", yes)
	}

	// Exactly what was put: a decision from the cache must not differ from the
	// one that was reported the first time.
	values := []float64{0, 1, 0.97, 0.1 + 0.2, 1e-12}

	for i, yes := range values {
		if err := c.put(keyOf(fmt.Sprint(i)), yes); err != nil {
			t.Fatal(err)
		}
	}

	// Another process, later.
	c = openTestCache(t, dir)

	for i, want := range values {
		if yes, ok := c.get(keyOf(fmt.Sprint(i))); !ok || yes != want {
			t.Errorf("get(%d) = %v, %v; want %v", i, yes, ok, want)
		}
	}

	if yes, ok := c.get(keyOf("other")); ok {
		t.Errorf("get = %v for a key that was not put", yes)
	}

	// The last answer stays.
	if err := c.put(keyOf("0"), 0.5); err != nil {
		t.Fatal(err)
	}

	if yes, _ := c.get(keyOf("0")); yes != 0.5 {
		t.Errorf("get = %v after putting 0.5 on top", yes)
	}
}

func TestDiskCacheFiles(t *testing.T) {
	dir := t.TempDir()
	c := openTestCache(t, dir)

	if err := c.put(keyOf("a"), 0.97); err != nil {
		t.Fatal(err)
	}

	var files []string

	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, _ error) error {
		if !entry.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}

		return nil
	})

	key := keyOf("a")
	name := fmt.Sprintf("%x", key[:])

	// Nothing else: no file left over from writing.
	if want := []string{name[:2] + "/" + name, trimMarker}; fmt.Sprint(files) != fmt.Sprint(want) {
		t.Errorf("files = %v, want %v", files, want)
	}

	if data, _ := os.ReadFile(c.path(key)); string(data) != "0.97\n" {
		t.Errorf("the file has %q", data)
	}
}

func TestDiskCacheIgnoresWhatItCannotRead(t *testing.T) {
	c := openTestCache(t, t.TempDir())

	contents := []string{"", "\n", "high\n", "NaN\n", "1.5\n", "-0.1\n", "0.5 \n", "0.5\n\n", `{"yes": 0.5}` + "\n", "0.97"}

	for i, data := range contents {
		key := keyOf(fmt.Sprint(i))

		if err := os.MkdirAll(filepath.Dir(c.path(key)), 0o777); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(c.path(key), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}

		if yes, ok := c.get(key); ok {
			t.Errorf("get = %v from a file with %q", yes, data)
		}
	}
}

// "go vet" runs a process for each package, many at a time.
func TestDiskCacheIsShared(t *testing.T) {
	dir := t.TempDir()

	var wg sync.WaitGroup

	for p := range 8 {
		wg.Go(func() {
			c, err := openDiskCache(dir)
			if err != nil {
				t.Error(err)

				return
			}

			for i := range 200 {
				// A few keys, so that they are written and read at once. All
				// the writers of a key put the same value.
				key, want := keyOf(fmt.Sprint(i%5)), float64(i%5)/10

				if p%2 == 0 {
					if err := c.put(key, want); err != nil {
						t.Error(err)
					}
				}

				// Whoever finds it, finds it whole.
				if yes, ok := c.get(key); ok && yes != want {
					t.Errorf("get = %v, want %v", yes, want)
				}
			}
		})
	}

	wg.Wait()
}

func TestDiskCacheErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if c, err := openDiskCache(filepath.Join(file, "below")); err == nil || !strings.HasPrefix(err.Error(), "semcheck: cache: ") {
		t.Errorf("openDiskCache below a file = %v, %v; want an error", c, err)
	}

	if os.Getuid() == 0 {
		t.Skip("root writes anywhere")
	}

	for _, readOnly := range []string{"the cache", "the directory of the key"} {
		t.Run(readOnly, func(t *testing.T) {
			dir := t.TempDir()
			c := openTestCache(t, dir)
			key := keyOf("a")

			locked := dir
			if readOnly != "the cache" {
				locked = filepath.Dir(c.path(key))

				if err := os.Mkdir(locked, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			if err := os.Chmod(locked, 0o500); err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

			if err := c.put(key, 0.5); err == nil || !strings.HasPrefix(err.Error(), "semcheck: cache: ") {
				t.Errorf("put = %v, want an error", err)
			}

			if yes, ok := c.get(key); ok {
				t.Errorf("get = %v after a put that failed", yes)
			}
		})
	}
}

func TestDiskCachePutLeavesNothingBehind(t *testing.T) {
	c := openTestCache(t, t.TempDir())
	key := keyOf("a")

	// A directory where the file should go: it cannot be renamed onto.
	if err := os.MkdirAll(c.path(key), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := c.put(key, 0.5); err == nil {
		t.Fatal("want an error")
	}

	entries, err := os.ReadDir(filepath.Dir(c.path(key)))
	if err != nil || len(entries) != 1 {
		t.Errorf("entries = %v, %v; want only the directory", entries, err)
	}
}

func TestDiskCacheTrim(t *testing.T) {
	dir := t.TempDir()
	c := openTestCache(t, dir)

	for _, fragment := range []string{"forgotten", "in use", "new"} {
		if err := c.put(keyOf(fragment), 0.5); err != nil {
			t.Fatal(err)
		}
	}

	leftover := filepath.Join(filepath.Dir(c.path(keyOf("new"))), "tmp-123")
	if err := os.WriteFile(leftover, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	age(t, c.path(keyOf("forgotten")), cacheMaxAge+time.Hour)
	age(t, c.path(keyOf("in use")), cacheMaxAge+time.Hour)
	age(t, leftover, cacheMaxAge+time.Hour)

	// Finding a decision makes it young again.
	if _, ok := c.get(keyOf("in use")); !ok {
		t.Fatal("the decision is not there")
	}

	// It was trimmed a moment ago, when it was opened: not again so soon.
	c = openTestCache(t, dir)

	if _, ok := c.get(keyOf("forgotten")); !ok {
		t.Fatal("trimmed twice in a day")
	}

	age(t, c.path(keyOf("forgotten")), cacheMaxAge+time.Hour)
	age(t, filepath.Join(dir, trimMarker), cacheDay+time.Hour)

	c = openTestCache(t, dir)

	for fragment, want := range map[string]bool{"forgotten": false, "in use": true, "new": true} {
		if _, ok := c.get(keyOf(fragment)); ok != want {
			t.Errorf("%q: found = %v, want %v", fragment, ok, want)
		}
	}

	if _, err := os.Stat(leftover); err == nil {
		t.Error("the file left over from a put that did not end is still there")
	}

	if info, err := os.Stat(filepath.Join(dir, trimMarker)); err != nil || time.Since(info.ModTime()) > time.Hour {
		t.Errorf("the marker was not renewed: %v, %v", info, err)
	}
}

func TestCacheDir(t *testing.T) {
	t.Setenv(CacheEnv, "off")

	if dir, err := cacheDir(); dir != "" || err != nil {
		t.Errorf("off: cacheDir = %q, %v", dir, err)
	}

	t.Setenv(CacheEnv, "some/dir")

	if dir, err := cacheDir(); dir != "some/dir" || err != nil {
		t.Errorf("a directory: cacheDir = %q, %v", dir, err)
	}

	t.Setenv(CacheEnv, "")

	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	if dir, err := cacheDir(); dir != filepath.Join(base, "semcheck") || err != nil {
		t.Errorf("default: cacheDir = %q, %v; want it in %s", dir, err, base)
	}

	// No home, no cache directory of the user.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	if dir, err := cacheDir(); err == nil || !strings.Contains(err.Error(), CacheEnv) {
		t.Errorf("no home: cacheDir = %q, %v; want an error that names %s", dir, err, CacheEnv)
	}
}
