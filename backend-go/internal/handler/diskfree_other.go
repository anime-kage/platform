//go:build !linux

package handler

// diskUsage is Linux-only (the deploy target and the dev container are both
// Linux). Elsewhere it reports "unknown" rather than failing the endpoint —
// the staging total is still useful on its own.
func diskUsage(string) (total, free uint64, err error) {
	return 0, 0, nil
}
