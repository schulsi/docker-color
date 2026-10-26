//go:build darwin

package wrapper

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

const (
	darwinTIOCPTYUNLK  = 0x20007452
	darwinTIOCPTYGNAME = 0x40807453
	darwinTIOCPTYGRANT = 0x20007454
	darwinTIOCGWINSZ   = 0x40087468
	darwinTIOCSWINSZ   = 0x80087467
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
	for _, request := range []uintptr{darwinTIOCPTYGRANT, darwinTIOCPTYUNLK} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFD), request, 0); errno != 0 {
			_ = master.Close()
			return nil, nil, errno
		}
	}
	name := make([]byte, 128)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFD), darwinTIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		_ = master.Close()
		return nil, nil, errno
	}
	slavePath := string(bytes.TrimRight(name, "\x00"))
	slaveFD, err := syscall.Open(slavePath, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, os.NewFile(uintptr(slaveFD), slavePath), nil
}

func resizePTY(master, source *os.File) error {
	var size terminalSize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, source.Fd(), darwinTIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return errno
	}
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), darwinTIOCSWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return errno
	}
	return nil
}
