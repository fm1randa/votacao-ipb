// Package web expõe os handlers HTTP e os templates embutidos.
package web

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"votacao-ipb/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed assets
var assetsFS embed.FS

type Server struct {
	mgr  *store.Elections            // pasta de dados: um .db por Eleição (ADR-0012)
	cur  atomic.Pointer[store.Store] // a Eleição ativa — trocada a quente pelo gerenciador
	tpl  *template.Template
	hub  *Hub
	addr string // porta de escuta (ex. ":8090") — usada no QR/baseURL
	host string // override do -host: IP/host anunciado (QR, telão); "" = autodetectar

	congCache atomic.Value // congEntry — alimenta os termos por âmbito (TTL curto)
}

// db devolve o Store da Eleição ativa. Handlers em voo seguram o ponteiro que
// leram — por isso o switchTo fecha o banco antigo com atraso, não na hora.
func (s *Server) db() *store.Store { return s.cur.Load() }

// switchTo abre a Eleição `file` e a torna ativa, sem derrubar o servidor.
// O Broadcast acorda as telas vivas (telão/board/delegado re-buscam fragmentos).
func (s *Server) switchTo(file string) error {
	p, err := s.mgr.Path(file)
	if err != nil {
		return err
	}
	st, err := store.Open(p)
	if err != nil {
		return err
	}
	if err := s.mgr.SetActive(file); err != nil {
		st.Close()
		return err
	}
	old := s.cur.Swap(st)
	s.congCache.Store(congEntry{}) // expira o vocabulário por âmbito
	if old != nil && old != st {
		time.AfterFunc(10*time.Second, func() { old.Close() }) // espera handlers em voo
	}
	s.hub.Broadcast()
	return nil
}

// congEntry guarda o congresso em cache para o vocabulário por âmbito (SPEC §10):
// os templates consultam `term`/`ambito` muitas vezes por render; o TTL de 1s
// evita marteladas no SQLite e ainda reflete mudanças (Ajustes/restore) na hora.
type congEntry struct {
	c   store.Congress
	exp time.Time
}

// cong devolve o congresso atual (ou um default Federação/UMP antes do setup).
func (s *Server) cong() store.Congress {
	if v := s.congCache.Load(); v != nil {
		if e := v.(congEntry); time.Now().Before(e.exp) {
			return e.c
		}
	}
	c, err := s.db().FirstCongress(context.Background())
	if err != nil {
		c = store.Congress{Ambito: store.AmbitoFederacao, Sociedade: "UMP"}
	}
	s.congCache.Store(congEntry{c, time.Now().Add(time.Second)})
	return c
}

// term traduz o vocabulário da UI conforme âmbito e sociedade (CONTEXT.md):
// local fala "Sócio/Plenária/Chamada"; federados, "Delegado/Congresso/Credenciar";
// a SAF flexiona no feminino (Sócia, Delegada).
func (s *Server) term(key string) string {
	c := s.cong()
	local := c.Ambito == store.AmbitoLocal
	fem := c.Sociedade == "SAF"
	votante := map[bool]map[bool]string{
		true:  {true: "Sócia", false: "Sócio"},
		false: {true: "Delegada", false: "Delegado"},
	}[local][fem]
	unidade, unidades := "", ""
	switch c.Ambito {
	case store.AmbitoFederacao:
		unidade, unidades = "UMP local", "UMPs locais"
		if c.Sociedade != "UMP" {
			unidade, unidades = c.Sociedade+" local", c.Sociedade+"s locais"
		}
	case store.AmbitoSinodal:
		unidade, unidades = "Federação", "Federações"
	case store.AmbitoNacional:
		unidade, unidades = "Sinodal", "Sinodais"
	}
	switch key {
	case "Votante":
		return votante
	case "Votantes":
		return votante + "s"
	case "votante":
		return strings.ToLower(votante)
	case "votantes":
		return strings.ToLower(votante) + "s"
	case "Evento":
		if local {
			return "Plenária"
		}
		return "Congresso"
	case "EventoEleicao": // subtítulo sob o nome da entidade ("• ano" vem depois)
		if local {
			return "Plenária de eleição" // a Plenária comum não é de eleição
		}
		return "Congresso"
	case "Credenciar": // rótulo da aba/página
		if local {
			return "Chamada"
		}
		return "Credenciamento"
	case "credenciar_btn": // botão da linha do rol
		if local {
			return "Chamar"
		}
		return "Credenciar"
	case "aba_credenciar": // rótulo curto da navegação
		if local {
			return "Chamada"
		}
		return "Credenciar"
	case "unidade":
		return unidade
	case "unidades":
		return unidades
	case "subunidade":
		return "Federação"
	case "subunidades":
		return "Federações"
	}
	return key
}

