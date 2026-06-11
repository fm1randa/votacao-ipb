package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"votacao-ipb/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := store.OpenElections(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := mgr.SetActive("t.db"); err != nil {
		t.Fatal(err)
	}
	srv, err := New(mgr, st, ":0", "")
	if err != nil {
		t.Fatal(err)
	}
	return srv, st
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// abreRoundHTTP monta uma Plenária local pronta para votar via os métodos
// exportados do store e devolve o round aberto e um token entregue. Espelha o
// helper abreRound do pacote store (não acessível daqui).
func abreRoundHTTP(t *testing.T, st *store.Store) (round int64, token string) {
	t.Helper()
	ctx := context.Background()
	cong, err := st.SetupCongress(ctx, store.AmbitoLocal, "UMP", "Plenária Teste", 2026, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		id, err := st.AddElector(ctx, cong, fmt.Sprintf("Sócio %d", i+1), nil, nil, false, "1999-01-01")
		if err != nil {
			t.Fatal(err)
		}
		code, err := st.Credenciar(ctx, cong, id)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			token = code
		}
	}
	if err := st.DeclararAbertura(ctx, cong); err != nil {
		t.Fatal(err)
	}
	pos, err := st.Positions(ctx, cong)
	if err != nil || len(pos) == 0 {
		t.Fatalf("sem cargos: %v", err)
	}
	r, err := st.AbrirCargo(ctx, cong, pos[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r.ID, token
}

// --- Auth da Mesa (PIN) ----------------------------------------------------

func TestBoardLoginExigePIN(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := noRedirectClient().Get(ts.URL + "/board")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("esperava 303, veio %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/board/setup" {
		t.Fatalf("esperava redirect para /board/setup, veio %q", loc)
	}
}

func TestBoardLoginPINErrado(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.SetPIN(context.Background(), "4729"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := noRedirectClient().PostForm(ts.URL+"/board/login", url.Values{"pin": {"0000"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("esperava 303, veio %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/board/login?e=1" {
		t.Fatalf("PIN errado: esperava /board/login?e=1, veio %q", loc)
	}
}

func TestBoardLoginPINCerto(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.SetPIN(context.Background(), "4729"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := noRedirectClient().PostForm(ts.URL+"/board/login", url.Values{"pin": {"4729"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("esperava 303, veio %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/board" {
		t.Fatalf("PIN certo: esperava /board, veio %q", loc)
	}
	var temCookieMesa bool
	for _, c := range resp.Cookies() {
		if c.Name == "mesa" {
			temCookieMesa = true
		}
	}
	if !temCookieMesa {
		t.Fatal("PIN certo deveria setar o cookie mesa")
	}
}

// --- Delegado + fluxo do voto ---------------------------------------------

func TestDelegadoLoginTokenInvalido(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := noRedirectClient().PostForm(ts.URL+"/delegado/login", url.Values{"token": {"ZZZZ"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("esperava 303, veio %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/delegado/login?e=1" {
		t.Fatalf("token inválido: esperava /delegado/login?e=1, veio %q", loc)
	}
}

func TestVotoPeloHTTP(t *testing.T) {
	ctx := context.Background()
	srv, st := newTestServer(t)
	round, token := abreRoundHTTP(t, st)

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	post := func() *http.Response {
		t.Helper()
		body := strings.NewReader(url.Values{
			"round_id": {strconv.FormatInt(round, 10)},
			"choice":   {"branco"},
		}.Encode())
		req, err := http.NewRequest("POST", ts.URL+"/vote", body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "delegado", Value: token})
		resp, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// 1º voto: sucesso → 303 para /delegado, e o voto fica gravado.
	resp := post()
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/delegado" {
		t.Fatalf("1º voto: esperava 303 -> /delegado, veio %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if ok, err := st.HasVoted(ctx, round, token); err != nil || !ok {
		t.Fatalf("voto não foi gravado: ok=%v err=%v", ok, err)
	}

	// 2º voto com o mesmo token: queima da UNIQUE(round_id, token). O handler
	// trata ErrAlreadyVoted como sucesso (303 -> /delegado), mas o voto NÃO
	// pode contar duas vezes.
	resp = post()
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/delegado" {
		t.Fatalf("2º voto: esperava 303 -> /delegado, veio %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	res, err := st.Tally(ctx, round)
	if err != nil {
		t.Fatal(err)
	}
	if res.Depositados != 1 {
		t.Fatalf("token não foi queimado: Depositados=%d, esperava 1", res.Depositados)
	}
}

// --- Versão no rodapé da Mesa -----------------------------------------------

func TestVersaoNoRodapeDaMesa(t *testing.T) {
	// Garante que a variável global não vaze entre testes.
	old := Version
	defer func() { Version = old }()
	Version = "v0.teste"

	srv, st := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	if err := st.SetPIN(context.Background(), "4729"); err != nil {
		t.Fatal(err)
	}

	// Faz login na Mesa para obter o cookie "mesa".
	jar := &cookieJar{}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.PostForm(ts.URL+"/board/login", url.Values{"pin": {"4729"}})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: esperava 303, veio %d", resp.StatusCode)
	}

	// Seguir o redirect manualmente para /board.
	req, err := http.NewRequest("GET", ts.URL+"/board", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Copia o cookie "mesa" para a requisição seguinte.
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	// Sem congresso, /board redireciona para o setup; configuramos um para
	// que a página da Mesa (template mesanav) seja renderizada.
	ctx := context.Background()
	cong, err := st.SetupCongress(ctx, store.AmbitoLocal, "UMP", "Plenária Teste", 2026, nil)
	if err != nil {
		t.Fatalf("setup congresso: %v", err)
	}
	_ = cong

	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /board: %v", err)
	}
	defer resp2.Body.Close()
	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("ler body: %v", err)
	}
	if !strings.Contains(string(body), "v0.teste") {
		t.Fatalf("versão %q não encontrada no rodapé da Mesa;\nbody:\n%s", "v0.teste", body)
	}
}

// cookieJar é um jar mínimo para guardar cookies entre requisições nos testes.
type cookieJar struct {
	cookies []*http.Cookie
}

func (j *cookieJar) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	j.cookies = append(j.cookies, cookies...)
}

func (j *cookieJar) Cookies(_ *url.URL) []*http.Cookie {
	return j.cookies
}
