package web

// Onboarding (wizard de 3 passos) e configuração posterior (SPEC §6C):
// 1) PIN da Mesa  2) Congresso (federação+ano; cargos GTSI e tokens automáticos)
// 3) Delegados (form individual + colar lista; "concluir depois" permitido).
// Depois: delegados na aba Credenciar; federação/ano em /board/ajustes (engrenagem).

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"votacao-ipb/internal/store"
)

// setupWizard decide o passo do wizard pelo estado atual.
func (s *Server) setupWizard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash, err := s.st.PINHash(ctx)
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
	_, err = s.st.FirstCongress(ctx)
	if errors.Is(err, sql.ErrNoRows) { // passo 2: congresso
		s.render(w, "setup_congresso.html", map[string]any{"AnoDefault": time.Now().Year()})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	// passo 3: delegados (rol vazio, ou ?step=delegados para continuar nele)
	cong, _ := s.st.FirstCongress(ctx)
	electors, err := s.st.Electors(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if len(electors) > 0 && r.URL.Query().Get("step") != "delegados" {
		http.Redirect(w, r, "/board", http.StatusSeeOther)
		return
	}
	locals, _ := s.st.Locals(ctx, cong.ID)
	s.render(w, "setup_delegados.html", map[string]any{
		"Congresso": cong, "Electors": electors, "Locals": locals,
	})
}

// setupCongresso cria o congresso (cargos GTSI + tokens automáticos) — passo 2.
func (s *Server) setupCongresso(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	federacao := strings.TrimSpace(r.FormValue("federacao"))
	ano, _ := strconv.Atoi(r.FormValue("ano"))
	if federacao == "" || ano < 2000 {
		http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
		return
	}
	if _, err := s.st.FirstCongress(r.Context()); err == nil {
		http.Redirect(w, r, "/board/setup", http.StatusSeeOther) // já existe
		return
	}
	// checkbox desmarcado = cargo opcional desativado (federações menores)
	var disabled []int
	for _, seq := range []int{2, 3, 5} {
		if r.FormValue("cargo"+strconv.Itoa(seq)) != "1" {
			disabled = append(disabled, seq)
		}
	}
	if _, err := s.st.SetupCongress(r.Context(), federacao, ano, disabled); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Delegados (wizard passo 3 e aba Credenciar)
// ---------------------------------------------------------------------------

func electorInputFrom(r *http.Request) store.ElectorInput {
	return store.ElectorInput{
		Nome:       strings.TrimSpace(r.FormValue("nome")),
		LocalNome:  strings.TrimSpace(r.FormValue("igreja")),
		Nascimento: strings.TrimSpace(r.FormValue("nascimento")),
		Nato:       r.FormValue("nato") == "1",
	}
}

// delegadoAdd cadastra um delegado (form individual).
func (s *Server) delegadoAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	in := electorInputFrom(r)
	if in.Nome == "" {
		http.Error(w, "nome obrigatório", 400)
		return
	}
	err = s.st.ImportElectors(ctx, cong.ID, []store.ElectorInput{in}, "Adicionou delegado "+in.Nome)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, "Delegado adicionado.")
}

// delegadoImport cadastra em massa (colar lista: uma linha = "Nome; Igreja").
func (s *Server) delegadoImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	items := parseImport(r.FormValue("lista"))
	if len(items) == 0 {
		http.Error(w, "nenhuma linha válida — use \"Nome; Igreja\", um por linha", 400)
		return
	}
	if err := s.st.ImportElectors(ctx, cong.ID, items,
		"Importou "+strconv.Itoa(len(items))+" delegados"); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, strconv.Itoa(len(items))+" delegados importados.")
}

// parseImport: uma linha = "Nome; Igreja" (ou "Nome, Igreja"); sem separador = só nome.
func parseImport(text string) []store.ElectorInput {
	var out []store.ElectorInput
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nome, igreja := line, ""
		if i := strings.Index(line, ";"); i >= 0 {
			nome, igreja = line[:i], line[i+1:]
		} else if i := strings.LastIndex(line, ","); i >= 0 {
			nome, igreja = line[:i], line[i+1:]
		}
		nome = strings.TrimSpace(nome)
		if nome == "" {
			continue
		}
		out = append(out, store.ElectorInput{Nome: nome, LocalNome: strings.TrimSpace(igreja)})
	}
	return out
}

// delegadoUpdate edita um delegado (modal de edição).
func (s *Server) delegadoUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("elector_id"), 10, 64)
	in := electorInputFrom(r)
	if in.Nome == "" {
		http.Error(w, "nome obrigatório", 400)
		return
	}
	if err := s.st.UpdateElector(ctx, cong.ID, id, in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, "Delegado atualizado.")
}

// delegadoDelete remove um delegado nunca credenciado.
func (s *Server) delegadoDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("elector_id"), 10, 64)
	if err := s.st.DeleteElector(ctx, id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.credListaResp(w, r, cong.ID, "Delegado removido.")
}

// credListaResp responde mutações de delegados: com htmx, devolve a lista
// re-renderizada (+ contadores OOB, toast e fecha modais); sem JS, redireciona
// (wizard usa ?next=/board/setup).
func (s *Server) credListaResp(w http.ResponseWriter, r *http.Request, congressID int64, toast string) {
	if r.Header.Get("HX-Request") == "true" {
		electors, err := s.st.Electors(r.Context(), congressID)
		if err != nil {
			fail(w, err)
			return
		}
		q, _ := s.st.Quorum(r.Context(), congressID)
		locals, _ := s.st.Locals(r.Context(), congressID)
		w.Header().Set("HX-Trigger", hxTrigger(map[string]any{
			"toast": map[string]any{"msg": toast, "undo": false}, "closeModals": true}))
		s.render(w, "credListaOOB", map[string]any{"Electors": electors, "Quorum": q, "Locals": locals})
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
	cong, err := s.st.FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	positions, err := s.st.AllPositions(r.Context(), cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "ajustes.html", map[string]any{
		"Active": "", "Congresso": cong, "Positions": positions,
	})
}

// ajustesCargos aplica os checkboxes de cargos opcionais (Ajustes).
func (s *Server) ajustesCargos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	r.ParseForm()
	positions, err := s.st.AllPositions(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	for _, p := range positions {
		want := r.FormValue("cargo"+strconv.Itoa(p.Seq)) == "1"
		if p.Seq != 2 && p.Seq != 3 && p.Seq != 5 {
			continue // obrigatórios
		}
		// checkbox disabled (cargo em curso) não vem no form — não é intenção de desativar
		if !want && p.Ativo && p.Status != "pendente" {
			continue
		}
		if err := s.st.SetPositionAtivo(ctx, p.ID, want); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		updated, err := s.st.AllPositions(ctx, cong.ID)
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
	cong, err := s.st.FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "zonaPerigo", map[string]any{"Congresso": cong})
}

func (s *Server) ajustesSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	federacao := strings.TrimSpace(r.FormValue("federacao"))
	ano, _ := strconv.Atoi(r.FormValue("ano"))
	if federacao == "" || ano < 2000 {
		http.Error(w, "informe federação e ano válidos", 400)
		return
	}
	if err := s.st.UpdateCongress(ctx, cong.ID, federacao, ano); err != nil {
		fail(w, err)
		return
	}
	s.actionDone(w, r, "/board/ajustes", "Dados do congresso salvos.", false)
}