// entidadeNome monta o nome de exibição por âmbito (SPEC §10.1). O campo
// `nome` guarda só a entidade-mãe (igreja, presbitério ou sínodo); a Nacional
// é única e não tem nome:
//   local      → "UMP da IPB Cordovil"
//   federacao  → "Federação de UMPs do PRNT"
//   sinodal    → "Sinodal de UMPs do sínodo SNT"
//   nacional   → "Nacional de UMPs"
func entidadeNome(ambito, sociedade, nome string) string {
	switch ambito {
	case store.AmbitoLocal:
		return strings.TrimSpace(sociedade + " da " + nome)
	case store.AmbitoSinodal:
		return strings.TrimSpace("Sinodal de " + sociedade + "s do sínodo " + nome)
	case store.AmbitoNacional:
		return "Nacional de " + sociedade + "s"
	default: // federação
		return strings.TrimSpace("Federação de " + sociedade + "s do " + nome)
	}
}

func New(mgr *store.Elections, st *store.Store, addr, host string) (*Server, error) {
	s := &Server{mgr: mgr, hub: newHub(), addr: addr, host: host}
	s.cur.Store(st)
	funcs := template.FuncMap{
		"term":      s.term,
		"ambito":    func() string { return s.cong().Ambito },
		"sociedade": func() string { return s.cong().Sociedade },
		// entidade: nome de exibição da eleição ativa ("UMP da IPB Cordovil").
		// entidadeDe é a variante para listas (gerenciador), onde cada item
		// tem o próprio âmbito e sociedade.
		"entidade":   func() string { c := s.cong(); return entidadeNome(c.Ambito, c.Sociedade, c.Nome) },
		"entidadeDe": entidadeNome,
		// dataBR: exibe data ISO (banco) como DD/MM/AAAA.
		"dataBR": func(iso string) string {
			if t, err := time.Parse("2006-01-02", iso); err == nil {
				return t.Format("02/01/2006")
			}
			return iso
		},
		"pct": func(v, total int) int {
			if total <= 0 {
				return 0
			}
			return v * 100 / total
		},
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[kv[i].(string)] = kv[i+1]
			}
			return m
		},
		"credStatus": func(e store.Elector) string {
			if !e.Credenciado {
				return "pendente"
			}
			if e.Presente {
				return "presente"
			}
			return "ausente"
		},
		// baseURL: endereço de acesso pelo celular (mostrado junto ao QR do telão).
		"baseURL": s.baseURL,
	}
	tpl, err := template.New("").Funcs(funcs).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s.tpl = tpl
	return s, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Home (seletor de papel)
	mux.HandleFunc("GET /{$}", s.home)
	// Delegado: login com token (1x) → hub com estados → voto sem campo de código
	mux.HandleFunc("GET /delegado/login", s.delegadoLoginForm)
	mux.HandleFunc("POST /delegado/login", s.delegadoLoginSubmit)
	mux.HandleFunc("POST /delegado/logout", s.delegadoLogout)
	mux.HandleFunc("GET /delegado", s.delegado)
	mux.HandleFunc("GET /delegado/fragment", s.delegadoFragment)
	mux.HandleFunc("GET /vote", s.voteForm)
	mux.HandleFunc("POST /vote", s.mut(s.voteSubmit))
	// Mesa — onboarding (wizard de 3 passos) e configuração
	mux.HandleFunc("GET /board/setup", s.setupWizard)
	mux.HandleFunc("POST /board/setup", s.setupSubmit)
	mux.HandleFunc("GET /board/setup/cargos", s.authPIN(s.setupCargosFragment))
	mux.HandleFunc("POST /board/setup/congresso", s.authPIN(s.setupCongresso))
	mux.HandleFunc("POST /board/delegados/add", s.authPIN(s.mut(s.delegadoAdd)))
	mux.HandleFunc("POST /board/delegados/import", s.authPIN(s.mut(s.delegadoImport)))
	mux.HandleFunc("POST /board/delegados/update", s.authPIN(s.mut(s.delegadoUpdate)))
	mux.HandleFunc("POST /board/delegados/delete", s.authPIN(s.mut(s.delegadoDelete)))
	mux.HandleFunc("GET /board/ajustes", s.auth(s.ajustes))
	mux.HandleFunc("GET /board/ajustes/zona", s.auth(s.ajustesZona))
	mux.HandleFunc("POST /board/ajustes", s.auth(s.mut(s.ajustesSave)))
	mux.HandleFunc("POST /board/ajustes/cargos", s.auth(s.mut(s.ajustesCargos)))
	mux.HandleFunc("GET /board/login", s.loginForm)
	mux.HandleFunc("POST /board/login", s.loginSubmit)
	mux.HandleFunc("GET /board", s.auth(s.board))
	mux.HandleFunc("GET /board/fragment", s.auth(s.boardFragment))
	mux.HandleFunc("GET /board/credenciamento", s.auth(s.credenciamento))
	// Histórico usa authPIN (não auth): depois de um Reset não há congresso,
	// mas o Desfazer precisa continuar acessível para reverter o reset.
	mux.HandleFunc("GET /board/historico", s.authPIN(s.historico))
	mux.HandleFunc("GET /board/historico/fragment", s.authPIN(s.historicoFragment))
	mux.HandleFunc("GET /board/restore/preview", s.authPIN(s.restorePreview))
	mux.HandleFunc("POST /board/undo", s.authPIN(s.mut(s.undo)))
	mux.HandleFunc("POST /board/restore", s.authPIN(s.mut(s.restore)))
	// Gerenciador de Eleições (ADR-0012): listar, criar, abrir, resetar, excluir.
	mux.HandleFunc("GET /board/eleicoes", s.authPIN(s.eleicoes))
	mux.HandleFunc("POST /board/eleicoes/criar", s.authPIN(s.mut(s.eleicaoCriar)))
	mux.HandleFunc("POST /board/eleicoes/abrir", s.authPIN(s.mut(s.eleicaoAbrir)))
	mux.HandleFunc("POST /board/eleicoes/resetar", s.authPIN(s.mut(s.eleicaoResetar)))
	mux.HandleFunc("POST /board/eleicoes/excluir", s.authPIN(s.mut(s.eleicaoExcluir)))
	mux.HandleFunc("POST /board/eleicao/reiniciar", s.auth(s.mut(s.reiniciar)))
	mux.HandleFunc("POST /board/eleicao/encerrar", s.auth(s.mut(s.encerrarEleicao)))
	mux.HandleFunc("POST /board/eleicao/reabrir", s.auth(s.mut(s.reabrirEleicao)))
	mux.HandleFunc("POST /board/credenciar", s.auth(s.mut(s.credenciar)))
	mux.HandleFunc("POST /board/reissue", s.auth(s.mut(s.reissue)))
	mux.HandleFunc("POST /board/presenca", s.auth(s.mut(s.presenca)))
	mux.HandleFunc("POST /board/abertura", s.auth(s.mut(s.declararAbertura)))
	mux.HandleFunc("GET /board/cargo/indicar", s.auth(s.indicarForm))
	mux.HandleFunc("POST /board/cargo/abrir", s.auth(s.mut(s.abrirCargo)))
	mux.HandleFunc("POST /board/escrutinio/encerrar", s.auth(s.mut(s.encerrar)))
	mux.HandleFunc("POST /board/escrutinio/proximo", s.auth(s.mut(s.proximo)))
	mux.HandleFunc("GET /report", s.auth(s.report))
	// Telão: /screen é o endereço fixo (acompanha o escrutínio da vez);
	// /screen/{id} continua para um escrutínio específico.
	mux.HandleFunc("GET /screen", s.screenCurrent)
	mux.HandleFunc("GET /screen/fragment", s.screenCurrentFragment)
	mux.HandleFunc("GET /screen/{roundID}", s.screen)
	mux.HandleFunc("GET /screen/{roundID}/fragment", s.screenFragment)
	// Atribuições do cargo (GTSI) — pública: texto normativo, usada na cédula.
	mux.HandleFunc("GET /atribuicoes", s.atribuicoes)
	// QR de acesso (telão), tempo real (SSE) + assets embutidos (offline)
	mux.HandleFunc("GET /qr.png", s.qrPNG)
	mux.HandleFunc("GET /events", s.events)
	mux.Handle("GET /assets/", http.FileServerFS(assetsFS))
	return mux
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func fail(w http.ResponseWriter, err error) { http.Error(w, err.Error(), 500) }

