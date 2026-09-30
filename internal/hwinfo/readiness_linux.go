//go:build linux

package hwinfo

import "context"

// DetectOSReadiness is a dev-only stub on Linux — see detect_linux.go's
// Detect() for why. Not meant to ship.
func DetectOSReadiness(ctx context.Context) (*OSReadiness, error) {
	return &OSReadiness{PendingUpdates: -1}, nil
}
