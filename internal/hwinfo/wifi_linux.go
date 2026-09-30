//go:build linux

package hwinfo

import "context"

// RemoveAllWiFiProfiles is a dev-only no-op on Linux — see detect_linux.go.
func RemoveAllWiFiProfiles(ctx context.Context) (*WiFiCleanupResult, error) {
	return &WiFiCleanupResult{}, nil
}
