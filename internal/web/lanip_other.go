//go:build !linux

package web

import "net"

// fallbackLanIP fora do Linux/Android: o netlink não é problema, então a API
// padrão resolve. Mesmo critério: primeiro IPv4 privado de interface ativa
// não-loopback; sem privado, o primeiro não-loopback; senão "".
func fallbackLanIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var primeiro string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			if ip4.IsPrivate() {
				return ip4.String()
			}
			if primeiro == "" {
				primeiro = ip4.String()
			}
		}
	}
	return primeiro
}
