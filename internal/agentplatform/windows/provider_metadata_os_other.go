//go:build !windows

package windows

func readWindowsOSMetadata() map[string]string {
	return map[string]string{}
}
