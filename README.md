# gowatch

[![Build Status](https://travis-ci.org/rfratto/gowatch.svg?branch=master)](https://travis-ci.org/rfratto/gowatch)
[![Go Report Card](https://goreportcard.com/badge/github.com/rfratto/gowatch)](https://goreportcard.com/report/github.com/rfratto/gowatch)
[![GoDoc](https://godoc.org/github.com/rfratto/gowatch?status.svg)](https://godoc.org/github.com/rfratto/gowatch)

gowatch is both a tool and a library to run both scripts and long-running
services (like HTTP servers) in response to file events.

*NOTE*: The design of the bianry and library of gowatch is not yet finalized
and may change in future commits. gowatch should be cross-platform but usage of
it has not yet been tested on Windows.

gowatch is driven by a yaml configuration file that tells it:

1. What scripts will be triggered by file events (called actions)
2. What long-running services will be run and restarted by file events (called
   services)
3. What triggers to run as soon as gowatch boots
4. Which files to watch and what events should they trigger when they change.

## Installing

```bash
go get -u github.com/rfratto/gowatch/...
```

## Running

```bash
gowatch -c path/to/config.yml
```

## Example configuration file

```yaml
# A list of actions that we will run. Each action is a bash-like script.
actions:
  vet: |
    echo running go vet...
    go vet ./...
  install: |
    echo running go install...
    go install ./cmd/...
# A list of services that gowatch will keep alive if they exit.
services:
  tick: |
    echo tick
    sleep 1
    echo tock
    sleep 1
# A list of actions and services we wish to trigger when starting gowatch
on_start:
  - tick
# Our list of file patterns. Each file pattern can watch a separate set of
# files and exclude patterns from that set.
file_triggers:
  # Watch all go files in the root directory and all subdirectories, excluding
  # vendor. When any of those files have changed, re-run vet, stop the tick
  # service, re-run install, and restart the tick service. tick:stop is not
  # necessary; omitting tick:stop would restart tick if it was currently running
  # once the noraml tick trigger occurs.
  #
  # If any trigger fails to launch, the whole trigger process will abort.
  - include: ["*.go", "**/*.go", "Gopkg.lock", "Gopkg.toml"]
    exclude: ["vendor/"]
    trigger:
      - vet
      - tick:stop
      - install
      - tick
```
