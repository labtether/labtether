//go:build !darwin

package securityruntime

import "syscall"

func bindRawConnToInterface(_ syscall.RawConn, _ *sameSubnetBinding) error {
	return nil
}
