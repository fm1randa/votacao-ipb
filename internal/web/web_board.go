package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"votacao-ipb/internal/store"
)

// boardData monta o estado da aba "Escrutínio" (sem o Toast, que é por-requisição).
func (s *Server) boardData(ctx context.Context) (map[string]any, error) {
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		return nil, err
	}
	q, err := s.db().Quorum(ctx, cong.ID)
	if err != nil {
		return nil, err
	}
	positions, err := s.db().Positions(ctx, cong.ID)
	if err != nil {
		return nil, err
	}
	decididos := 0
	for _, p := range positions {
		if p.Status == "decidido" {
			decididos++
		}
	}
	data := map[string]any{
		"Active": "escrutinio", "Congresso": cong, "Quorum": q,
		"Positions": positions, "Decididos": decididos, "Total": len(positions),
	}
	atual, ok, err := s.db().CurrentPosition(ctx, cong.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		data["Concluido"] = true
		return data, nil
	}
	data["Atual"] = atual
	for i, p := range positions {
		if p.ID == atual.ID {
			data["AtualIdx"] = i + 1 // posição na lista ATIVA (seqs podem pular)
			break
		}
	}
	if atual.Status == "em_eleicao" {
		if round, err := s.db().CurrentRound(ctx, atual.ID); err == nil {
			data["Round"] = round
			// Selo "indicação: N nomes" (só fora do runoff — lá o top-2 é regra).
			if !round.Runoff {
				if n, err := s.db().IndicadosCount(ctx, round.ID); err == nil && n > 0 {
					data["Indicados"] = n
				}
			}
			if round.Status == "aberto" {
				data["PodeEncerrar"] = true
				if res, err := s.db().Tally(ctx, round.ID); err == nil {
					data["Depositados"] = res.Depositados
					data["Presentes"] = res.Presentes
				}
			} else if round.Numero < store.MaxRounds {
				data["PodeProximo"] = true
			}
		}
	} else {
		data["PodeAbrir"] = true
	}
	return data, nil
}

// board é a aba "Escrutínio": foco no cargo da vez + uma ação, com progresso.
func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	data, err := s.boardData(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "board.html", data)
}

// boardFragment: só a parte viva da aba Escrutínio (re-buscada via SSE/htmx).
func (s *Server) boardFragment(w http.ResponseWriter, r *http.Request) {
	data, err := s.boardData(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "boardLive", data)
}

// credenciamento é a aba dedicada: busca (nome/igreja) + filtros + ação inline (JS no template).
func (s *Server) credenciamento(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	q, _ := s.db().Quorum(ctx, cong.ID)
	electors, err := s.db().Electors(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	locals, _ := s.db().Locals(ctx, cong.ID)
	subLocals, _ := s.db().SubLocals(ctx, cong.ID)
	s.render(w, "credenciamento.html", map[string]any{
		"Active": "credenciamento", "Congresso": cong, "Quorum": q,
		"Electors": electors, "Locals": locals, "SubLocals": subLocals,
	})
}

// historico é a aba do log de operações (Desfazer/Restaurar + zona de perigo).
// Tolera banco sem congresso (pós-Reset): o Desfazer é o caminho de volta.
func (s *Server) historico(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(w, err)
		return
	}
	ops, err := s.db().Operations(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "historico.html", map[string]any{
		"Active": "historico", "Congresso": cong, "Operations": ops,
	})
}

func (s *Server) undo(w http.ResponseWriter, r *http.Request) {
	if err := s.db().UndoLast(r.Context()); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Operação desfeita.", false)
}

type diffRow struct {
	Label, De, Para string
	Mudou           bool
}

func statusTxt(c store.CargoState) string {
	switch c.Status {
	case "decidido":
		return "eleito: " + c.Eleito
	case "em_eleicao":
		return "em eleição"
	default:
		return "pendente"
	}
}

func eleicaoTxt(encerrada bool) string {
	if encerrada {
		return "encerrada"
	}
	return "em andamento"
}

