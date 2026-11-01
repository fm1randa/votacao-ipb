// Package cli é o ponto de entrada do binário único: lê as flags, abre a pasta
// de dados e serve a app web até receber o sinal de parada.
//
// Mora fora do package main para que as cascas nativas reaproveitem o MESMO
// bootstrap: o binário de linha de comando (main.go) e o app iOS, que não pode
// dar exec() num processo filho e roda o servidor dentro do próprio processo
// (ios/gobridge, ADR-0015). Por isso nada aqui chama log.Fatal — erro volta
// para quem chamou, em vez de derrubar o app hospedeiro.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"votacao-ipb/internal/store"
	"votacao-ipb/internal/web"
)

// Run executa o servidor com os argumentos de linha de comando (sem o nome do
// programa) e bloqueia até `stop` fechar ou o servidor falhar. Com -seed,
// popula o exemplo e retorna sem servir.
func Run(args []string, stop <-chan struct{}) error {
	fs := flag.NewFlagSet("votacao", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "endereço de escuta")
	host := fs.String("host", "", "IP/host anunciado no QR, telão e logs (vazio = autodetecção; útil em hotspot)")
	data := fs.String("data", "", "pasta de dados — um arquivo .db por eleição (vazio = pasta do -db)")
	dbPath := fs.String("db", "votacao.db", "eleição inicial (atalho; o gerenciador em /board/eleicoes troca a quente)")
	pin := fs.String("pin", "", "define/troca o PIN da mesa (opcional; senão, define-se na 1ª vez em /board)")
	seed := fs.Bool("seed", false, "popula dados de exemplo e sai")
	tokens := fs.Int("tokens", 200, "qtd de tokens a gerar no -seed")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Pasta de dados (ADR-0012): -data manda; senão, a pasta do -db. A eleição
	// ativa vem do -db explícito > meta da pasta > votacao.db.
	dir, file := *data, filepath.Base(*dbPath)
	if dir == "" {
		dir = filepath.Dir(*dbPath)
	}
	mgr, err := store.OpenElections(dir)
	if err != nil {
		return fmt.Errorf("pasta de dados: %w", err)
	}
	dbExplicit := false
	fs.Visit(func(f *flag.Flag) { dbExplicit = dbExplicit || f.Name == "db" })
	active := mgr.Active()
	if dbExplicit || active == "" {
		if file == "" {
			file = "votacao.db"
		}
		active = file
	}
	activePath, err := mgr.Path(active)
	if err != nil {
		return fmt.Errorf("eleição ativa: %w", err)
	}
	st, err := store.Open(activePath)
	if err != nil {
		return fmt.Errorf("abrir store: %w", err)
	}
	defer st.Close()
	if err := mgr.SetActive(active); err != nil {
		return fmt.Errorf("eleição ativa: %w", err)
	}

	if *seed {
		if err := semear(st, *tokens); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		log.Printf("seed pronto em %s (%d tokens). Suba com: go run .", activePath, *tokens)
		return nil
	}

	// -pin opcional: define/troca o PIN salvo. Senão, a Mesa define na 1ª vez via UI.
	if *pin != "" {
		if err := st.SetPIN(context.Background(), *pin); err != nil {
			return fmt.Errorf("definir PIN: %w", err)
		}
	}
	pinDefinido, _ := st.PINHash(context.Background())

	srv, err := web.New(mgr, st, *addr, *host)
	if err != nil {
		return fmt.Errorf("web: %w", err)
	}
	httpSrv := &http.Server{
		Addr: *addr, Handler: srv.Routes(),
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second,
	}
	// Escuta antes de anunciar: porta ocupada vira erro de retorno, não Fatal.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Endereço anunciado: -host manda; senão autodetecção (dial UDP → interfaces).
	announce := *host
	if announce == "" {
		announce = web.LanIP()
	}
	log.Printf("votação %s no ar em http://%s%s", web.Version, announce, *addr)
	log.Printf("  eleitores: abra esse endereço no celular (mesma rede WiFi)")
	if announce == "localhost" {
		log.Printf("  AVISO: IP da rede não detectado — rode com -host=<IP> (ex. hotspot)")
	}
	if pinDefinido == "" {
		log.Printf("  mesa:      http://%s%s/board   (defina o PIN da mesa na 1ª vez)", announce, *addr)
	} else {
		log.Printf("  mesa:      http://%s%s/board   (PIN da mesa já definido)", announce, *addr)
	}

	served := make(chan error, 1)
	go func() { served <- httpSrv.Serve(ln) }()

	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-stop:
	}
	log.Println("encerrando...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

// semear cria um congresso de exemplo (âmbito Federação/UMP): UMPs locais,
// delegados, o preset de cargos (Art. 26a) e a pilha de tokens.
func semear(st *store.Store, n int) error {
	ctx := context.Background()
	// O nome guarda só a entidade-mãe: exibe-se "Federação de UMPs do <nome>".
	cong, err := st.CreateCongress(ctx, store.AmbitoFederacao, "UMP", "Presbitério de Exemplo", 2026)
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
