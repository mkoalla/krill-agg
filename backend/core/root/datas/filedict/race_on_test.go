//go:build race

package filedict

// the race detector is on: some tests are too slow to finish under it
const raceEnabled = true
