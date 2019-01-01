package gowatch_test

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"testing"

	"github.com/rfratto/gowatch"
)

func wd(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	return projectd(wd)
}

func projectd(wd string) string {
	return path.Join(wd, "testdata", "js-project")
}

func getFtWatcher(inc []string, exc []string) gowatch.Watcher {
	wd, _ := os.Getwd()

	return gowatch.Watcher{
		Directory: projectd(wd),
		Config: gowatch.Config{
			FileTriggers: []gowatch.FileTrigger{
				gowatch.FileTrigger{
					Include:  inc,
					Exclude:  exc,
					Triggers: []string{"foo", "bar"},
				},
			},
		},
	}
}

func compareWatched(t *testing.T, act []string, expect []string) {
	sortedAct := make([]string, len(act))
	copy(sortedAct, act)
	sort.Strings(sortedAct)

	sortedExp := make([]string, len(expect))
	copy(sortedExp, expect)
	sort.Strings(sortedExp)

	if !reflect.DeepEqual(sortedAct, sortedExp) {
		t.Errorf("expected paths %v to be %v", sortedAct, sortedExp)
	}
}

func TestMatchingTriggers(t *testing.T) {
	wd := wd(t)

	tt := []struct {
		name  string
		inc   []string
		path  string
		match bool
	}{
		{"watched directory", []string{"."}, path.Join(wd), true},
		{"watched file", []string{"package.json"}, path.Join(wd, "package.json"), true},
		{"watched file in watched directory", []string{"."}, path.Join(wd, "package.json"), true},
		{"unwatched directory", []string{"."}, path.Join(wd, "src"), false},
		{"unwatched file", []string{"."}, path.Join(wd, "src", "main.js"), false},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			w := getFtWatcher(tc.inc, nil)
			matches, err := w.MatchingTriggers(tc.path)
			if err != nil {
				t.Fatal(err)
			}

			if tc.match && len(matches) != 1 {
				t.Errorf("expected number of matches %d, got %d", 1, len(matches))
			} else if !tc.match && len(matches) != 0 {
				t.Errorf("expected number of matches %d, got %d", 0, len(matches))
			}
		})
	}
}

func TestWatchAbsolutePath(t *testing.T) {
	p, err := ioutil.TempDir("", "gowatch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(p)

	os.Mkdir(path.Join(p, "src"), os.ModePerm)
	os.Mkdir(path.Join(p, "src", "lib"), os.ModePerm)
	os.Mkdir(path.Join(p, "node_modules"), os.ModePerm)

	tt := []struct {
		name string
		inc  []string
		exp  []string
	}{
		{"absolute path", []string{
			path.Join(p, "**/"),
		}, []string{
			path.Join(p, "src"),
			path.Join(p, "src", "lib"),
			path.Join(p, "node_modules"),
		}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			w := getFtWatcher(tc.inc, nil)
			act := w.WatchedPaths()
			compareWatched(t, act, tc.exp)
		})
	}
}

func TestWatchAbsolutePathIgnored(t *testing.T) {
	p, err := ioutil.TempDir("", "gowatch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(p)

	os.Mkdir(path.Join(p, "src"), os.ModePerm)
	os.Mkdir(path.Join(p, "src", "lib"), os.ModePerm)
	os.Mkdir(path.Join(p, "node_modules"), os.ModePerm)

	tt := []struct {
		name string
		inc  []string
		exc  []string
		exp  []string
	}{
		{"absolute path", []string{
			path.Join(p, "**/"),
		}, []string{
			path.Join(p, "src/"),
		}, []string{
			path.Join(p, "node_modules"),
		}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			w := getFtWatcher(tc.inc, tc.exc)
			act := w.WatchedPaths()
			compareWatched(t, act, tc.exp)
		})
	}
}

func TestWatchAbsoluteFile(t *testing.T) {
	f, err := ioutil.TempFile("", "*.gowatch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	tt := []struct {
		name string
		inc  []string
		exp  []string
	}{
		{"absolute path", []string{f.Name()}, []string{f.Name()}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			w := getFtWatcher(tc.inc, nil)
			act := w.WatchedPaths()
			compareWatched(t, act, tc.exp)
		})
	}
}

func TestWatchedPaths(t *testing.T) {
	wd := wd(t)

	tt := []struct {
		name string
		inc  []string
		exp  []string
	}{
		{"current dir", []string{"."}, []string{wd}},
		{"single file", []string{"package.json"}, []string{path.Join(wd, "package.json")}},
		{"duplicate entry", []string{"package.json", "package.json"}, []string{path.Join(wd, "package.json")}},

		// although we have to includes here, we watch per-folder so we should roll
		// this up into one.
		{"simplifiy", []string{".", "package.json"}, []string{wd}},

		{"single dir", []string{"src"}, []string{path.Join(wd, "src")}},
		{"single dir trailing slash", []string{"src/"}, []string{path.Join(wd, "src")}},
		{"**/", []string{"./", "**/"}, []string{
			wd,
			path.Join(wd, "node_modules"),
			path.Join(wd, "src"),
			path.Join(wd, "src", "lib"),
		}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			w := getFtWatcher(tc.inc, nil)
			act := w.WatchedPaths()
			compareWatched(t, act, tc.exp)
		})
	}
}

func TestWatchedPathsIgnored(t *testing.T) {
	expect := []string{wd(t)}

	w := getFtWatcher([]string{".", "**/"}, []string{"node_modules/", "src/"})
	actual := w.WatchedPaths()

	compareWatched(t, actual, expect)
}
