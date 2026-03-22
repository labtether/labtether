//go:build !darwin

package backends

func newDarwinLogBackend() LogBackend {
	return UnsupportedLogBackend{OS: "darwin"}
}
