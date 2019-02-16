// Package gowatch implements utilities for watching a subset of directories
// and running sequences of actions when given files change.
//
// The core of gowatch is gowatch.Config, which holds the following:
//
// 1. A list of named scripts (actions and services). An action script is
// something that exits, while a service does not. For example, starting a
// server is considered a service, since it continues to run in the
// background. Each script has a name that can be referenced by other
// parts of the config.
//
// 2. A list of scripts to run on startup of gowatch.
//
// 3. A list of file triggers. The combined list of file triggers indicate
// which directories gowatch should watch for file changes.
//
// A file trigger holds the following:
//
// 1. The files/directories to include in the trigger.
//
// 2. The files/directories to exclude from the included list.
//
// 3. The scripts to trigger when a file in the watched list changes.
//
// Include and exclude patterns can be glob expressions to a path relative to the
// working directory of the gowatch to an absolute path. Glob expressions match *
// to a directory and ** to any level of subdirectories. For example, src/**/*.js
// matches any JS file in any subfolder of src.
//
// To have a pattern that only matches against directories, append a / at the end
// of the pattern. For example, **/ matches all directories.
//
// Relative patterns will match relative to the working directory as defined by
// gowatch.Watcher. Absolute paths can still be used to watch paths outside of the
// working directory.
//
// Triggers
//
// When a sequence of scripts is triggered, actions will be fired off
// and gowatch will wait for them to finish before going to the next script
// in the sequence. If the script is a service, one of two things will happen:
// It will be started if it is not currently running, and it will be restarted
// if it is.
//
// File System Events
//
// File Triggers are collected in batches in case of many files changing at once.
// Whenever a file change is detected, a 250ms timer starts. All other file changes
// within that 250ms window will be collected. After that 250ms timer expires, all
// file updates collected in that batch will be analyzed and the proper triggers
// will fire.
//
// Trigger Priority
//
// Triggers run in the order as defined in the trigger list. If multiple file_triggers
// match, the actions and services will be ran in definition order with duplicates
// removed. Each action will be run to completion before the next one is started.
//
// Trigger Cancellation
//
// If another trigger event occurs while one or more triggers is queued up to run,
// then the queue will be cancelled and the running trigger will be aborted.
//
// Here is an example configuration YAML file for a NodeJS project that uses gulp:
//
//   actions:
//     install: npm i
//     build: gulp build
//   services:
//     run: gulp start
//   on_start:
//     - install
//     - build
//     - run
//   file_triggers:
//     - include: ["/tmp/cache.lock"]
//       trigger:
//         - build
//         - run
//       exclude: ["node_modules/", "build/", "package.json", "package-lock.json"]
//       trigger:
//         - build
//         - run
//     - include:
//         - package.json
//         - package-lock.json
//       trigger:
//         - install
//         - build
//         - run
//
package gowatch
