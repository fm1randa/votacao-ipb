package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

// ns monta um nascimento ISO válido para os testes de desempate por idade.
func ns(d string) sql.NullString { return sql.NullString{String: d, Valid: true} }

// --- Testes puros da regra de vitória (decidirEleito) ---------------------

func TestDecidirEleito_MaioriaEscrutinio1(t *testing.T) {
	r := Result{
		Depositados: 3, Maioria: 2,
		Lines: []ResultLine{{Nome: "A", Votos: 2}, {Nome: "B", Votos: 1}},
	}
	decidirEleito(&r)
	if r.Eleito == nil || r.Eleito.Nome != "A" {
		t.Fatalf("esperava A eleito, veio %v", r.Eleito)
	}
}

func TestDecidirEleito_SemMaioriaComBrancos(t *testing.T) {
	// 2+1+branco => depositados 4, maioria 3; ninguém alcança.
	r := Result{
		Depositados: 4, Maioria: 3, Brancos: 1,
		Lines: []ResultLine{{Nome: "A", Votos: 2}, {Nome: "B", Votos: 1}},
	}
	decidirEleito(&r)
	if r.Eleito != nil {
		t.Fatalf("não devia eleger ninguém, veio %v", r.Eleito)
	}
}

func TestDecidirEleito_RunoffPluralidade(t *testing.T) {
	// Runoff: mesmo sem maioria (brancos altos), vence o mais votado.
	r := Result{
		Round: Round{Runoff: true}, Depositados: 5, Maioria: 3, Brancos: 2,
		Lines: []ResultLine{{Nome: "A", Votos: 2}, {Nome: "B", Votos: 1}},
	}
	decidirEleito(&r)
	if r.Eleito == nil || r.Eleito.Nome != "A" {
		t.Fatalf("esperava A por pluralidade, veio %v", r.Eleito)
	}
}

func TestDecidirEleito_RunoffEmpateMaiorIdade(t *testing.T) {
	// Empate exato no runoff → maior idade (menor data de nascimento).
	r := Result{
		Round: Round{Runoff: true}, Depositados: 4, Maioria: 3,
		Lines: []ResultLine{
			{Nome: "A", Votos: 2, Nascimento: ns("1990-05-10")},
			{Nome: "B", Votos: 2, Nascimento: ns("1985-03-01")}, // mais velho
		},
	}
	decidirEleito(&r)
	if r.Eleito == nil || r.Eleito.Nome != "B" {
		t.Fatalf("esperava B (mais velho), veio %v", r.Eleito)
	}
}

func TestDecidirEleito_RunoffEmpateSemNascimento(t *testing.T) {
	r := Result{
		Round: Round{Runoff: true}, Depositados: 4, Maioria: 3,
		Lines: []ResultLine{
			{Nome: "A", Votos: 2, Nascimento: ns("1990-05-10")},
			{Nome: "B", Votos: 2}, // sem nascimento
		},
	}
	decidirEleito(&r)
	if r.Eleito != nil || !r.EmpateNaoResolvido {
		t.Fatalf("esperava empate não resolvido, veio eleito=%v flag=%v", r.Eleito, r.EmpateNaoResolvido)
	}
}

// --- Testes de integração (banco temporário) ------------------------------

type cenario struct {
	st       *Store
	cong     int64
	pos      int64
	tokens   map[string]string // nome do delegado -> token
	electors map[string]int64
}

