package semcheck

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CacheEnv names the directory where decisions are kept, instead of one in the
// cache directory of the user. "off" keeps them for the run only.
const CacheEnv = "SEMCHECK_CACHE"

const (
	// cacheMaxAge is how long a decision that nobody asks for is kept. Code
	// that is gone leaves its decisions behind.
	cacheMaxAge = 30 * 24 * time.Hour

	// cacheDay is how often old decisions are looked for, and how old the date
	// of a decision gets before using it is worth writing down.
	cacheDay = 24 * time.Hour

	trimMarker = "trimmed"
)

// A decisionCache remembers the probability of yes of the questions that have
// been answered.
type decisionCache interface {
	get(key cacheKey) (yes float64, ok bool)
	put(key cacheKey, yes float64) error
}

// cacheDir returns the directory for the decisions, or "" if CacheEnv turns
// the cache off.
func cacheDir() (string, error) {
	switch dir := os.Getenv(CacheEnv); dir {
	case "off":
		return "", nil
	case "":
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("semcheck: %w (set %s to a directory, or to off)", err, CacheEnv)
		}

		return filepath.Join(base, "semcheck"), nil
	default:
		return dir, nil
	}
}

// A diskCache keeps every decision in a file of its own, named after its key.
// Several processes can share the directory, which is what "go vet" needs, with
// one process for each package: a file shows up whole or not at all.
type diskCache struct {
	dir string
}

var _ decisionCache = (*diskCache)(nil)

func openDiskCache(dir string) (*diskCache, error) {
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, fmt.Errorf("semcheck: cache: %w", err)
	}

	d := &diskCache{dir}
	d.trim()

	return d, nil
}

// path spreads the files over 256 directories: some file systems slow down
// with many thousands of files in one.
func (d *diskCache) path(key cacheKey) string {
	name := hex.EncodeToString(key[:])

	return filepath.Join(d.dir, name[:2], name)
}

// get takes anything it cannot make sense of for a decision it does not have.
func (d *diskCache) get(key cacheKey) (float64, bool) {
	path := d.path(key)

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}

	// Without the end of the line, the file was cut short: "0.9" for "0.97".
	text, whole := strings.CutSuffix(string(data), "\n")

	yes, err := strconv.ParseFloat(text, 64)
	if !whole || err != nil || math.IsNaN(yes) || yes < 0 || yes > 1 {
		return 0, false
	}

	// The date of a file is when it was last of use, for trim.
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > cacheDay {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}

	return yes, true
}

func (d *diskCache) put(key cacheKey, yes float64) error {
	// -1: as many digits as it takes to read back the very same number.
	data := strconv.FormatFloat(yes, 'g', -1, 64) + "\n"

	if err := writeWhole(d.path(key), data); err != nil {
		return fmt.Errorf("semcheck: cache: %w", err)
	}

	return nil
}

// writeWhole writes to a file with another name, and renames it when it is
// complete: renaming is atomic, so nobody ever reads half a decision, and of
// two processes that write the same file one wins, whole.
func writeWhole(path, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "tmp-*")
	if err != nil {
		return err
	}

	_, err = tmp.WriteString(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}

	if err != nil {
		_ = os.Remove(tmp.Name())
	}

	return err
}

// trim removes, once a day at most, what has not been of use for cacheMaxAge.
// It does what it can: a cache that cannot be tidied up still works.
func (d *diskCache) trim() {
	marker := filepath.Join(d.dir, trimMarker)

	if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < cacheDay {
		return
	}

	// First, so that the processes that start meanwhile do not do the same.
	if err := os.WriteFile(marker, nil, 0o666); err != nil {
		return
	}

	cutoff := time.Now().Add(-cacheMaxAge)

	_ = filepath.WalkDir(d.dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //nolint:nilerr // whatever cannot be read is left alone
		}

		if info, err := entry.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}

		return nil
	})
}
