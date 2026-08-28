//go:build !freebsd && !linux

package conn

func (dso DialerSocketOptions) buildSetFns() setFuncSlice {
	return setFuncSlice{}
}
