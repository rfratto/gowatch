package gowatch

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar"
)

// A FileTrigger is a pattern of whitelisted and blacklisted files that
// will invoke a series of steps when a file within the watched list
// changes.
type FileTrigger struct {
	// Include holds patterns to include when checking if the file trigger
	// is activated. A * matches all files.
	Include []string `yaml:"include"`

	// Exclude holds patterns to ignore when checking if the file trigger
	// is activated.
	Exclude []string `yaml:"exclude"`

	// Triggers holds the list of scripts and services to trigger when the
	// file trigger is detected.
	Triggers []string `yaml:"trigger"`
}

// Matches takes an path to a file and returns whether or not that path
// is included in the current trigger.
func (t *FileTrigger) Matches(root string, path string) bool {
	// Get containing directory of path
	dir := path
	if !isDir(path) {
		dir = filepath.Dir(path)
	}

	watched := t.watchedPaths(root)
	for _, w := range watched {
		if w == path || w == dir {
			return true
		}
	}

	return false
}

func (t *FileTrigger) watchedPaths(root string) []string {
	if len(t.Triggers) == 0 {
		return nil
	}

	absInc := makeAbsolute(root, t.Include)
	absExc := makeAbsolute(root, t.Exclude)

	return findAbsolutes(absInc, absExc)
}

func contains(list []string, entry string) bool {
	for _, e := range list {
		if e == entry {
			return true
		}
	}

	return false
}

func makeAbsolute(root string, patterns []string) []string {
	absPatterns := []string{}

	for _, p := range patterns {
		if filepath.IsAbs(p) {
			absPatterns = append(absPatterns, p)
			continue
		}

		absPath := path.Join(root, p)
		if strings.HasSuffix(p, "/") {
			absPath = absPath + "/"
		}

		absPatterns = append(absPatterns, absPath)
	}

	return absPatterns
}

func getDirs(paths []string) []string {
	dirs := []string{}
	for _, path := range paths {
		if isDir(path) {
			dirs = append(dirs, path)
		} else {
			dirs = append(dirs, filepath.Dir(path))
		}
	}

	return fixDirectories(dirs)
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fi.IsDir()
}

func findAbsolutes(inc []string, exc []string) []string {
	getAbsolutePatterns := func(in []string) []string {
		out := []string{}
		for _, i := range in {
			if filepath.IsAbs(i) {
				out = append(out, i)
			}
		}
		return out
	}

	expandPatterns := func(in []string) []string {
		matches := []string{}
		for _, i := range in {
			mm, _ := doublestar.Glob(i)

			// If our pattern ends in a /, only add
			// directories.
			if strings.HasSuffix(i, "/") {
				for _, m := range mm {
					if isDir(m) {
						matches = append(matches, m)
					}
				}
				continue
			}

			matches = append(matches, mm...)
		}
		return matches
	}

	// substr returns true if str is a suffix of any
	// element in strs.
	substr := func(str string, strs []string) bool {
		for _, prefix := range strs {
			if strings.HasPrefix(str, prefix) {
				return true
			}
		}

		return false
	}

	// diff gets elements of inc that are not in exc or are
	// subpaths of any element in exc.
	diff := func(inc []string, exc []string) []string {
		excludedMap := make(map[string]bool)
		for _, e := range exc {
			excludedMap[e] = true
		}

		ret := []string{}
		for _, i := range inc {
			if _, ok := excludedMap[i]; !ok && !substr(i, exc) {
				ret = append(ret, i)
			}
		}

		return ret
	}

	// get include matches and exclude matches for
	// absolute inc/exc patterns
	imatches := expandPatterns(getAbsolutePatterns(inc))
	ematches := expandPatterns(getAbsolutePatterns(exc))

	// get matches that aren't excluded
	return diff(imatches, ematches)
}
