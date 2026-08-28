package conn

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func setFwmark(fd, fwmark int) error {
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_MARK, fwmark); err != nil {
		return fmt.Errorf("failed to set socket option SO_MARK: %w", err)
	}
	return nil
}

func (dso DialerSocketOptions) buildSetFns() setFuncSlice {
	return setFuncSlice{}.
		appendSetFwmarkFunc(dso.Fwmark)
}
