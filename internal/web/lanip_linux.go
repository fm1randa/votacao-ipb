//go:build linux

package web

import (
	"net"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fallbackLanIP enumera os IPv4 das interfaces via ioctl SIOCGIFCONF — o mesmo
// mecanismo do ifconfig. É o caminho que funciona no Android/Termux: o Android
// 13+ bloqueia netlink para apps comuns (net.Interfaces falha com EPERM), e no
// modo hotspot o dial UDP não tem rota (e a subnet é randomizada, então sondar
// faixas fixas também não serve). Devolve o primeiro IPv4 privado de interface
// ativa não-loopback; sem privado, o primeiro não-loopback; senão "".
func fallbackLanIP() string {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return ""
	}
	defer unix.Close(fd)

	const sizeofIfreq = 40 // 16 (nome) + 24 (union) em 64 bits
	buf := make([]byte, 64*sizeofIfreq)
	ifc := struct {
		length int32
		_      int32
		ptr    unsafe.Pointer
	}{length: int32(len(buf)), ptr: unsafe.Pointer(&buf[0])}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.SIOCGIFCONF,
		uintptr(unsafe.Pointer(&ifc))); errno != 0 {
		return ""
	}

	var primeiro string
	for off := 0; off+sizeofIfreq <= int(ifc.length); off += sizeofIfreq {
		entry := buf[off : off+sizeofIfreq]
		name := string(entry[:16])
		if i := strings.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		if entry[16] != unix.AF_INET { // sa_family (little-endian, byte baixo)
			continue
		}
		ip := net.IP(entry[20:24]) // sockaddr_in: family(2) port(2) addr(4)
		if ip.IsLoopback() {
			continue
		}
		ifr, err := unix.NewIfreq(name)
		if err != nil {
			continue
		}
		if err := unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, ifr); err != nil {
			continue
		}
		if ifr.Uint16()&unix.IFF_UP == 0 {
			continue
		}
		if ip.IsPrivate() {
			return ip.String()
		}
		if primeiro == "" {
			primeiro = ip.String()
		}
	}
	return primeiro
}
