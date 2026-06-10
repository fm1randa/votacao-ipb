package web

import (
	"net"
	"net/http"

	qrcode "github.com/skip2/go-qrcode"
)

// LanIP descobre o IP desta máquina na LAN, na ordem:
//  1. dial UDP (resolve a rota de saída — cobre rede normal com roteador);
//  2. enumeração de interfaces (fallbackLanIP; cobre hotspot Android, onde não
//     há rota externa — ver lanip_linux.go);
//  3. "localhost" como último recurso.
//
// O override manual (-host) é tratado antes, em Server.baseURL e no main.
func LanIP() string {
	if ip := udpLanIP(); ip != "" {
		return ip
	}
	if ip := fallbackLanIP(); ip != "" {
		return ip
	}
	return "localhost"
}

// udpLanIP não envia pacote; só resolve a rota. Falha (sem rota, ex. hotspot) → "".
func udpLanIP() string {
	conn, err := net.Dial("udp", "192.168.0.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// baseURL é o endereço que os celulares devem abrir. Com -host, usa o valor
// manual; senão detecta a cada chamada (sobrevive a troca de rede no evento).
func (s *Server) baseURL() string {
	host := s.host
	if host == "" {
		host = LanIP()
	}
	return "http://" + host + s.addr
}

// qrPNG serve o QR code da URL base — apontado no telão para acesso pelo celular.
func (s *Server) qrPNG(w http.ResponseWriter, r *http.Request) {
	png, err := qrcode.Encode(s.baseURL()+"/", qrcode.Medium, 512)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache") // IP pode mudar
	w.Write(png)
}
