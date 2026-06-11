package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

// abreRound monta uma Plenária local pronta para votar: cargos do preset,
// 5 presentes elegíveis (satisfaz quórum e o gate elegíveis≥cargos), abertura
// declarada e o 1º cargo aberto. Devolve o congresso, o round aberto e um token
// já entregue (apto a votar).
func abreRound(t *testing.T, st *Store) (cong, round int64, token string) {
	t.Helper()
	ctx := context.Background()
	cong, err := st.SetupCongress(ctx, AmbitoLocal, "UMP", "Plenária Teste", 2026, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ { // 5 jovens presentes > qualquer preset local (3 cargos)
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
	return cong, r.ID, token
}

func openStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// --- Token + PIN ----------------------------------------------------------

func TestCheckPIN(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	if ok, err := st.CheckPIN(ctx, "1234"); err != nil || ok {
		t.Fatalf("sem PIN salvo esperava false sem erro, veio ok=%v err=%v", ok, err)
	}
	if err := st.SetPIN(ctx, "1234"); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.CheckPIN(ctx, "1234"); err != nil || !ok {
		t.Fatalf("PIN certo esperava true, veio ok=%v err=%v", ok, err)
	}
	if ok, err := st.CheckPIN(ctx, "9999"); err != nil || ok {
		t.Fatalf("PIN errado esperava false, veio ok=%v err=%v", ok, err)
	}
}

func TestIssueToken(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	cong, err := st.CreateCongress(ctx, AmbitoLocal, "UMP", "Plenária Teste", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.GenerateTokens(ctx, cong, 2); err != nil {
		t.Fatal(err)
	}

	a, err := st.IssueToken(ctx, cong)
	if err != nil || a == "" {
		t.Fatalf("1ª emissão esperava código, veio code=%q err=%v", a, err)
	}
	b, err := st.IssueToken(ctx, cong)
	if err != nil || b == "" {
		t.Fatalf("2ª emissão esperava código, veio code=%q err=%v", b, err)
	}
	if a == b {
		t.Fatalf("emissões devem ser distintas, ambas %q", a)
	}
	if _, err := st.IssueToken(ctx, cong); err == nil {
		t.Fatal("pilha vazia: esperava erro na 3ª emissão, veio nil")
	}
}

func TestTokenIssued(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	cong, err := st.CreateCongress(ctx, AmbitoLocal, "UMP", "Plenária Teste", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.GenerateTokens(ctx, cong, 2); err != nil {
		t.Fatal(err)
	}
	code, err := st.IssueToken(ctx, cong)
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := st.TokenIssued(ctx, code); err != nil || !ok {
		t.Fatalf("token entregue esperava true, veio ok=%v err=%v", ok, err)
	}
	if ok, err := st.TokenIssued(ctx, "ZZZZ"); err != nil || ok {
		t.Fatalf("token inexistente esperava false sem erro, veio ok=%v err=%v", ok, err)
	}
}

func TestHasVoted(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	_, round, token := abreRound(t, st)

	if ok, err := st.HasVoted(ctx, round, token); err != nil || ok {
		t.Fatalf("antes de votar esperava false, veio ok=%v err=%v", ok, err)
	}
	if err := st.CastVote(ctx, round, token, "branco", 0); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.HasVoted(ctx, round, token); err != nil || !ok {
		t.Fatalf("depois de votar esperava true, veio ok=%v err=%v", ok, err)
	}
}

// --- Ciclo de vida + leituras de estado -----------------------------------

func TestEncerrarReabrirEleicao(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	cong, err := st.SetupCongress(ctx, AmbitoLocal, "UMP", "Plenária Teste", 2026, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.EncerrarEleicao(ctx, cong); err != nil {
		t.Fatal(err)
	}
	c, err := st.FirstCongress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Encerrada {
		t.Fatal("após encerrar esperava Encerrada=true")
	}

	if err := st.ReabrirEleicao(ctx, cong); err != nil {
		t.Fatal(err)
	}
	c, err = st.FirstCongress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c.Encerrada {
		t.Fatal("após reabrir esperava Encerrada=false")
	}
}

func TestEncerradaBloqueiaVoto(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	cong, round, token := abreRound(t, st)

	if err := st.EncerrarEleicao(ctx, cong); err != nil {
		t.Fatal(err)
	}
	err := st.CastVote(ctx, round, token, "branco", 0)
	if !errors.Is(err, ErrRoundClosed) {
		t.Fatalf("eleição encerrada deve bloquear voto com ErrRoundClosed, veio %v", err)
	}
}

func TestOpenRoundECurrentPosition(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	cong, err := st.SetupCongress(ctx, AmbitoLocal, "UMP", "Plenária Teste", 2026, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := st.OpenRound(ctx, cong); err != nil || ok {
		t.Fatalf("sem cargo aberto esperava ok=false, veio ok=%v err=%v", ok, err)
	}

	// Agora monta um cenário com round aberto (mesma sequência de abreRound).
	st2 := openStore(t)
	cong2, round2, _ := abreRound(t, st2)

	r, _, ok, err := st2.OpenRound(ctx, cong2)
	if err != nil || !ok {
		t.Fatalf("com cargo aberto esperava ok=true, veio ok=%v err=%v", ok, err)
	}
	if r.ID != round2 {
		t.Fatalf("OpenRound devolveu round %d, esperava %d", r.ID, round2)
	}

	pos, err := st2.Positions(ctx, cong2)
	if err != nil || len(pos) == 0 {
		t.Fatalf("sem cargos: %v", err)
	}
	cp, ok, err := st2.CurrentPosition(ctx, cong2)
	if err != nil || !ok {
		t.Fatalf("CurrentPosition esperava ok=true, veio ok=%v err=%v", ok, err)
	}
	if cp.Nome != pos[0].Nome {
		t.Fatalf("CurrentPosition devolveu %q, esperava %q", cp.Nome, pos[0].Nome)
	}
}
