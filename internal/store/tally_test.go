package store

import (
	"context"
	"database/sql"
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

	cong, err := st.CreateCongress(ctx, "Federação Teste", 2026)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := st.AddLocal(ctx, cong, "IP Central")
	if err := st.GenerateTokens(ctx, cong, 20); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPosition(ctx, cong, "Presidente", 1); err != nil {
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
		id, err := st.AddElector(ctx, cong, d.nome, &loc, false, d.nasc)
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
	if err := st.DeclararQuorum(ctx, cong); err != nil {
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
	must(t, c.st.AddPosition(ctx, c.cong, "Vice-presidente", 2))

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
	cong, err := st.SetupCongress(ctx, "Fed Pequena", 2026, []int{2, 5})
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
	// Abrir escrutínio de cargo desativado (Vice, seq 2) é recusado.
	st.DeclararQuorum(ctx, cong)
	for _, p := range all {
		if p.Seq == 2 {
			if _, err := st.AbrirCargo(ctx, cong, p.ID, nil); err == nil {
				t.Fatal("abrir escrutínio de cargo desativado deveria falhar")
			}
		}
	}
}

func TestAbrirCargo_ExigeQuorumDeclarado(t *testing.T) {
	ctx := context.Background()
	st, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { st.Close() })
	cong, _ := st.CreateCongress(ctx, "F", 2026)
	st.AddPosition(ctx, cong, "Presidente", 1)
	poss, _ := st.Positions(ctx, cong)
	if _, err := st.AbrirCargo(ctx, cong, poss[0].ID, nil); err == nil {
		t.Fatal("deveria recusar abrir cargo sem quórum declarado")
	}
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