func setup(t *testing.T) *cenario {
	t.Helper()
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	cong, err := st.CreateCongress(ctx, AmbitoFederacao, "UMP", "Federação Teste", 2026)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := st.AddLocal(ctx, cong, "IP Central", 0)
	if err := st.GenerateTokens(ctx, cong, 20); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPosition(ctx, cong, "Presidente", RolePresidente, 1); err != nil {
		t.Fatal(err)
	}
	poss, _ := st.Positions(ctx, cong)

	c := &cenario{st: st, cong: cong, pos: poss[0].ID,
		tokens: map[string]string{}, electors: map[string]int64{}}
	// 4 delegados com nascimentos distintos.
	for _, d := range []struct{ nome, nasc string }{
		{"Ana", "1985-01-01"}, {"Bruno", "1990-01-01"},
		{"Caio", "1995-01-01"}, {"Davi", "2000-01-01"},
	} {
		id, err := st.AddElector(ctx, cong, d.nome, &loc, nil, false, d.nasc)
		if err != nil {
			t.Fatal(err)
		}
		c.electors[d.nome] = id
		tok, err := st.Credenciar(ctx, cong, id) // marca presente + entrega token
		if err != nil {
			t.Fatal(err)
		}
		c.tokens[d.nome] = tok
	}
	if err := st.DeclararAbertura(ctx, cong); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCastVote_QueimaToken(t *testing.T) {
	ctx := context.Background()
	c := setup(t)
	round, err := c.st.AbrirCargo(ctx, c.cong, c.pos, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.st.CastVote(ctx, round.ID, c.tokens["Ana"], "candidato", c.electors["Bruno"]); err != nil {
		t.Fatalf("1º voto deveria passar: %v", err)
	}
	// Mesmo token, de novo, no mesmo escrutínio → queimado.
	err = c.st.CastVote(ctx, round.ID, c.tokens["Ana"], "branco", 0)
	if err != ErrAlreadyVoted {
		t.Fatalf("esperava ErrAlreadyVoted, veio %v", err)
	}
}

func TestTally_DenominadorIncluiBrancoNulo(t *testing.T) {
	ctx := context.Background()
	c := setup(t)
	round, _ := c.st.AbrirCargo(ctx, c.cong, c.pos, nil)
	// Ana e Bruno votam em Caio; Caio vota branco; Davi vota nulo.
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Ana"], "candidato", c.electors["Caio"]))
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Bruno"], "candidato", c.electors["Caio"]))
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Caio"], "branco", 0))
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Davi"], "nulo", 0))

	res, err := c.st.Tally(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Depositados != 4 || res.Brancos != 1 || res.Nulos != 1 {
		t.Fatalf("depositados/brancos/nulos errados: %+v", res)
	}
	if res.Maioria != 3 { // ⌊4/2⌋+1
		t.Fatalf("maioria esperada 3, veio %d", res.Maioria)
	}
	// Caio tem 2 < 3 → ninguém eleito (brancos/nulos elevaram o denominador).
	if res.Eleito != nil {
		t.Fatalf("não devia haver eleito, veio %v", res.Eleito)
	}
	if res.Presentes != 4 {
		t.Fatalf("presentes esperado 4, veio %d", res.Presentes)
	}
}

func TestFluxoCompleto_CargoDecidido(t *testing.T) {
	ctx := context.Background()
	c := setup(t)
	round, _ := c.st.AbrirCargo(ctx, c.cong, c.pos, nil)
	// 3 votos em Ana, 1 em Bruno: maioria de 4 depositados = 3 → Ana eleita.
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Ana"], "candidato", c.electors["Ana"]))
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Bruno"], "candidato", c.electors["Ana"]))
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Caio"], "candidato", c.electors["Ana"]))
	must(t, c.st.CastVote(ctx, round.ID, c.tokens["Davi"], "candidato", c.electors["Bruno"]))

	res, err := c.st.EncerrarEscrutinio(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Eleito == nil || res.Eleito.ElectorID != c.electors["Ana"] {
		t.Fatalf("esperava Ana eleita, veio %v", res.Eleito)
	}
	pos, _ := c.st.GetPosition(ctx, c.pos)
	if pos.Status != "decidido" || pos.EleitoNome != "Ana" {
		t.Fatalf("cargo deveria estar decidido com Ana, veio %+v", pos)
	}
}

func TestEleitoNaoConcorreAOutroCargo(t *testing.T) {
	ctx := context.Background()
	c := setup(t)
	must(t, c.st.AddPosition(ctx, c.cong, "Vice-presidente", RoleVice, 2))

	// Elege Ana para Presidente (4 votos de 4).
	round, _ := c.st.AbrirCargo(ctx, c.cong, c.pos, nil)
	for _, nome := range []string{"Ana", "Bruno", "Caio", "Davi"} {
		must(t, c.st.CastVote(ctx, round.ID, c.tokens[nome], "candidato", c.electors["Ana"]))
	}
	if _, err := c.st.EncerrarEscrutinio(ctx, round.ID); err != nil {
		t.Fatal(err)
	}

	// Abre o Vice: Ana não pode ser votável nem receber voto.
	poss, _ := c.st.Positions(ctx, c.cong)
	vice := poss[1]
	r2, err := c.st.AbrirCargo(ctx, c.cong, vice.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	votaveis, err := c.st.VotableElectors(ctx, r2.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range votaveis {
		if e.ID == c.electors["Ana"] {
			t.Fatalf("Ana já foi eleita e não deveria estar na cédula do Vice")
		}
	}
	if err := c.st.CastVote(ctx, r2.ID, c.tokens["Bruno"], "candidato", c.electors["Ana"]); err != ErrInvalidVotee {
		t.Fatalf("voto em eleita deveria ser recusado (ErrInvalidVotee), veio %v", err)
	}
	// Votar em alguém não eleito segue normal.
	must(t, c.st.CastVote(ctx, r2.ID, c.tokens["Caio"], "candidato", c.electors["Bruno"]))
}

func TestCargosConfiguraveis(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	// Setup com Vice e 2º Secretário desativados (federação menor).
	cong, err := st.SetupCongress(ctx, AmbitoFederacao, "UMP", "Fed Pequena", 2026,
		[]string{RoleVice, RoleSegundoSec})
	if err != nil {
		t.Fatal(err)
	}
	ativos, _ := st.Positions(ctx, cong)
	if len(ativos) != 4 {
		t.Fatalf("esperava 4 cargos ativos, veio %d", len(ativos))
	}
	// Sem o 2º Secretário, o 1º exibe-se como "Secretário".
	temSecretario := false
	for _, p := range ativos {
		if p.Nome == "Secretário" {
			temSecretario = true
		}
		if p.Nome == "Vice-presidente" || p.Nome == "2º Secretário" || p.Nome == "1º Secretário" {
			t.Fatalf("cargo inesperado na lista ativa: %s", p.Nome)
		}
	}
	if !temSecretario {
		t.Fatal("1º Secretário deveria exibir-se como 'Secretário'")
	}

	all, _ := st.AllPositions(ctx, cong)
	if len(all) != 6 {
		t.Fatalf("AllPositions deveria ter 6, veio %d", len(all))
	}
	// Obrigatório não desativa; reativar o 2º devolve o nome "1º Secretário".
	for _, p := range all {
		if p.Seq == 1 {
			if err := st.SetPositionAtivo(ctx, p.ID, false); err == nil {
				t.Fatal("Presidente é obrigatório — desativar deveria falhar")
			}
		}
		if p.Seq == 5 {
			must(t, st.SetPositionAtivo(ctx, p.ID, true))
		}
	}
	ativos, _ = st.Positions(ctx, cong)
	nomes := map[string]bool{}
	for _, p := range ativos {
		nomes[p.Nome] = true
	}
	if !nomes["1º Secretário"] || !nomes["2º Secretário"] {
		t.Fatalf("com o 2º reativado, esperava 1º e 2º Secretário; veio %v", nomes)
	}
	// Abrir escrutínio de cargo desativado (Vice) é recusado mesmo com abertura.
	loc, _ := st.AddLocal(ctx, cong, "IP Única", 0)
	id, _ := st.AddElector(ctx, cong, "Zé", &loc, nil, false, "1999-01-01")
	if _, err := st.Credenciar(ctx, cong, id); err != nil {
		t.Fatal(err)
	}
	must(t, st.DeclararAbertura(ctx, cong))
	for _, p := range all {
		if p.Role == RoleVice {
			if _, err := st.AbrirCargo(ctx, cong, p.ID, nil); err == nil {
				t.Fatal("abrir escrutínio de cargo desativado deveria falhar")
			}
		}
	}
}

func TestAbrirCargo_ExigeAberturaDeclarada(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	cong, _ := st.CreateCongress(ctx, AmbitoFederacao, "UMP", "F", 2026)
	st.AddPosition(ctx, cong, "Presidente", RolePresidente, 1)
	poss, _ := st.Positions(ctx, cong)
	if _, err := st.AbrirCargo(ctx, cong, poss[0].ID, nil); err == nil {
		t.Fatal("deveria recusar abrir cargo sem abertura declarada")
	}
}

func TestDeclararAbertura_ExigeQuorumComputado(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	// Federação com 3 UMPs e 1 só representada → 1/3 não é mais da metade.
	cong, _ := st.SetupCongress(ctx, AmbitoFederacao, "UMP", "F", 2026, nil)
	var locs []int64
	for _, nome := range []string{"A", "B", "C"} {
		id, _ := st.AddLocal(ctx, cong, nome, 0)
		locs = append(locs, id)
	}
	e1, _ := st.AddElector(ctx, cong, "Um", &locs[0], nil, false, "1999-01-01")
	if _, err := st.Credenciar(ctx, cong, e1); err != nil {
		t.Fatal(err)
	}
	if err := st.DeclararAbertura(ctx, cong); err == nil {
		t.Fatal("sem quórum, declarar abertura deveria falhar (ADR-0010)")
	}
	// Representa a 2ª UMP → 2/3 é mais da metade → declara.
	e2, _ := st.AddElector(ctx, cong, "Dois", &locs[1], nil, false, "1998-01-01")
	if _, err := st.Credenciar(ctx, cong, e2); err != nil {
		t.Fatal(err)
	}
	must(t, st.DeclararAbertura(ctx, cong))
}

func TestQuorumLocal_MetadeDoRol(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	// UMP local com 4 sócios no rol: 2 presentes não bastam; 3 sim.
	cong, _ := st.SetupCongress(ctx, AmbitoLocal, "UMP", "UMP da IP Central", 2026, nil)
	var ids []int64
	for _, nome := range []string{"A", "B", "C", "D"} {
		id, _ := st.AddElector(ctx, cong, nome, nil, nil, false, "1999-01-01")
		ids = append(ids, id)
	}
	for _, id := range ids[:2] {
		if _, err := st.Credenciar(ctx, cong, id); err != nil {
			t.Fatal(err)
		}
	}
	if q, _ := st.Quorum(ctx, cong); q.Ok {
		t.Fatalf("2/4 do rol não é mais da metade: %+v", q)
	}
	if _, err := st.Credenciar(ctx, cong, ids[2]); err != nil {
		t.Fatal(err)
	}
	q, _ := st.Quorum(ctx, cong)
	if !q.Ok || q.RolTotal != 4 || q.Presentes != 3 {
		t.Fatalf("3/4 do rol deveria dar quórum: %+v", q)
	}
	must(t, st.DeclararAbertura(ctx, cong))
}

func TestQuorumNacional_Composto(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	// Nacional: 2 Sinodais (precisa 2 repr.) e 6 Federações (precisa ≥2 repr.).
	cong, _ := st.SetupCongress(ctx, AmbitoNacional, "UMP", "Confederação Nacional", 2026, nil)
	sin1, _ := st.AddLocal(ctx, cong, "Sinodal Sul", 0)
	sin2, _ := st.AddLocal(ctx, cong, "Sinodal Norte", 0)
	var feds []int64
	for _, nome := range []string{"F1", "F2", "F3", "F4", "F5", "F6"} {
		id, _ := st.AddLocal(ctx, cong, nome, 1)
		feds = append(feds, id)
	}
	add := func(nome string, sin, fed int64) int64 {
		id, err := st.AddElector(ctx, cong, nome, &sin, &fed, false, "1999-01-01")
		must(t, err)
		_, err = st.Credenciar(ctx, cong, id)
		must(t, err)
		return id
	}
	// 2 sinodais representadas mas só 1 federação (1/6 < 1/3) → sem quórum.
	add("A", sin1, feds[0])
	add("B", sin2, feds[0])
	if q, _ := st.Quorum(ctx, cong); q.Ok {
		t.Fatalf("1/6 federações não atinge 1/3: %+v", q)
	}
	// 2ª federação representada (2/6 = 1/3) → quórum.
	add("C", sin1, feds[1])
	q, _ := st.Quorum(ctx, cong)
	if !q.Ok || q.UnidadesRepr != 2 || q.SubRepr != 2 {
		t.Fatalf("2/2 sinodais + 2/6 federações deveria dar quórum: %+v", q)
	}
}

func TestQuorumNacional_PorSociedade(t *testing.T) {
	ctx := context.Background()
	// Cenário comum: 2 Sinodais (ambas representadas) e 4 Federações, 1 representada.
	monta := func(t *testing.T, soc string) (*Store, int64) {
		st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
		t.Cleanup(func() { st.Close() })
		cong, err := st.SetupCongress(ctx, AmbitoNacional, soc, "Nacional "+soc, 2026, nil)
		must(t, err)
		sin1, _ := st.AddLocal(ctx, cong, "Sinodal A", 0)
		sin2, _ := st.AddLocal(ctx, cong, "Sinodal B", 0)
		var feds []int64
		for _, nome := range []string{"F1", "F2", "F3", "F4"} {
			id, _ := st.AddLocal(ctx, cong, nome, 1)
			feds = append(feds, id)
		}
		for i, sin := range []int64{sin1, sin2} {
			id, err := st.AddElector(ctx, cong, fmt.Sprintf("D%d", i), &sin, &feds[0], false, "1999-01-01")
			must(t, err)
			_, err = st.Credenciar(ctx, cong, id)
			must(t, err)
		}
		return st, cong
	}
	// UMP: 1/4 federações < 1/3 → sem quórum.
	st, cong := monta(t, "UMP")
	if q, _ := st.Quorum(ctx, cong); q.Ok || q.SubRegra != SubRegraTerco {
		t.Fatalf("UMP: 1/4 federações não atinge 1/3: %+v", q)
	}
	// UPH: 1/4 federações < metade → sem quórum (regra mais dura).
	st, cong = monta(t, "UPH")
	if q, _ := st.Quorum(ctx, cong); q.Ok || q.SubRegra != SubRegraMetade {
		t.Fatalf("UPH: 1/4 federações não é mais da metade: %+v", q)
	}
	// UPA: federações não contam → 2/2 sinodais bastam.
	st, cong = monta(t, "UPA")
	if q, _ := st.Quorum(ctx, cong); !q.Ok || q.SubRegra != SubRegraNada {
		t.Fatalf("UPA: 2/2 sinodais deveria bastar (sem critério de federações): %+v", q)
	}
}

func TestUCP_SemConfederacaoNacional(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	if _, err := st.SetupCongress(ctx, AmbitoNacional, "UCP", "X", 2026, nil); err == nil {
		t.Fatal("UCP não possui Confederação Nacional — setup deveria falhar")
	}
	cong, err := st.SetupCongress(ctx, AmbitoSinodal, "UCP", "Sinodal UCP", 2026, nil)
	must(t, err)
	if err := st.UpdateCongress(ctx, cong, AmbitoNacional, "UCP", "X", 2026); err == nil {
		t.Fatal("trocar UCP para âmbito nacional deveria falhar")
	}
}

func TestIdadeMaxima_BloqueiaCandidaturaNaoOVoto(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	// Sinodal UMP: limite 33 anos para SER votado (Art. 4º §4).
	cong, _ := st.SetupCongress(ctx, AmbitoSinodal, "UMP", "Sinodal Teste", 2026, nil)
	fed, _ := st.AddLocal(ctx, cong, "Federação Única", 0)
	velho, _ := st.AddElector(ctx, cong, "Veterano", &fed, nil, false, "1980-01-01") // 46 anos
	jovem, _ := st.AddElector(ctx, cong, "Jovem", &fed, nil, false, "2000-01-01")    // 26 anos
	tokVelho, err := st.Credenciar(ctx, cong, velho)
	must(t, err)
	_, err = st.Credenciar(ctx, cong, jovem)
	must(t, err)
	must(t, st.DeclararAbertura(ctx, cong))

	poss, _ := st.Positions(ctx, cong)
	round, err := st.AbrirCargo(ctx, cong, poss[0].ID, nil)
	must(t, err)
	votaveis, _ := st.VotableElectors(ctx, round.ID)
	for _, e := range votaveis {
		if e.ID == velho {
			t.Fatal("quem excede 33 anos não deveria estar na cédula do Sinodal")
		}
	}
	if err := st.CastVote(ctx, round.ID, tokVelho, "candidato", velho); err != ErrInvalidVotee {
		t.Fatalf("voto em quem excede a idade deveria ser recusado, veio %v", err)
	}
	// O veterano VOTA normalmente (limite é só para ser votado).
	must(t, st.CastVote(ctx, round.ID, tokVelho, "candidato", jovem))
}

func TestPresets_LocalENacional(t *testing.T) {
	local := PresetPositions(AmbitoLocal, "UMP")
	if len(local) != 5 {
		t.Fatalf("local UMP deveria ter 5 cargos, veio %d", len(local))
	}
	for _, p := range local {
		if p.Role == RoleSecExec {
			t.Fatal("local não tem Secretário Executivo (Art. 13)")
		}
	}
	nac := PresetPositions(AmbitoNacional, "UMP")
	if len(nac) != 10 { // Pres + 5 vices + SecExec + 2 Secs + Tes
		t.Fatalf("nacional UMP deveria ter 10 cargos, veio %d", len(nac))
	}
	nacSAF := PresetPositions(AmbitoNacional, "SAF")
	if len(nacSAF) != 11 { // SAF: 6 vices (Sudeste Norte/Sul)
		t.Fatalf("nacional SAF deveria ter 11 cargos, veio %d", len(nacSAF))
	}
	temTesoureira := false
	for _, p := range nacSAF {
		if p.Nome == "Tesoureira" {
			temTesoureira = true
		}
		if p.Optional {
			t.Fatal("nacional é prescritivo — nenhum cargo opcional (Art. 26b)")
		}
	}
	if !temTesoureira {
		t.Fatal("SAF usa títulos femininos (Tesoureira)")
	}
}

func TestMudarAmbito_SoAntesDaAbertura(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	cong, _ := st.SetupCongress(ctx, AmbitoFederacao, "UMP", "F", 2026, nil)
	// Antes da abertura: muda pra local e o preset é re-aplicado (5 cargos, sem SecExec).
	must(t, st.UpdateCongress(ctx, cong, AmbitoLocal, "UMP", "UMP da IP Central", 2026))
	all, _ := st.AllPositions(ctx, cong)
	if len(all) != 5 {
		t.Fatalf("após trocar pra local, esperava 5 cargos, veio %d", len(all))
	}
	// Declara abertura (1 sócio de rol 1 = mais da metade) e tenta trocar de novo.
	id, _ := st.AddElector(ctx, cong, "Zé", nil, nil, false, "1999-01-01")
	if _, err := st.Credenciar(ctx, cong, id); err != nil {
		t.Fatal(err)
	}
	must(t, st.DeclararAbertura(ctx, cong))
	if err := st.UpdateCongress(ctx, cong, AmbitoSinodal, "UMP", "X", 2026); err == nil {
		t.Fatal("trocar âmbito após a abertura deveria falhar")
	}
	// Nome/ano seguem editáveis sem trocar âmbito.
	must(t, st.UpdateCongress(ctx, cong, AmbitoLocal, "UMP", "UMP Central", 2027))
}

func TestOperationLog_ReiniciarEDesfazer(t *testing.T) {
	ctx := context.Background()
	c := setup(t)
	round, _ := c.st.AbrirCargo(ctx, c.cong, c.pos, nil)
	// 4 votos em Ana → maioria 3 → eleita; cargo decidido.
	for _, nome := range []string{"Ana", "Bruno", "Caio", "Davi"} {
		must(t, c.st.CastVote(ctx, round.ID, c.tokens[nome], "candidato", c.electors["Ana"]))
	}
	if _, err := c.st.EncerrarEscrutinio(ctx, round.ID); err != nil {
		t.Fatal(err)
	}

	// Estado pós-eleição.
	if p, _ := c.st.GetPosition(ctx, c.pos); p.Status != "decidido" {
		t.Fatalf("antes do reset: esperava decidido, veio %s", p.Status)
	}
	if q, _ := c.st.Quorum(ctx, c.cong); q.Presentes != 4 {
		t.Fatalf("antes do reset: esperava 4 presentes, veio %d", q.Presentes)
	}

	// Reiniciar a eleição → tudo zerado.
	must(t, c.st.ReiniciarEleicao(ctx, c.cong))
	if p, _ := c.st.GetPosition(ctx, c.pos); p.Status != "pendente" {
		t.Fatalf("após reset: esperava pendente, veio %s", p.Status)
	}
	if q, _ := c.st.Quorum(ctx, c.cong); q.Presentes != 0 {
		t.Fatalf("após reset: esperava 0 presentes, veio %d", q.Presentes)
	}

	// Desfazer → restaura o estado pré-reset.
	must(t, c.st.UndoLast(ctx))
	if p, _ := c.st.GetPosition(ctx, c.pos); p.Status != "decidido" || p.EleitoNome != "Ana" {
		t.Fatalf("após desfazer: esperava decidido/Ana, veio %s/%s", p.Status, p.EleitoNome)
	}
	if q, _ := c.st.Quorum(ctx, c.cong); q.Presentes != 4 {
		t.Fatalf("após desfazer: esperava 4 presentes de volta, veio %d", q.Presentes)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
