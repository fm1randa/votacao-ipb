package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"votacao-ipb/internal/store"
)

const saidaNetworksetup = `
Hardware Port: Thunderbolt Bridge
Device: bridge0
Ethernet Address: 36:2c:59:1a:2b:40

Hardware Port: Wi-Fi
Device: en0
Ethernet Address: a4:fc:14:00:00:01
`

const bridgeVM = `bridge100: flags=8a63<UP,BROADCAST,SMART,RUNNING,ALLMULTI,SIMPLEX,MULTICAST> mtu 1500
	inet 192.168.64.1 netmask 0xffffff00 broadcast 192.168.64.255
	member: vmenet0 flags=20803<LEARNING,DISCOVER,PRIVATE,VIRTIO>
`

const bridgeHotspot = `bridge101: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	inet 192.168.2.1 netmask 0xffffff00 broadcast 192.168.2.255
	member: en0 flags=3<LEARNING,DISCOVER>
`

func TestParseWiFiDevice(t *testing.T) {
	if got := parseWiFiDevice(saidaNetworksetup); got != "en0" {
		t.Fatalf("esperava en0, veio %q", got)
	}
	if got := parseWiFiDevice("Hardware Port: Ethernet\nDevice: en5\n"); got != "" {
		t.Fatalf("sem Wi-Fi, esperava vazio, veio %q", got)
	}
}

func TestHotspotIgnoraBridgeDeVM(t *testing.T) {
	bridges := map[string]string{"bridge100": bridgeVM, "bridge101": bridgeHotspot}
	ordem := []string{"bridge100", "bridge101"}
	nome, ip := escolherBridge(bridges, ordem, "en0", true)
	if nome != "bridge101" || ip != "192.168.2.1" {
		t.Fatalf("esperava bridge101/192.168.2.1, veio %s/%s", nome, ip)
	}
}

func TestHotspotDesligadoSoComVM(t *testing.T) {
	bridges := map[string]string{"bridge100": bridgeVM}
	for _, nat := range []bool{false, true} {
		if nome, ip := escolherBridge(bridges, []string{"bridge100"}, "en0", nat); nome != "" || ip != "" {
			t.Fatalf("nat=%v: só VM não é hotspot, veio %s/%s", nat, nome, ip)
		}
	}
}

func TestHotspotSemWiFiComoMembroUsaNAT(t *testing.T) {
	// Fallback: compartilhamento ligado e a bridge não lista o Wi-Fi.
	semMembro := "bridge101: flags=8863<UP>\n\tinet 192.168.2.1 netmask 0xffffff00\n"
	bridges := map[string]string{"bridge100": bridgeVM, "bridge101": semMembro}
	ordem := []string{"bridge100", "bridge101"}
	if _, ip := escolherBridge(bridges, ordem, "en0", true); ip != "192.168.2.1" {
		t.Fatalf("com NAT ligado esperava 192.168.2.1, veio %q", ip)
	}
	if _, ip := escolherBridge(bridges, ordem, "en0", false); ip != "" {
		t.Fatalf("com NAT desligado esperava vazio, veio %q", ip)
	}
}

func TestTelaDeRedeExigeMesaERenderiza(t *testing.T) {
	srv, st := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	ctx := context.Background()
	if err := st.SetPIN(ctx, "4729"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetupCongress(ctx, store.AmbitoLocal, "UMP", "Plenária Teste", 2026, nil); err != nil {
		t.Fatal(err)
	}
	client := noRedirectClient()

	resp, err := client.Get(ts.URL + "/board/rede")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sem login esperava 303, veio %d", resp.StatusCode)
	}

	resp, err = client.PostForm(ts.URL+"/board/login", url.Values{"pin": {"4729"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/board/rede", nil)
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "Endereço de acesso") {
		t.Fatalf("página de rede sem o endereço de acesso")
	}
}
