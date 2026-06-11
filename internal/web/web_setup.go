package web

// Onboarding (wizard de 3 passos) e configuração posterior (SPEC §6C):
// 1) PIN da Mesa  2) Congresso (federação+ano; cargos GTSI e tokens automáticos)
// 3) Delegados (form individual + colar lista; "concluir depois" permitido).
// Depois: delegados na aba Credenciar; federação/ano em /board/ajustes (engrenagem).

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"votacao-ipb/internal/store"
)

// setupWizard decide o passo do wizard pelo estado atual.
func (s *Server) setupWizard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash, err := s.db().PINHash(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	if hash == "" { // passo 1: PIN
		s.render(w, "setup.html", map[string]any{"Erro": r.URL.Query().Get("e")})
		return
	}
	if c, err := r.Cookie("mesa"); err != nil || c.Value != hash {
		http.Redirect(w, r, "/board/login", http.StatusSeeOther)
		return
	}
	_, err = s.db().FirstCongress(ctx)
	if errors.Is(err, sql.ErrNoRows) { // passo 2: a eleição (âmbito, sociedade, cargos)
		// Banco com histórico mas sem congresso = Eleição resetada: oferece o
		// caminho de volta (Desfazer pelo Histórico) antes de reconfigurar.
		_, temOp, _ := s.db().LastOperation(ctx)
		s.render(w, "setup_congresso.html", map[string]any{
			"AnoDefault":   time.Now().Year(),
			"Sociedades":   store.Sociedades,
			"Presets":      store.PresetPositions(store.AmbitoFederacao, "UMP"),
			"Ambito":       store.AmbitoFederacao,
			"TemHistorico": temOp,
		})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	// passo 3: delegados (rol vazio, ou ?step=delegados para continuar nele)
	cong, _ := s.db().FirstCongress(ctx)
	electors, err := s.db().Electors(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if len(electors) > 0 && r.URL.Query().Get("step") != "delegados" {
		http.Redirect(w, r, "/board", http.StatusSeeOther)
		return
	}
	locals, _ := s.db().Locals(ctx, cong.ID)
	subLocals, _ := s.db().SubLocals(ctx, cong.ID)
	s.render(w, "setup_delegados.html", map[string]any{
		"Congresso": cong, "Electors": electors, "Locals": locals, "SubLocals": subLocals,
	})
}

// setupCargosFragment re-renderiza o bloco de cargos do wizard quando a Mesa
// troca âmbito/sociedade (hx-get) — o preset muda (SPEC §10.2).
func (s *Server) setupCargosFragment(w http.ResponseWriter, r *http.Request) {
	ambito, sociedade := r.URL.Query().Get("ambito"), r.URL.Query().Get("sociedade")
	if !store.ValidAmbito(ambito) || !store.ValidSociedade(sociedade) {
		http.Error(w, "âmbito ou sociedade inválidos", 400)
		return
	}
	s.render(w, "cargosPreset", map[string]any{
		"Ambito": ambito, "Presets": store.PresetPositions(ambito, sociedade),
	})
}

// disabledRolesFrom lê os checkboxes de cargos opcionais (desmarcado = desativado).
func disabledRolesFrom(r *http.Request, ambito, sociedade string) []string {
	var disabled []string
	for _, p := range store.PresetPositions(ambito, sociedade) {
		if p.Optional && r.FormValue("cargo_"+p.Role) != "1" {
			disabled = append(disabled, p.Role)
		}
	}
	return disabled
}

// setupCongresso cria a eleição (preset de cargos + tokens automáticos) — passo 2.
func (s *Server) setupCongresso(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ambito := r.FormValue("ambito")
	sociedade := r.FormValue("sociedade")
	nome := strings.TrimSpace(r.FormValue("nome"))
	if ambito == store.AmbitoNacional {
		nome = "" // a Nacional é única — não há entidade-mãe a nomear
	}
	ano, _ := strconv.Atoi(r.FormValue("ano"))
	if (nome == "" && ambito != store.AmbitoNacional) || ano < 2000 ||
		!store.ValidAmbito(ambito) || !store.ValidSociedade(sociedade) {
		http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
		return
	}
	if _, err := s.db().FirstCongress(r.Context()); err == nil {
		http.Redirect(w, r, "/board/setup", http.StatusSeeOther) // já existe
		return
	}
	if _, err := s.db().SetupCongress(r.Context(), ambito, sociedade, nome, ano,
		disabledRolesFrom(r, ambito, sociedade)); err != nil {
		http.Error(w, err.Error(), 400) // ex.: UCP não tem Confederação Nacional
		return
	}
	http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Delegados (wizard passo 3 e aba Credenciar)
// ---------------------------------------------------------------------------

func electorInputFrom(r *http.Request) store.ElectorInput {
	return store.ElectorInput{
		Nome:         strings.TrimSpace(r.FormValue("nome")),
		LocalNome:    strings.TrimSpace(r.FormValue("igreja")),
		SubLocalNome: strings.TrimSpace(r.FormValue("subunidade")),
		Nascimento:   strings.TrimSpace(r.FormValue("nascimento")),
		Nato:         r.FormValue("nato") == "1",
	}
}

// normalizeNascimento aceita DD/MM/AAAA (formato brasileiro, listas coladas) ou
// AAAA-MM-DD (ISO, input type=date) e devolve sempre ISO (formato do banco).
func normalizeNascimento(s string) (string, error) {
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", errors.New("nascimento obrigatório (DD/MM/AAAA)")
}

// validInput valida nome e nascimento (obrigatório — SPEC §10: desempate por
// idade e limites de candidatura dependem dele) e normaliza a data para ISO.
func validInput(in *store.ElectorInput) error {
	if in.Nome == "" {
		return errors.New("nome obrigatório")
	}
	nasc, err := normalizeNascimento(in.Nascimento)
	if err != nil {
		return err
	}
	in.Nascimento = nasc
	return nil
}

// delegadoAdd cadastra um votante (form individual).
func (s *Server) delegadoAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	in := electorInputFrom(r)
	if err := validInput(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err = s.db().ImportElectors(ctx, cong.ID, []store.ElectorInput{in}, "Adicionou "+in.Nome)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, s.term("Votante")+" adicionado.")
}

// delegadoImport cadastra em massa (colar lista; formato por âmbito — SPEC §10.3).
func (s *Server) delegadoImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	items, err := parseImport(r.FormValue("lista"), cong.Ambito)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.db().ImportElectors(ctx, cong.ID, items,
		"Importou "+strconv.Itoa(len(items))+" nomes"); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, strconv.Itoa(len(items))+" nomes importados.")
}

