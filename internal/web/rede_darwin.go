//go:build darwin

package web

import (
	"net"
	"os/exec"
	"strings"
)

const natPlist = "/Library/Preferences/SystemConfiguration/com.apple.nat.plist"

// lerHotspotMac consulta o sistema: config do Compartilhamento de Internet
// (com.apple.nat.plist, legível sem root), o dispositivo Wi-Fi e as bridges.
func lerHotspotMac() HotspotMac {
	h := HotspotMac{Suportado: true}
	h.NAT = plistRaw("NAT.Enabled") == "1"
	h.WiFi = plistRaw("NAT.AirPort.Enabled") == "1"
	h.Rede = plistRaw("NAT.AirPort.NetworkName")

	out, _ := exec.Command("networksetup", "-listallhardwareports").Output()
	wifi := parseWiFiDevice(string(out))

	bridges := map[string]string{}
	var ordem []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if !strings.HasPrefix(ifc.Name, "bridge") || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		o, err := exec.Command("ifconfig", ifc.Name).Output()
		if err != nil {
			continue
		}
		bridges[ifc.Name] = string(o)
		ordem = append(ordem, ifc.Name)
	}
	h.Interface, h.IP = escolherBridge(bridges, ordem, wifi, h.NAT)
	return h
}

func plistRaw(chave string) string {
	out, err := exec.Command("plutil", "-extract", chave, "raw", "-o", "-", natPlist).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func abrirAjustesCompartilhamento() error {
	return exec.Command("open", "x-apple.systempreferences:com.apple.Sharing-Settings.extension").Run()
}
