package gowatch

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"mvdan.cc/sh/interp"
	"mvdan.cc/sh/syntax"
)

type triggerWriter struct {
	Name string
	w    io.Writer

	wroteHeader bool
}

func (t *triggerWriter) writeHeader() {
	bb := fmt.Sprintf("[%s] ", t.Name)
	t.w.Write([]byte(bb))
	t.wroteHeader = true
}

func (t *triggerWriter) Write(p []byte) (n int, err error) {
	total := 0
	for _, b := range p {
		// Write the header every time a newline is written
		if !t.wroteHeader {
			t.writeHeader()
		}

		n, err := t.w.Write([]byte{b})
		if err != nil {
			return total, err
		}
		total += n

		if b == '\n' {
			t.wroteHeader = false
		}
	}

	return total, nil
}

// Watcher is the instance of the watcher itself. It holds the configuration
// for the directory tree to be watched and the root directory to watch.
type Watcher struct {
	// Working directory to watch
	Directory string

	// The writer for debug output to go to.
	Debug io.Writer

	// The writer for triggers to write output to
	Stdout io.Writer

	// The writer for triggers to write errors to
	Stderr io.Writer

	// Config of file triggers and events to run
	Config Config

	services map[string]*service
	files    map[string]*syntax.File
	ctx      context.Context
}

func (w *Watcher) parseTriggerName(orig string) (trigger string, action string) {
	parts := strings.SplitN(orig, ":", 2)

	trigger = parts[0]
	if len(parts) == 2 {
		action = parts[1]
	}

	return
}

func (w *Watcher) validateTriggerNames() error {
	// Get a list of all triggers
	allTriggers := w.Config.StartupSteps
	for _, ft := range w.Config.FileTriggers {
		allTriggers = append(allTriggers, ft.Triggers...)
	}

	invalidTriggers := []string{}

	// For each unique trigger, make sure there's an action or
	// a service that matches it
outer:
	for _, trigger := range uniqueStringSlice(allTriggers) {
		trigger, _ = w.parseTriggerName(trigger)

		for action := range w.Config.Actions {
			if action == trigger {
				continue outer
			}
		}

		for service := range w.Config.Services {
			if service == trigger {
				continue outer
			}
		}

		invalidTriggers = append(invalidTriggers, trigger)
	}

	if len(invalidTriggers) == 1 {
		return fmt.Errorf(
			"the referenced trigger %s does not exist",
			invalidTriggers[0],
		)
	} else if len(invalidTriggers) > 1 {
		return fmt.Errorf(
			"the following referenced triggers do not exist: %s",
			strings.Join(invalidTriggers, ", "),
		)
	}

	return nil
}

func (w Watcher) validateActionNames() error {
	invalid := []string{}

	for action := range w.Config.Actions {
		if strings.ContainsRune(action, ':') {
			invalid = append(invalid, action)
		}
	}

	for service := range w.Config.Services {
		if strings.ContainsRune(service, ':') {
			invalid = append(invalid, service)
		}
	}

	invalid = uniqueStringSlice(invalid)

	if len(invalid) == 1 {
		return fmt.Errorf(
			"%s name invalid; cannot use ':'",
			invalid[0],
		)
	} else if len(invalid) > 1 {
		return fmt.Errorf(
			"the following names are invalid because they have a ':' in the name: %s",
			strings.Join(invalid, ", "),
		)
	}

	return nil
}

func (w *Watcher) validateServiceUniqueness() error {
	invalidServices := []string{}

outer:
	for service := range w.Config.Services {
		for action := range w.Config.Actions {
			if service == action {
				invalidServices = append(invalidServices, service)
				continue outer
			}
		}
	}

	invalidServices = uniqueStringSliceOrdered(invalidServices)

	if len(invalidServices) == 1 {
		return fmt.Errorf(
			"%s is defined as both an action and a service",
			invalidServices[0],
		)
	} else if len(invalidServices) > 1 {
		return fmt.Errorf(
			"the following are defined as both actions and services: %s",
			strings.Join(invalidServices, ", "),
		)
	}

	return nil
}

// Validate validates the configuration file and returns any errors.
func (w *Watcher) Validate() error {
	type validateFunc func() error

	validations := []validateFunc{
		w.validateTriggerNames,
		w.validateServiceUniqueness,
		w.validateActionNames,
	}

	for _, validation := range validations {
		if err := validation(); err != nil {
			return err
		}
	}

	return nil
}