// importFormat descreve o formato de linha esperado por âmbito (SPEC §10.3).
func importFormat(ambito string) string {
	switch ambito {
	case store.AmbitoLocal:
		return "Nome; Nascimento"
	case store.AmbitoSinodal:
		return "Nome; Federação; Nascimento"
	case store.AmbitoNacional:
		return "Nome; Sinodal; Federação; Nascimento"
	default:
		return "Nome; UMP local; Nascimento"
	}
}

// parseImport: uma linha por pessoa, campos separados por ";" conforme o âmbito.
// Nascimento é obrigatório em todos (aceita DD/MM/AAAA ou AAAA-MM-DD).
func parseImport(text, ambito string) ([]store.ElectorInput, error) {
	want := 3 // nome; unidade; nascimento
	switch ambito {
	case store.AmbitoLocal:
		want = 2
	case store.AmbitoNacional:
		want = 4
	}
	var out []store.ElectorInput
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) != want {
			return nil, fmt.Errorf("linha %d: use \"%s\"", n+1, importFormat(ambito))
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		in := store.ElectorInput{Nome: parts[0], Nascimento: parts[len(parts)-1]}
		switch ambito {
		case store.AmbitoLocal:
		case store.AmbitoNacional:
			in.LocalNome, in.SubLocalNome = parts[1], parts[2]
		default:
			in.LocalNome = parts[1]
		}
		if err := validInput(&in); err != nil {
			return nil, fmt.Errorf("linha %d: %s", n+1, err)
		}
		out = append(out, in)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nenhuma linha válida — use \"%s\", um por linha", importFormat(ambito))
	}
	return out, nil
}