// hxTrigger monta o JSON do header HX-Trigger em ASCII puro. Cabeçalhos HTTP não
// são UTF-8 safe, então acentos são escapados como \uXXXX (o htmx decodifica).
func hxTrigger(events map[string]any) string {
	b, _ := json.Marshal(events)
	var sb strings.Builder
	for _, r := range string(b) {
		if r < 0x80 {
			sb.WriteByte(byte(r))
		} else {
			fmt.Fprintf(&sb, `\u%04x`, r)
		}
	}
	return sb.String()
}

func hxToast(msg string, undo bool) string {
	return hxTrigger(map[string]any{"toast": map[string]any{"msg": msg, "undo": undo}})
}

// actionDone encerra uma ação de mutação (ADR-0008): com htmx, devolve 204 + um
// toast via HX-Trigger; sem JS, redireciona (degradação). O Broadcast é feito pelo
// wrapper s.mut; o SSE atualiza as regiões vivas.
func (s *Server) actionDone(w http.ResponseWriter, r *http.Request, fallback, toast string, undo bool) {
	if r.Header.Get("HX-Request") == "true" {
		if toast != "" {
			w.Header().Set("HX-Trigger", hxToast(toast, undo))
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, fallback, http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Autenticação da Mesa (PIN simples; suficiente p/ LAN — não é cripto forte)
// ---------------------------------------------------------------------------

// authPIN exige PIN definido + cookie da mesa (rotas do wizard pós-passo-1).
func (s *Server) authPIN(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash, err := s.db().PINHash(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		if hash == "" {
			http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
			return
		}
		if c, err := r.Cookie("mesa"); err != nil || c.Value != hash {
			http.Redirect(w, r, "/board/login", http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

// auth protege a área da mesa: PIN + congresso configurado (senão, wizard).
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return s.authPIN(func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.db().FirstCongress(r.Context()); errors.Is(err, sql.ErrNoRows) {
			http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
			return
		}
		h(w, r)
	})
}

// grantCookie loga o dispositivo guardando o hash do PIN no cookie.
func (s *Server) grantCookie(w http.ResponseWriter, hash string) {
	http.SetCookie(w, &http.Cookie{Name: "mesa", Value: hash, Path: "/", HttpOnly: true})
}

func (s *Server) setupSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash, err := s.db().PINHash(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	if hash != "" { // PIN já definido: não deixa redefinir por aqui
		http.Redirect(w, r, "/board/login", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	pin, confirm := r.FormValue("pin"), r.FormValue("confirm")
	if len(pin) < 4 || pin != confirm {
		http.Redirect(w, r, "/board/setup?e=1", http.StatusSeeOther)
		return
	}
	if err := s.db().SetPIN(ctx, pin); err != nil {
		fail(w, err)
		return
	}
	newHash, _ := s.db().PINHash(ctx)
	s.grantCookie(w, newHash)
	http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	hash, err := s.db().PINHash(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if hash == "" { // ainda não há PIN → setup
		http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", map[string]any{"Erro": r.URL.Query().Get("e")})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.ParseForm()
	ok, err := s.db().CheckPIN(ctx, r.FormValue("pin"))
	if err != nil {
		fail(w, err)
		return
	}
	if !ok {
		http.Redirect(w, r, "/board/login?e=1", http.StatusSeeOther)
		return
	}
	hash, _ := s.db().PINHash(ctx)
	s.grantCookie(w, hash)
	http.Redirect(w, r, "/board", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Home (seletor de papel: delegado / mesa / telão)
// ---------------------------------------------------------------------------

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if errors.Is(err, sql.ErrNoRows) { // ainda não configurado → convite ao wizard
		s.render(w, "home.html", map[string]any{"Setup": true})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "home.html", map[string]any{"Congresso": cong})
}

// screenCurrent: telão de endereço fixo — acompanha sozinho o escrutínio da vez.
func (s *Server) screenCurrent(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "screen_live.html", map[string]any{"Congresso": cong})
}

// screenCurrentFragment: fragmento do escrutínio MAIS RECENTE (ou a espera com QR).
// É o que faz o telão fixo trocar de escrutínio sem intervenção manual.
func (s *Server) screenCurrentFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	rd, ok, err := s.db().LatestRound(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !ok {
		s.render(w, "telaoAguarde", map[string]any{"Congresso": cong})
		return
	}
	res, err := s.db().Tally(ctx, rd.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "telaoLive", res)
}

// screenFragment: só a parte viva do telão (respeita ADR-0001 ao renderizar).
func (s *Server) screenFragment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("roundID"), 10, 64)
	if err != nil {
		http.Error(w, "escrutínio inválido", 400)
		return
	}
	res, err := s.db().Tally(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "telaoLive", res)
}

// ---------------------------------------------------------------------------
// Voto (exige sessão de delegado; o token vem do cookie, não do form)
// ---------------------------------------------------------------------------

func (s *Server) voteForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tok := s.requireDelegado(ctx, r)
	if tok == "" {
		http.Redirect(w, r, "/delegado/login", http.StatusSeeOther)
		return
	}
	s.renderVote(w, r, tok, "")
}

func (s *Server) renderVote(w http.ResponseWriter, r *http.Request, token, errMsg string) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	round, pos, open, err := s.db().OpenRound(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !open {
		http.Redirect(w, r, "/delegado", http.StatusSeeOther) // sem escrutínio → hub
		return
	}
	if voted, _ := s.db().HasVoted(ctx, round.ID, token); voted {
		http.Redirect(w, r, "/delegado", http.StatusSeeOther) // já votou → hub
		return
	}
	cands, err := s.db().VotableElectors(ctx, round.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "vote.html", map[string]any{
		"Round": round, "Cargo": pos.Nome, "Candidatos": cands, "Erro": errMsg,
	})
}

func (s *Server) voteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tok := s.requireDelegado(ctx, r)
	if tok == "" {
		http.Redirect(w, r, "/delegado/login?e=1", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form inválido", 400)
		return
	}
	roundID, _ := strconv.ParseInt(r.FormValue("round_id"), 10, 64)
	choice := r.FormValue("choice")

	kind, votee := "candidato", int64(0)
	switch choice {
	case "branco":
		kind = "branco"
	case "nulo":
		kind = "nulo"
	case "":
		s.renderVote(w, r, tok, "Selecione uma opção antes de confirmar.")
		return
	default:
		votee, _ = strconv.ParseInt(choice, 10, 64)
	}

	err := s.db().CastVote(ctx, roundID, tok, kind, votee)
	switch {
	case err == nil, errors.Is(err, store.ErrAlreadyVoted), errors.Is(err, store.ErrRoundClosed):
		// Sucesso, voto duplicado ou rodada fechada: o hub mostra o estado certo.
		http.Redirect(w, r, "/delegado", http.StatusSeeOther)
	case errors.Is(err, store.ErrInvalidToken):
		http.Redirect(w, r, "/delegado/login?e=1", http.StatusSeeOther)
	case errors.Is(err, store.ErrInvalidVotee):
		s.renderVote(w, r, tok, "Esse nome não pode receber voto neste escrutínio.")
	default:
		s.renderVote(w, r, tok, "Erro ao registrar o voto. Chame a mesa.")
	}
}

// ---------------------------------------------------------------------------
// Telão
// ---------------------------------------------------------------------------

func (s *Server) screen(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("roundID"), 10, 64)
	if err != nil {
		http.Error(w, "escrutínio inválido", 400)
		return
	}
	res, err := s.db().Tally(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	// ADR-0001: enquanto aberto, só progresso — nunca placar por candidato.
	s.render(w, "screen.html", res)
}
