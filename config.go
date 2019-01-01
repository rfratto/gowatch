package gowatch

// Config holds the configuration for the directory tree that will be watched
// and the scripts that will be ran on it.
type Config struct {
	// Actions is a named list of oneshot scripts.
	Actions map[string]string `yaml:"actions"`

	// Services is a named list of long-running scripts that are intended to not exit.
	Services map[string]string `yaml:"services"`

	// StartupSteps holds the list of actions and services to run on start.
	StartupSteps []string `yaml:"on_start"`

	// FileTriggers holds a list of file events to watch for and a list of
	// scripts to execute when a matching event occurs. This is a sorted list;
	// earlier triggers are treated as higher precedence and will execute
	// first.
	FileTriggers []FileTrigger `yaml:"file_triggers"`
}
