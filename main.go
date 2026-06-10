// Sistema de votação offline para Congresso de Federação UMP (IPB).
//
// Binário único: serve a app web e fala com um SQLite local (WAL).
// Roda numa LAN sem internet — notebook da mesa + roteador de viagem.
//
//	go run . -seed                    # cria dados de exemplo + tokens
//	go run .                          # sobe em http://0.0.0.0:8080
//	go run . -addr :9000 -pin 4729    # porta e PIN da mesa custom
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"votacao-ipb/internal/store"
	"votacao-ipb/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "endereço de escuta")
	host := flag.String("host", "", "IP/host anunciado no QR, telão e logs (vazio = autodetecção; útil em hotspot)")
	dbPath := flag.String("db", "votacao.db", "caminho do arquivo SQLite")
	pin := flag.String("pin", "", "define/troca o PIN da mesa (opcional; senão, define-se na 1ª vez em /board)")
	seed := flag.Bool("seed", false, "popula dados de exemplo e sai")
	tokens := flag.Int("tokens", 200, "qtd de tokens a gerar no -seed")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("abrir store: %v", err)
	}
	defer st.Close()

	if *seed {
		if err := semear(st, *tokens); err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Printf("seed pronto em %s (%d tokens). Suba com: go run .", *dbPath, *tokens)
		return
	}

	// -pin opcional: define/troca o PIN salvo. Senão, a Mesa define na 1ª vez via UI.
	if *pin != "" {
		if err := st.SetPIN(context.Background(), *pin); err != nil {
			log.Fatalf("definir PIN: %v", err)
		}
	}
	pinDefinido, _ := st.PINHash(context.Background())

	srv, err := web.New(st, *addr, *host)
	if err != nil {
		log.Fatalf("web: %v", err)
	}
	httpSrv := &http.Server{
		Addr: *addr, Handler: srv.Routes(),
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second,
	}

	// Endereço anunciado: -host manda; senão autodetecção (dial UDP → interfaces).
	announce := *host
	if announce == "" {
		announce = web.LanIP()
	}
	go func() {
		log.Printf("votação no ar em http://%s%s", announce, *addr)
		log.Printf("  eleitores: abra esse endereço no celular (mesma rede WiFi)")
		if announce == "localhost" {
			log.Printf("  AVISO: IP da rede não detectado — rode com -host=<IP> (ex. hotspot)")
		}
		if pinDefinido == "" {
			log.Printf("  mesa:      http://%s%s/board   (defina o PIN da mesa na 1ª vez)", announce, *addr)
		} else {
			log.Printf("  mesa:      http://%s%s/board   (PIN da mesa já definido)", announce, *addr)
		}
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("encerrando...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}

// semear cria um congresso de exemplo (âmbito Federação/UMP): UMPs locais,
// delegados, o preset de cargos (Art. 26a) e a pilha de tokens.
func semear(st *store.Store, n int) error {
	ctx := context.Background()
	cong, err := st.CreateCongress(ctx, store.AmbitoFederacao, "UMP", "Federação UMP de Exemplo", 2026)
	if err != nil {
		return err
	}

	locais := map[string]int64{}
	for _, nome := range []string{"1ª IP Central", "IP do Jardim", "IP Betel"} {
		id, err := st.AddLocal(ctx, cong, nome, 0)
		if err != nil {
			return err
		}
		locais[nome] = id
	}

	delegados := []struct {
		nome, local, nasc string
		nato              bool
	}{
		{"Ana Lúcia Ferreira", "1ª IP Central", "1995-03-12", false},
		{"Bruno Carvalho", "1ª IP Central", "1992-07-01", false},
		{"Caio Menezes", "IP do Jardim", "1998-11-23", false},
		{"Daniela Rocha", "IP do Jardim", "1990-01-30", false},
		{"Eduardo Pinto", "IP Betel", "1997-05-09", false},
		{"Fernanda Lima", "IP Betel", "1993-09-17", false},
		{"Rev. Secretário Presbiterial", "", "1975-02-02", true}, // membro nato
	}
	for _, d := range delegados {
		var localID *int64
		if !d.nato {
			id := locais[d.local]
			localID = &id
		}
		if _, err := st.AddElector(ctx, cong, d.nome, localID, nil, d.nato, d.nasc); err != nil {
			return err
		}
	}

	for i, p := range store.PresetPositions(store.AmbitoFederacao, "UMP") {
		if err := st.AddPosition(ctx, cong, p.Nome, p.Role, i+1); err != nil {
			return err
		}
	}

	return st.GenerateTokens(ctx, cong, n)
}
