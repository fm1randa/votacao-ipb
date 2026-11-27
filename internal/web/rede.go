package web

import (
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Assistente de rede (/board/rede): ajuda a mesa a montar a rede local quando
// não há roteador por perto. No macOS, o "hotspot" é o Compartilhamento de
// Internet com Wi-Fi — a Apple não tem API pública para ligá-lo, então o
// assistente só detecta o estado, abre o painel certo nos Ajustes e guia os
// passos. A detecção também corrige o IP anunciado no QR (LanIP).

// HotspotMac é o estado do Compartilhamento de Internet do macOS.
type HotspotMac struct {
	Suportado   bool   // só no macOS
	NAT         bool   // Compartilhamento de Internet ligado
	WiFi        bool   // "Wi-Fi" marcado em "Com computadores que usam"
	Origem      string // "Compartilhar conexão de" (nome legível)
	OrigemAtiva bool   // a origem tem IPv4 — sem isso o macOS não sobe o Wi-Fi
	Rede        string // nome da rede Wi-Fi criada pelo Mac
	Interface   string // bridge do compartilhamento (ex. bridge101)
	IP          string // IP do Mac nessa rede — o que os celulares devem abrir
}

// Ligado diz se os celulares já conseguem entrar na rede do Mac.
func (h HotspotMac) Ligado() bool { return h.IP != "" }

// cache curto: a detecção chama comandos do sistema, e baseURL/LanIP rodam a
// cada render do telão e do QR.
var (
	hotspotMu  sync.Mutex
	hotspotVal HotspotMac
	hotspotExp time.Time
)

func hotspotAtual() HotspotMac {
	hotspotMu.Lock()
	defer hotspotMu.Unlock()
	if time.Now().Before(hotspotExp) {
		return hotspotVal
	}
	hotspotVal = lerHotspotMac()
	hotspotExp = time.Now().Add(2 * time.Second)
	return hotspotVal
}

// hotspotIP devolve o IP do Mac na rede do hotspot, ou "" (desligado / não-macOS).
func hotspotIP() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	return hotspotAtual().IP
}

// parseWiFiDevice extrai o dispositivo Wi-Fi (ex. en0) da saída de
// `networksetup -listallhardwareports`.
func parseWiFiDevice(out string) string {
	achou := false
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "Hardware Port:") {
			porta := strings.TrimSpace(strings.TrimPrefix(l, "Hardware Port:"))
			achou = porta == "Wi-Fi" || porta == "AirPort"
			continue
		}
		if achou && strings.HasPrefix(l, "Device:") {
			return strings.TrimSpace(strings.TrimPrefix(l, "Device:"))
		}
	}
	return ""
}

// parseBridge lê de `ifconfig bridgeN` os membros e o primeiro IPv4.
func parseBridge(out string) (membros []string, ip string) {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "member:":
			membros = append(membros, f[1])
		case "inet":
			if ip == "" {
				ip = f[1]
			}
		}
	}
	return membros, ip
}

// escolherBridge acha a bridge do Compartilhamento de Internet entre as
// bridges da máquina (nome → saída do ifconfig). VMs (Docker, UTM, vmnet)
// também criam bridges — a bridge100 nem sempre é o hotspot. Critério:
//  1. a bridge que tem o Wi-Fi como membro;
//  2. com o compartilhamento ligado, a primeira bridge com IPv4 privado que
//     não seja só de VM (membros vmenet*).
func escolherBridge(bridges map[string]string, ordem []string, wifi string, nat bool) (nome, ip string) {
	for _, n := range ordem {
		membros, addr := parseBridge(bridges[n])
		if addr == "" || wifi == "" {
			continue
		}
		for _, m := range membros {
			if m == wifi {
				return n, addr
			}
		}
	}
	if !nat {
		return "", ""
	}
	for _, n := range ordem {
		membros, addr := parseBridge(bridges[n])
		if p := net.ParseIP(addr); p == nil || !p.IsPrivate() {
			continue
		}
		soVM := len(membros) > 0
		for _, m := range membros {
			if !strings.HasPrefix(m, "vmenet") {
				soVM = false
			}
		}
		if !soVM {
			return n, addr
		}
	}
	return "", ""
}

// IfaceIP é um IPv4 desta máquina (lista de diagnóstico na tela de rede).
type IfaceIP struct{ Nome, IP string }

func ipsLocais() []IfaceIP {
	var out []IfaceIP
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, IfaceIP{ifc.Name, ipn.IP.String()})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *Server) redeDados() map[string]any {
	h := hotspotAtual()
	return map[string]any{
		"Active":   "ajustes",
		"Mac":      runtime.GOOS == "darwin",
		"Hotspot":  h,
		"URL":      s.baseURL(),
		"Override": s.host,
		"IPs":      ipsLocais(),
	}
}

func (s *Server) rede(w http.ResponseWriter, r *http.Request) {
	s.render(w, "rede.html", s.redeDados())
}

// redeEstado: região viva (htmx, a cada 3s) — reflete o hotspot ligar/desligar.
func (s *Server) redeEstado(w http.ResponseWriter, r *http.Request) {
	s.render(w, "redeEstado", s.redeDados())
}

// redeAbrirAjustes abre o painel de Compartilhamento nos Ajustes do Sistema
// **da máquina do servidor** — é nela que o hotspot precisa ser ligado, não no
// aparelho que está com a mesa aberta.
func (s *Server) redeAbrirAjustes(w http.ResponseWriter, r *http.Request) {
	if err := abrirAjustesCompartilhamento(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.actionDone(w, r, "/board/rede", "Ajustes de Compartilhamento abertos no Mac.", false)
}
