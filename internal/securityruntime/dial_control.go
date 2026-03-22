package securityruntime

import (
	"context"
	"syscall"
)

func wrapDialerControl(
	base func(context.Context, string, string, syscall.RawConn) error,
	binding *sameSubnetBinding,
) func(context.Context, string, string, syscall.RawConn) error {
	if binding == nil || binding.ifaceIndex <= 0 {
		return base
	}

	return func(ctx context.Context, network, address string, rawConn syscall.RawConn) error {
		if base != nil {
			if err := base(ctx, network, address, rawConn); err != nil {
				return err
			}
		}
		return bindRawConnToInterface(rawConn, binding)
	}
}
