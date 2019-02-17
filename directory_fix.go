// +build !linux

package gowatch

// Do nothing on a non-linux build
func fixDirectories(input []string) []string {
	return input
}
