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

// Dialer wraps a [net.Dialer] and provides a subjectively nicer API.
type Dialer struct {
	td  net.Dialer
	fns setFuncSlice
}

// Dial wraps [net.Dialer.DialContext].
func (d *Dialer) Dial(ctx context.Context, network, address string) (c net.Conn, err error) {
	td := d.td
	td.ControlContext = d.fns.controlContextFunc()
	return td.DialContext(ctx, network, address)
}

// DialerSocketOptions contains dialer-specific socket options.
type DialerSocketOptions struct {
	// Fwmark sets the dialer's fwmark on Linux, or user cookie on FreeBSD.
	//
	// Available on Linux and FreeBSD.
	Fwmark int
}

// Dialer returns a [Dialer] with a control function that sets the socket options.
func (dso DialerSocketOptions) Dialer() Dialer {
	return Dialer{
		fns: dso.buildSetFns(),
	}
}
