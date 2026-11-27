package cli

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func portaLivre(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// O app iOS sobe e derruba o servidor no mesmo processo: Run precisa servir,
// encerrar limpo ao fechar `stop` e poder subir de novo na mesma porta.
func TestRunServeEEncerraAoFecharStop(t *testing.T) {
	dir, addr := t.TempDir(), portaLivre(t)
	for i := 0; i < 2; i++ {
		stop := make(chan struct{})
		fim := make(chan error, 1)
		go func() { fim <- Run([]string{"-addr", addr, "-host", "127.0.0.1", "-data", dir}, stop) }()

		var resp *http.Response
		var err error
		for tent := 0; tent < 50; tent++ {
			if resp, err = http.Get("http://" + addr + "/"); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("rodada %d: servidor não respondeu: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("rodada %d: GET / = %d", i, resp.StatusCode)
		}

		close(stop)
		select {
		case err := <-fim:
			if err != nil {
				t.Fatalf("rodada %d: Run devolveu %v", i, err)
			}
		case <-time.After(6 * time.Second):
			t.Fatalf("rodada %d: Run não encerrou", i)
		}
	}
}

// Porta ocupada vira erro de retorno — nunca log.Fatal, que derrubaria o app.
func TestRunPortaOcupadaDevolveErro(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	err = Run([]string{"-addr", ln.Addr().String(), "-data", t.TempDir()}, make(chan struct{}))
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("esperava erro de listen, veio %v", err)
	}
}