func (w *Watcher) watchForNewPatterns(init []string, n *fsnotify.Watcher) {
	watchedMap := make(map[string]bool)
	for _, p := range init {
		watchedMap[p] = true
	}

	reloadPatterns := func() {
		latest := w.WatchedPaths()
		addedPaths := []string{}
		for _, p := range latest {
			_, exists := watchedMap[p]
			if !exists {
				addedPaths = append(addedPaths, p)
			}

			watchedMap[p] = true
		}

		if len(addedPaths) > 0 {
			watched := uniqueStringSlice(getDirs(addedPaths))
			for _, p := range watched {
				if err := n.Add(p); err != nil {
					fmt.Fprintf(w.Debug, "failed to add new path %s: %v\n", p, err)
				} else {
					fmt.Fprintf(w.Debug, "watching new path %s\n", p)
				}
			}
		}
	}

	// New files may be added at any point while gowatch continues to run.
	// Every 1s we'll see if there are any new files that've been added so we
	// can tell the watcher to include them.
	for {
		select {
		case <-time.After(1 * time.Second):
			reloadPatterns()
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Watcher) watchLoop(n *fsnotify.Watcher) error {
	eventsBuffer := []string{}

	var (
		handlerContext context.Context
		handlerCancel  context.CancelFunc
		flushTimer     <-chan time.Time
	)

	for {
		select {
		case ev := <-n.Events:
			if ev.Op == fsnotify.Chmod {
				break
			}

			eventsBuffer = append(eventsBuffer, ev.Name)

			if flushTimer == nil {
				flushTimer = time.After(250 * time.Millisecond)
			}
		case err := <-n.Errors:
			fmt.Println(err)
		case <-flushTimer:
			if len(w.triggersForFiles(eventsBuffer)) == 0 {
				eventsBuffer = []string{}
				flushTimer = nil
				continue
			}

			if handlerCancel != nil {
				handlerCancel()
			}

			handlerContext, handlerCancel = context.WithCancel(context.Background())

			go w.handleFilesChanged(handlerContext, eventsBuffer)
			eventsBuffer = []string{}
			flushTimer = nil
		case <-w.ctx.Done():
			if handlerCancel != nil {
				handlerCancel()
			}
			return w.ctx.Err()
		}
	}
}

// Start starts the watcher. Start should not exit normally unless an error occurred or
// the watcher is cancelled through the context passed to NewWatchWithContext.
func (w *Watcher) Start() error {
	if err := w.Validate(); err != nil {
		return err
	}

	if err := w.compileFiles(); err != nil {
		return err
	} else if err := w.compileServices(); err != nil {
		return err
	}

	// Before we start the watcher, run all the startup triggers
	for _, start := range w.Config.StartupSteps {
		err := w.Run(context.Background(), start)
		if err != nil {
			return fmt.Errorf("startup trigger %s failed: %v", start, err)
		}
	}

	n, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("unable to start watcher: %v", err)
	}

	paths := w.WatchedPaths()
	watched := uniqueStringSlice(getDirs(paths))
	if len(watched) == 0 {
		return fmt.Errorf("no paths to watch")
	}

	for _, p := range watched {
		if err := n.Add(p); err != nil {
			return err
		}
	}

	go w.watchForNewPatterns(paths, n)
	return w.watchLoop(n)
}

func (w *Watcher) stopService(ctx context.Context, trigger string) error {
	s, ok := w.services[trigger]
	if !ok {
		return fmt.Errorf("no service named %s found", trigger)
	}

	// Stop the service. Fails if it's not running, but we don't care.
	s.Stop()
	return nil
}

func (w *Watcher) runService(ctx context.Context, trigger string) error {
	s, ok := w.services[trigger]
	if !ok {
		return fmt.Errorf("no service named %s found", trigger)
	}

	// Stop the service. Fails if it's not running, but we don't care.
	s.Stop()

	tout := &triggerWriter{Name: trigger, w: w.Stdout}
	terr := &triggerWriter{Name: trigger, w: w.Stderr}

	// Start running the service in a new goroutine. We want to directly
	// handle it being cancelled so we don't propagate the context above.
	go s.Run(context.Background(), tout, terr)
	return nil
}

func (w *Watcher) runAction(ctx context.Context, trigger string) error {
	f, ok := w.files[trigger]
	if !ok {
		return fmt.Errorf("no action named %s found", trigger)
	}

	tout := &triggerWriter{Name: trigger, w: w.Stdout}
	terr := &triggerWriter{Name: trigger, w: w.Stderr}

	runner, err := interp.New(
		interp.Dir(w.Directory),
		interp.StdIO(nil, tout, terr),
	)
	if err != nil {
		return err
	}

	return runner.Run(ctx, f)
}

// Run runs a specific named trigger defined from the watcher's config. The trigger
// can either be a service or an action.
func (w *Watcher) Run(ctx context.Context, trigger string) error {
	trigger, action := w.parseTriggerName(trigger)

	_, ok := w.files[trigger]
	if ok {
		if action != "" {
			return fmt.Errorf("trigger verb %s not supported for actions", action)
		}

		return w.runAction(ctx, trigger)
	}

	_, ok = w.services[trigger]
	if ok {
		if action == "stop" {
			return w.stopService(ctx, trigger)
		}

		if action != "" {
			return fmt.Errorf("trigger verb %s not supported for actions", action)
		}

		return w.runService(ctx, trigger)
	}

	return fmt.Errorf("no action or service named %s found", trigger)
}

// MatchingTriggers takes a full path to a file and returns all trigers that
// match that path.
func (w *Watcher) MatchingTriggers(path string) (triggers []FileTrigger, err error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("path must be absolute")
	}

	for _, t := range w.Config.FileTriggers {
		if t.Matches(w.Directory, path) {
			triggers = append(triggers, t)
		}
	}

	return
}