// delegadoUpdate edita um votante (modal de edição).
func (s *Server) delegadoUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("elector_id"), 10, 64)
	in := electorInputFrom(r)
	if err := validInput(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.db().UpdateElector(ctx, cong.ID, id, in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, "Cadastro atualizado.")
}

// delegadoDelete remove um delegado nunca credenciado.
func (s *Server) delegadoDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("elector_id"), 10, 64)
	if err := s.db().DeleteElector(ctx, id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, "Removido do rol.")
}

// credListaResp responde mutações de delegados: com htmx, devolve a lista
// re-renderizada (+ contadores OOB, toast e fecha modais); sem JS, redireciona
// (wizard usa ?next=/board/setup).
func (s *Server) credListaResp(w http.ResponseWriter, r *http.Request, congressID int64, toast string) {
	if r.Header.Get("HX-Request") == "true" {
		electors, err := s.db().Electors(r.Context(), congressID)
		if err != nil {
			fail(w, err)
			return
		}
		q, _ := s.db().Quorum(r.Context(), congressID)
		locals, _ := s.db().Locals(r.Context(), congressID)
		subLocals, _ := s.db().SubLocals(r.Context(), congressID)
		w.Header().Set("HX-Trigger", hxTrigger(map[string]any{
			"toast": map[string]any{"msg": toast, "undo": false}, "closeModals": true}))
		s.render(w, "credListaOOB", map[string]any{
			"Electors": electors, "Quorum": q, "Locals": locals, "SubLocals": subLocals})
		return
	}
	next := r.FormValue("next")
	if next == "" {
		next = "/board/credenciamento"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Ajustes (engrenagem): federação e ano
// ---------------------------------------------------------------------------

func (s *Server) ajustes(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	positions, err := s.db().AllPositions(r.Context(), cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "ajustes.html", map[string]any{
		"Active": "ajustes", "Congresso": cong, "Positions": positions,
		"Sociedades": store.Sociedades,
	})
}

// ajustesCargos aplica os checkboxes de cargos opcionais (Ajustes).
func (s *Server) ajustesCargos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	r.ParseForm()
	positions, err := s.db().AllPositions(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	for _, p := range positions {
		want := r.FormValue("cargo_"+p.Role) == "1"
		if !p.Optional {
			continue // obrigatórios neste âmbito
		}
		// checkbox disabled (cargo em curso) não vem no form — não é intenção de desativar
		if !want && p.Ativo && p.Status != "pendente" {
			continue
		}
		if err := s.db().SetPositionAtivo(ctx, p.ID, want); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		updated, err := s.db().AllPositions(ctx, cong.ID)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("HX-Trigger", hxToast("Cargos atualizados.", false))
		s.render(w, "cargosConfig", map[string]any{"Positions": updated})
		return
	}
	http.Redirect(w, r, "/board/ajustes", http.StatusSeeOther)
}

// ajustesZona: região viva da Zona de perigo (o card muda Encerrar↔Reabrir sozinho).
func (s *Server) ajustesZona(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "zonaPerigo", map[string]any{"Congresso": cong})
}

func (s *Server) ajustesSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	ambito := r.FormValue("ambito")
	sociedade := r.FormValue("sociedade")
	if ambito == "" { // selects desabilitados (abertura declarada) não enviam valor
		ambito, sociedade = cong.Ambito, cong.Sociedade
	}
	nome := strings.TrimSpace(r.FormValue("nome"))
	if ambito == store.AmbitoNacional {
		nome = "" // a Nacional é única — não há entidade-mãe a nomear
	}
	ano, _ := strconv.Atoi(r.FormValue("ano"))
	if (nome == "" && ambito != store.AmbitoNacional) || ano < 2000 ||
		!store.ValidAmbito(ambito) || !store.ValidSociedade(sociedade) {
		http.Error(w, "informe nome, ano, âmbito e sociedade válidos", 400)
		return
	}
	if err := s.db().UpdateCongress(ctx, cong.ID, ambito, sociedade, nome, ano); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	mudou := ambito != cong.Ambito || sociedade != cong.Sociedade
	if mudou {
		// O preset de cargos foi re-aplicado e a página inteira muda de vocabulário.
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", "/board/ajustes")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	s.actionDone(w, r, "/board/ajustes", "Dados da eleição salvos.", false)
}
