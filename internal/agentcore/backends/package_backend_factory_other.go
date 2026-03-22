//go:build !darwin

package backends

func newDarwinPackageBackend() PackageBackend {
	return UnsupportedPackageBackend{OS: "darwin"}
}
