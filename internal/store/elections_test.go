package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// --- Reset da Eleição (ADR-0012): esvazia tudo, mas é desfazível ----------

func TestReset_VoltaAoInicioEDesfazivel(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cong, err := st.SetupCongress(ctx, AmbitoFederacao, "UMP", "Federação Teste", 2026, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddElector(ctx, cong, "Ana", nil, nil, true, "1999-01-01"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPIN(ctx, "1234"); err != nil {
		t.Fatal(err)
	}

	if err := st.ResetEleicao(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FirstCongress(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("após o reset não deveria haver congresso, veio %v", err)
	}
	// O PIN e o Histórico sobrevivem ao reset.
	if h, _ := st.PINHash(ctx); h == "" {
		t.Fatal("o PIN não deveria ser apagado pelo reset")
	}
	if _, ok, _ := st.LastOperation(ctx); !ok {
		t.Fatal("o reset deveria estar registrado no histórico")
	}

	// Desfazer o reset traz tudo de volta.
	if err := st.UndoLast(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := st.FirstCongress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c.Nome != "Federação Teste" {
		t.Fatalf("congresso restaurado errado: %+v", c)
	}
	es, err := st.Electors(ctx, c.ID)
	if err != nil || len(es) != 1 {
		t.Fatalf("rol deveria voltar com 1 votante, veio %d (%v)", len(es), err)
	}
}

// --- Gerenciador de Eleições: pasta de .db ---------------------------------

func TestEleicoes_CriarListarAtivaExcluir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m, err := OpenElections(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Dois bancos: um configurado, um em branco.
	a := m.NewFile() // eleicao-001.db
	pa, _ := m.Path(a)
	sa, err := Open(pa)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sa.SetupCongress(ctx, AmbitoLocal, "SAF", "SAF da Igreja Teste", 2026, nil); err != nil {
		t.Fatal(err)
	}
	sa.Close()

	b := m.NewFile()
	if b == a {
		t.Fatalf("NewFile repetiu o nome %q", b)
	}
	pb, _ := m.Path(b)
	sb, err := Open(pb)
	if err != nil {
		t.Fatal(err)
	}
	sb.Close()

	list, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2 eleições, veio %d", len(list))
	}
	byFile := map[string]ElectionInfo{}
	for _, e := range list {
		byFile[e.File] = e
	}
	if e := byFile[a]; !e.Configurada || e.Nome != "SAF da Igreja Teste" || e.Ambito != AmbitoLocal {
		t.Fatalf("retrato errado da eleição configurada: %+v", e)
	}
	if e := byFile[b]; e.Configurada {
		t.Fatalf("eleição em branco veio como configurada: %+v", e)
	}

	// Eleição ativa: meta na pasta.
	if got := m.Active(); got != "" {
		t.Fatalf("sem meta, ativa deveria ser vazia, veio %q", got)
	}
	if err := m.SetActive(a); err != nil {
		t.Fatal(err)
	}
	if got := m.Active(); got != a {
		t.Fatalf("ativa = %q, esperava %q", got, a)
	}

	// Excluir leva o arquivo embora (e a ativa apontando pra ele fica órfã → "").
	if err := m.Delete(a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pa); !os.IsNotExist(err) {
		t.Fatal("o arquivo da eleição deveria ter sido apagado")
	}
	if got := m.Active(); got != "" {
		t.Fatalf("ativa órfã deveria voltar vazia, veio %q", got)
	}
}

func TestEleicoes_NomeDeArquivoInvalido(t *testing.T) {
	m, err := OpenElections(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../fora.db", "sub/x.db", "semextensao", "a.db; rm -rf"} {
		if _, err := m.Path(bad); err == nil {
			t.Fatalf("Path(%q) deveria falhar", bad)
		}
	}
}