// WatchedPaths returns the list of files and directories that will be monitored
// by the watcher. Each path is the absolute path on disk.
func (w *Watcher) WatchedPaths() []string {
	matched := []string{}

	for _, ft := range w.Config.FileTriggers {
		ww := ft.watchedPaths(w.Directory)
		for _, w := range ww {
			matched = append(matched, w)
		}
	}

	return reducePaths(uniqueStringSlice(matched))
}

// NewWatcherWithContext returns a new Watcher given a directory to watch and
// a config with file patterns and triggers. It accepts a context that, when
// the watcher is started, allows for cancellation.
func NewWatcherWithContext(ctx context.Context, dir string, config Config) *Watcher {
	return &Watcher{
		Directory: dir,
		Config:    config,
		Debug:     ioutil.Discard,
		Stdout:    ioutil.Discard,
		Stderr:    ioutil.Discard,

		ctx: ctx,
	}
}

// NewWatcher returns a new Watcher given a directory to watch and a
// config with file patterns and triggers.
func NewWatcher(dir string, config Config) *Watcher {
	return NewWatcherWithContext(context.Background(), dir, config)
}

func (w *Watcher) triggersForFiles(files []string) []string {
	// Get the list of triggers from all the files that changed
	shouldTrigger := []string{}
	for _, file := range files {
		matching, err := w.MatchingTriggers(file)
		if err != nil {
			log.Println(err)
		}

		for _, match := range matching {
			for _, trigger := range match.Triggers {
				shouldTrigger = append(shouldTrigger, trigger)
			}
		}
	}

	return uniqueStringSliceOrdered(shouldTrigger)
}

func (w *Watcher) handleFilesChanged(ctx context.Context, files []string) {
	triggerList := w.triggersForFiles(files)

outer:
	for _, trigger := range triggerList {
		select {
		// Stop processing more triggers
		case <-ctx.Done():
			return
		default:
			fmt.Fprintf(w.Debug, "[%s] STARTING\n", trigger)

			err := w.Run(ctx, trigger)
			if err != nil && err != context.Canceled {
				fmt.Fprintf(w.Stderr, "[%s] FAILED: %v\n", trigger, err)

				// Stop the other triggers from running if a command
				// fails.
				break outer
			} else if err == context.Canceled {
				fmt.Fprintf(w.Stderr, "[%s] CANCELLED\n", trigger)
			}
		}
	}
}

func (w *Watcher) compileFiles() error {
	w.files = make(map[string]*syntax.File)

	p := syntax.NewParser()
	for name, action := range w.Config.Actions {
		r := strings.NewReader(action)
		f, err := p.Parse(r, name)
		if err != nil {
			return fmt.Errorf("failed parsing action %s: %v", name, err)
		}
		w.files[name] = f
	}

	return nil
}

func (w *Watcher) compileServices() error {
	w.services = make(map[string]*service)

	p := syntax.NewParser()
	for name, action := range w.Config.Services {
		r := strings.NewReader(action)
		f, err := p.Parse(r, name)
		if err != nil {
			return fmt.Errorf("failed parsing service %s: %v", name, err)
		}

		w.services[name] = &service{
			Dir:  w.Directory,
			File: f,
		}
	}

	return nil
}

// reducePaths will reduce the number of watched paths by combining
// multiple paths in the same folder to the folder.
func reducePaths(input []string) []string {
	m := make(map[string][]string)

	for _, i := range input {
		dir := i
		if !isDir(i) {
			dir = filepath.Dir(i)
		}

		m[dir] = append(m[dir], i)
	}

	reduced := []string{}
	for k, v := range m {
		if len(v) > 1 {
			reduced = append(reduced, k)
		} else {
			reduced = append(reduced, v[0])
		}
	}

	return reduced
}

// uniqueStringSliceOrdered removes duplicate elements from an input
// list while retaining their order
func uniqueStringSliceOrdered(input []string) []string {
	output := []string{}

outer:
	for _, i := range input {
		// is i in output?
		for _, o := range output {
			if i == o {
				continue outer
			}
		}

		output = append(output, i)
	}

	return output
}

// uniqueStringSlice removes duplicate elements from an input slice.
// Order may not be retained.
func uniqueStringSlice(input []string) []string {
	unique := make(map[string]bool)

	for _, str := range input {
		unique[str] = true
	}

	ret := []string{}
	for u := range unique {
		ret = append(ret, u)
	}

	return ret
}
