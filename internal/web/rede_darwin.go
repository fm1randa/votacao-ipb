//go:build darwin

package web

import (
	"encoding/json"
	"net"
	"os/exec"
	"slices"
	"strings"
)

const natPlist = "/Library/Preferences/SystemConfiguration/com.apple.nat.plist"

// lerHotspotMac consulta o sistema: config do Compartilhamento de Internet
// (com.apple.nat.plist, legível sem root), o dispositivo Wi-Fi e as bridges.
func lerHotspotMac() HotspotMac {
	h := HotspotMac{Suportado: true}
	h.NAT = plistRaw("NAT.Enabled") == "1"
	h.Rede = plistRaw("NAT.AirPort.NetworkName")
	h.Origem = plistRaw("NAT.PrimaryInterface.PrimaryUserReadable")
	h.OrigemAtiva = temIPv4(plistRaw("NAT.PrimaryInterface.Device"))

	out, _ := exec.Command("networksetup", "-listallhardwareports").Output()
	wifi := parseWiFiDevice(string(out))
	// No macOS atual o Wi-Fi marcado vai para NAT.SharingDevices; o antigo
	// NAT.AirPort.Enabled pode ficar 0 mesmo com o Wi-Fi marcado.
	var compartilhados []string
	if j, err := exec.Command("plutil", "-extract", "NAT.SharingDevices", "json", "-o", "-", natPlist).Output(); err == nil {
		json.Unmarshal(j, &compartilhados)
	}
	h.WiFi = plistRaw("NAT.AirPort.Enabled") == "1" || (wifi != "" && slices.Contains(compartilhados, wifi))

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

func temIPv4(dev string) bool {
	if dev == "" {
		return false
	}
	ifc, err := net.InterfaceByName(dev)
	if err != nil {
		return false
	}
	addrs, _ := ifc.Addrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return true
		}
	}
	return false
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
