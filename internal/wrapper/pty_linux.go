//go:build linux

package wrapper

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	linuxTIOCGPTN   = 0x80045430
	linuxTIOCSPTLCK = 0x40045431
	linuxTIOCGWINSZ = 0x5413
	linuxTIOCSWINSZ = 0x5414
)

type terminalSize struct {
	rows uint16
	cols uint16
	x    uint16
	y    uint16
}

func openPTY() (*os.File, *os.File, error) {
	masterFD, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	master := os.NewFile(uintptr(masterFD), "/dev/ptmx")
	locked := int32(0)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFD), linuxTIOCSPTLCK, uintptr(unsafe.Pointer(&locked))); errno != 0 {
		_ = master.Close()
		return nil, nil, errno
	}
	var number uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFD), linuxTIOCGPTN, uintptr(unsafe.Pointer(&number))); errno != 0 {
		_ = master.Close()
		return nil, nil, errno
	}
	slavePath := fmt.Sprintf("/dev/pts/%d", number)
	slaveFD, err := syscall.Open(slavePath, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, os.NewFile(uintptr(slaveFD), slavePath), nil
}

func resizePTY(master, source *os.File) error {
	var size terminalSize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, source.Fd(), linuxTIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return errno
	}
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), linuxTIOCSWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return errno
	}
	return nil
}