func buildDiff(cur, tgt store.StateSummary) ([]diffRow, bool) {
	d := func(label, de, para string) diffRow { return diffRow{label, de, para, de != para} }
	rows := []diffRow{
		d("Situação", eleicaoTxt(cur.Encerrada), eleicaoTxt(tgt.Encerrada)),
		d("Presentes", strconv.Itoa(cur.Presentes), strconv.Itoa(tgt.Presentes)),
		d("Credenciados", strconv.Itoa(cur.Credenciados), strconv.Itoa(tgt.Credenciados)),
		d("Votos registrados", strconv.Itoa(cur.TotalVotos), strconv.Itoa(tgt.TotalVotos)),
	}
	n := len(tgt.Cargos)
	if len(cur.Cargos) > n {
		n = len(cur.Cargos)
	}
	for i := 0; i < n; i++ {
		// Cargo ausente num dos lados (ex.: banco resetado) exibe "—", não
		// "pendente" — senão o diff acha que nada mudou.
		nome, de, para := "", "—", "—"
		if i < len(tgt.Cargos) {
			para = statusTxt(tgt.Cargos[i])
			nome = tgt.Cargos[i].Nome
		}
		if i < len(cur.Cargos) {
			de = statusTxt(cur.Cargos[i])
			if nome == "" {
				nome = cur.Cargos[i].Nome
			}
		}
		rows = append(rows, d(nome, de, para))
	}
	changed := false
	for _, r := range rows {
		if r.Mudou {
			changed = true
		}
	}
	return rows, changed
}

// restorePreview devolve o fragmento do modal: diff "antes → depois" da restauração.
func (s *Server) restorePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opID, _ := strconv.ParseInt(r.URL.Query().Get("op_id"), 10, 64)
	cur, err := s.db().SummarizeCurrent(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	tgt, desc, err := s.db().SummarizeSnapshot(ctx, opID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	rows, changed := buildDiff(cur, tgt)
	s.render(w, "restore_preview", map[string]any{
		"OpID": opID, "Desc": desc, "Rows": rows, "Changed": changed,
	})
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	opID, _ := strconv.ParseInt(r.FormValue("op_id"), 10, 64)
	if err := s.db().RestoreToOp(r.Context(), opID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board/historico", "Estado restaurado.", false)
}

func (s *Server) reiniciar(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.db().ReiniciarEleicao(r.Context(), cong.ID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Eleição reiniciada.", true)
}

func (s *Server) encerrarEleicao(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.db().EncerrarEleicao(r.Context(), cong.ID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Eleição encerrada.", true)
}

func (s *Server) reabrirEleicao(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.db().ReabrirEleicao(r.Context(), cong.ID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Eleição reaberta.", false)
}

// historicoFragment: a parte viva do Histórico (lista de operações + zona de perigo).
func (s *Server) historicoFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(w, err)
		return
	}
	ops, err := s.db().Operations(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "historicoLive", map[string]any{"Congresso": cong, "Operations": ops})
}

// ---------------------------------------------------------------------------
// Ações
// ---------------------------------------------------------------------------

func (s *Server) credenciar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	electorID, _ := strconv.ParseInt(r.FormValue("elector_id"), 10, 64)
	code, err := s.db().Credenciar(ctx, cong.ID, electorID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		el, _ := s.db().GetElector(ctx, electorID)
		q, _ := s.db().Quorum(ctx, cong.ID)
		w.Header().Set("HX-Trigger", `{"openToken":true}`)
		s.render(w, "credResult", map[string]any{"Token": code, "Row": el, "Quorum": q})
		return
	}
	s.render(w, "token.html", map[string]any{"Token": code, "Reissue": false})
}

func (s *Server) reissue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	code, err := s.db().IssueToken(ctx, cong.ID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		q, _ := s.db().Quorum(ctx, cong.ID)
		w.Header().Set("HX-Trigger", `{"openToken":true}`)
		s.render(w, "credResult", map[string]any{"Token": code, "Row": nil, "Quorum": q})
		return
	}
	s.render(w, "token.html", map[string]any{"Token": code, "Reissue": true})
}

func (s *Server) presenca(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	electorID, _ := strconv.ParseInt(r.FormValue("elector_id"), 10, 64)
	presente := r.FormValue("presente") == "1"
	if err := s.db().SetPresente(ctx, electorID, presente); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		el, _ := s.db().GetElector(ctx, electorID)
		s.render(w, "credRow", map[string]any{"E": el, "OOB": false})
		return
	}
	http.Redirect(w, r, "/board/credenciamento", http.StatusSeeOther)
}

