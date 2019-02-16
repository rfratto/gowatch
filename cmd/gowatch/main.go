// Command gowatch is a tool for running services and tasks triggered by
// filesystem changes.
package main

import (
	"fmt"
	"os"

	"github.com/rfratto/gowatch"
	"github.com/spf13/cobra"

	yaml "gopkg.in/yaml.v2"
)

var (
	watchDirectory string
	configFile     string
	verbose        bool
)

var rootCmd = &cobra.Command{
	Use:   `gowatch`,
	Short: "run actions and services in response to file events",
	Long: `gowatch is a tool that lets you run specific actions and maintain long-running
services in response to file events. A action is a one-off script that you
expect to finish, while a service is a script that you want to keep alive
(such as an HTTP server). When gowatch detects a service exited, it will
restart it to keep it alive.

gowatch can run any number of actions or services on startup and it can also
be configured to re-run actions and re-start services when responding to
specific file events.

Use yaml files to define configurations for events:

actions:
  vet: go vet ./...
	test: go test ./...
services:
	run: go run ./cmd/my-server
on_start:
	- run
file_triggers:
	- include: ["*.go", "**/*.go", "Gopkg.lock", "Gopkg.toml"]
		exclude: ["vendor/"]
		trigger:
			- vet
			- test
			- run

When gowatch is run with this configuration file, it will automatically start
the run service and maintain it. Then, when any go file changes in any
directory except for vendor/, or if the Gopkg files change, then
vet and test will be ran followed by restarting run if the previous two
commands passed.

Visit https://github.com/rfratto/gowatch for more information.`,
	Run: func(cmd *cobra.Command, args []string) {
		r, err := os.Open(configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load configuration file: %v\n", err)
			return
		}
		defer r.Close()

		cfg := &gowatch.Config{}
		err = yaml.NewDecoder(r).Decode(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "decoding configuration failed: %v\n", err)
			return
		}

		dir, err := os.Getwd()
		if err != nil && watchDirectory == "" {
			fmt.Fprintf(os.Stderr, "failed to get working directory: %v", err)
			return
		} else if watchDirectory != "" {
			dir = watchDirectory
		}

		w := gowatch.NewWatcher(dir, *cfg)
		w.Stdout = os.Stdout
		w.Stderr = os.Stderr

		if verbose {
			w.Debug = os.Stderr
		}

		err = w.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start gowatch: %v", err)
			return
		}
	},
}

func init() {
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "path to config file to load")
	rootCmd.Flags().StringVarP(&watchDirectory, "dir", "d", "", "directory to watch. defaults to working directory")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "adds extra output")

	rootCmd.MarkFlagRequired("config")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
