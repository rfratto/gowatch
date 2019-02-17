// +build linux

package gowatch

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jsgilmore/mount"
)

var (
	cachedDetectDocker *bool
)

// fixDirectories fixes an issue where watched directories mounted
// inside of a docker container do not trigger events unless you
// are also watching the parents of those directories.
//
// fixDirectories will return the original list with all unique
// parent directories up to the mount point at the end.
func fixDirectories(input []string) []string {
	if !detectDocker() {
		return input
	}

	mm, err := mount.Mounts()
	if err != nil {
		log.Printf(
			"WARNING: could not get mounts (%v). Some file events may not work\n",
			err,
		)

		return input
	}

	parentsMap := make(map[string]bool)

	addParents := func(root string, path string) {
		parent := path
		for parent != root {
			parent = filepath.Dir(parent)
			parentsMap[parent] = true
		}
	}

	for _, i := range input {
		for _, m := range mm {
			if m.Filesystem != "fuse.osxfs" {
				continue
			}

			if strings.HasPrefix(i, m.Path) {
				addParents(m.Path, i)
			}
		}
	}

	for parent := range parentsMap {
		input = append(input, parent)
	}

	return input
}

func detectDocker() bool {
	if cachedDetectDocker != nil {
		return *cachedDetectDocker
	}

	f, err := os.Open("/proc/1/cgroup")
	if err != nil {
		return false
	}
	defer f.Close()

	bb, err := ioutil.ReadAll(f)
	if err != nil {
		return false
	}

	res := strings.Contains(string(bb), "/docker/")
	cachedDetectDocker = &res
	return res
}
