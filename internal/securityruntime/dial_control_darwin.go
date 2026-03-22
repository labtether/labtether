//go:build darwin

package securityruntime

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

const maxInt = int(^uint(0) >> 1)

func bindRawConnToInterface(rawConn syscall.RawConn, binding *sameSubnetBinding) error {
	if rawConn == nil || binding == nil || binding.ifaceIndex <= 0 || binding.localIP == nil {
		return nil
	}

	var sockErr error
	controlErr := rawConn.Control(func(fd uintptr) {
		if fd > uintptr(maxInt) {
			sockErr = fmt.Errorf("socket descriptor %d exceeds int range", fd)
			return
		}
		socketFD := int(fd)
		if binding.localIP.To4() != nil {
			sockErr = unix.SetsockoptInt(socketFD, unix.IPPROTO_IP, unix.IP_BOUND_IF, binding.ifaceIndex)
			return
		}
		if binding.localIP.To16() != nil {
			sockErr = unix.SetsockoptInt(socketFD, unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, binding.ifaceIndex)
		}
	})
	if controlErr != nil {
		return controlErr
	}
	return sockErr
}