// declararAbertura: gate computado (ADR-0010) — o store recusa sem quórum.
func (s *Server) declararAbertura(w http.ResponseWriter, r *http.Request) {
	cong, err := s.db().FirstCongress(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.db().DeclararAbertura(r.Context(), cong.ID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Abertura declarada — quórum verificado.", false)
}

// indicarForm: corpo do modal de indicação (Art. 91d) — lista fresca dos
// indicáveis, carregada no clique de "Com indicações…".
func (s *Server) indicarForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	positionID, _ := strconv.ParseInt(r.URL.Query().Get("position_id"), 10, 64)
	pos, err := s.db().GetPosition(ctx, positionID)
	if err != nil {
		http.Error(w, "cargo não encontrado", 400)
		return
	}
	electors, err := s.db().IndicaveisElectors(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "indicarForm", map[string]any{"Position": pos, "Electors": electors})
}

func (s *Server) abrirCargo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	r.ParseForm()
	positionID, _ := strconv.ParseInt(r.FormValue("position_id"), 10, 64)
	var indicados []int64
	for _, v := range r.Form["indicados"] {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			indicados = append(indicados, id)
		}
	}
	// Caminho "Com indicações…" sem ninguém marcado: não abre cédula vazia.
	if r.FormValue("via") == "indicacao" && len(indicados) == 0 {
		http.Error(w, "marque ao menos um nome para indicar", 400)
		return
	}
	if _, err := s.db().AbrirCargo(ctx, cong.ID, positionID, indicados); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	toast := "Escrutínio aberto."
	if n := len(indicados); n > 0 {
		toast = "Escrutínio aberto com " + strconv.Itoa(n) + " indicados."
	}
	s.actionDone(w, r, "/board", toast, false)
}

func (s *Server) encerrar(w http.ResponseWriter, r *http.Request) {
	roundID, _ := strconv.ParseInt(r.FormValue("round_id"), 10, 64)
	if _, err := s.db().EncerrarEscrutinio(r.Context(), roundID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Escrutínio encerrado.", false)
}

func (s *Server) proximo(w http.ResponseWriter, r *http.Request) {
	positionID, _ := strconv.ParseInt(r.FormValue("position_id"), 10, 64)
	if _, err := s.db().AbrirProximoEscrutinio(r.Context(), positionID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.actionDone(w, r, "/board", "Novo escrutínio aberto.", false)
}

// report monta a saída imprimível: Verificação de Poderes + resultado de cada escrutínio.
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cong, err := s.db().FirstCongress(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	q, err := s.db().Quorum(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}
	positions, err := s.db().Positions(ctx, cong.ID)
	if err != nil {
		fail(w, err)
		return
	}

	type cargoReport struct {
		Position store.Position
		Results  []store.Result
	}
	var cargos []cargoReport
	for _, p := range positions {
		rounds, err := s.db().Rounds(ctx, p.ID)
		if err != nil {
			fail(w, err)
			return
		}
		cr := cargoReport{Position: p}
		for _, rd := range rounds {
			res, err := s.db().Tally(ctx, rd.ID)
			if err != nil {
				fail(w, err)
				return
			}
			cr.Results = append(cr.Results, res)
		}
		cargos = append(cargos, cr)
	}
	s.render(w, "report.html", map[string]any{"Congresso": cong, "Quorum": q, "Cargos": cargos})
}
