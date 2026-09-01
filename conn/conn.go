package conn

import (
	"context"
	"net"
	"syscall"
)

type setFunc = func(fd int, network string) error

type setFuncSlice []setFunc

func (fns setFuncSlice) controlContextFunc() func(ctx context.Context, network, address string, c syscall.RawConn) error {
	if len(fns) == 0 {
		return nil
	}
	return func(ctx context.Context, network, address string, c syscall.RawConn) (err error) {
		if cerr := c.Control(func(fd uintptr) {
			for _, fn := range fns {
				if err = fn(int(fd), network); err != nil {
					return
				}
			}
		}); cerr != nil {
			return cerr
		}
		return
	}
}

// DialerSocketOptions contains dialer-specific socket options.
type DialerSocketOptions struct {
	// Fwmark sets the dialer's fwmark on Linux, or user cookie on FreeBSD.
	//
	// Available on Linux and FreeBSD.
	Fwmark int
}

// Dialer returns a [net.Dialer] with a control function that sets the socket options.
func (dso DialerSocketOptions) Dialer() net.Dialer {
	return net.Dialer{
		ControlContext: dso.buildSetFns().controlContextFunc(),
	}
}
