package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Log de operações restaurável (ADR-0006), inspirado no jj.
//
// Cada operação da Mesa grava um snapshot JSON do estado de DOMÍNIO de antes da
// ação. Desfazer/Restaurar recarregam um snapshot. Restaurar é também uma operação
// (logo, reversível). `operation` e `setting` ficam de fora dos snapshots.

// Tabelas de domínio capturadas no snapshot (ordem filhos→pais ajuda no DELETE).
var domainTables = []string{"vote", "round_candidate", "round", "position", "token", "elector", "local", "congress"}

type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// snapshotDomain serializa todas as tabelas de domínio num JSON.
func snapshotDomain(ctx context.Context, q rowQuerier) (string, error) {
	out := map[string][]map[string]any{}
	for _, t := range domainTables {
		rows, err := q.QueryContext(ctx, "SELECT * FROM "+t)
		if err != nil {
			return "", err
		}
		cols, _ := rows.Columns()
		var list []map[string]any
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return "", err
			}
			m := map[string]any{}
			for i, c := range cols {
				v := vals[i]
				if b, ok := v.([]byte); ok {
					v = string(b)
				}
				m[c] = v
			}
			list = append(list, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return "", err
		}
		out[t] = list
	}
	b, err := json.Marshal(out)
	return string(b), err
}

// restoreDomain limpa as tabelas de domínio e recarrega do snapshot, numa transação.
func restoreDomain(ctx context.Context, tx *sql.Tx, snap string) error {
	dec := json.NewDecoder(strings.NewReader(snap))
	dec.UseNumber()
	var data map[string][]map[string]any
	if err := dec.Decode(&data); err != nil {
		return err
	}
	for _, t := range domainTables { // filhos→pais
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
	}
	for i := len(domainTables) - 1; i >= 0; i-- { // pais→filhos no insert
		t := domainTables[i]
		for _, row := range data[t] {
			cols := make([]string, 0, len(row))
			ph := make([]string, 0, len(row))
			args := make([]any, 0, len(row))
			for c, v := range row {
				cols = append(cols, c)
				ph = append(ph, "?")
				args = append(args, normalizeJSON(v))
			}
			q := "INSERT INTO " + t + " (" + strings.Join(cols, ",") + ") VALUES (" + strings.Join(ph, ",") + ")"
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return fmt.Errorf("restore insert %s: %w", t, err)
			}
		}
	}
	return nil
}

// normalizeJSON converte json.Number em int64/float64 (preserva IDs inteiros).
func normalizeJSON(v any) any {
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i
		}
		f, _ := n.Float64()
		return f
	}
	return v
}

// recordOp grava uma operação (snapshot do estado ANTES) dentro da transação `tx`.
// Toda mutação da Mesa chama isto no início da sua transação.
func recordOp(ctx context.Context, tx *sql.Tx, descricao string) error {
	snap, err := snapshotDomain(ctx, tx)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO operation(descricao, snapshot) VALUES (?, ?)`, descricao, snap)
	return err
}

// snapshotOp grava um checkpoint (operação + snapshot do estado atual) antes de uma
// mutação. As mutações simples da Mesa chamam isto no início.
func (s *Store) snapshotOp(ctx context.Context, descricao string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := recordOp(ctx, tx, descricao); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Histórico, desfazer, restaurar
// ---------------------------------------------------------------------------

type Operation struct {
	ID        int64
	CriadoEm  string
	Descricao string
}

func (s *Store) Operations(ctx context.Context) ([]Operation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, criado_em, descricao FROM operation ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		var o Operation
		if err := rows.Scan(&o.ID, &o.CriadoEm, &o.Descricao); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Resumo de estado (para o "diff" do modal de restaurar)
// ---------------------------------------------------------------------------

type CargoState struct {
	Nome   string
	Status string // pendente | em_eleicao | decidido
	Eleito string
}

type StateSummary struct {
	Encerrada    bool
	Presentes    int
	Credenciados int
	LocaisRepr   int
	LocaisTotal  int
	TotalVotos   int
	Cargos       []CargoState
}

// SummarizeCurrent resume o estado VIVO (reaproveita o snapshot do banco).
func (s *Store) SummarizeCurrent(ctx context.Context) (StateSummary, error) {
	snap, err := snapshotDomain(ctx, s.db)
	if err != nil {
		return StateSummary{}, err
	}
	return summarizeJSON(snap)
}

// SummarizeSnapshot resume o estado guardado numa operação. Devolve também a descrição.
func (s *Store) SummarizeSnapshot(ctx context.Context, opID int64) (StateSummary, string, error) {
	var snap, desc string
	err := s.db.QueryRowContext(ctx,
		`SELECT snapshot, descricao FROM operation WHERE id = ?`, opID).Scan(&snap, &desc)
	if errors.Is(err, sql.ErrNoRows) {
		return StateSummary{}, "", errors.New("operação não encontrada")
	}
	if err != nil {
		return StateSummary{}, "", err
	}
	sum, err := summarizeJSON(snap)
	return sum, desc, err
}

func summarizeJSON(snap string) (StateSummary, error) {
	dec := json.NewDecoder(strings.NewReader(snap))
	dec.UseNumber()
	var d map[string][]map[string]any
	if err := dec.Decode(&d); err != nil {
		return StateSummary{}, err
	}
	var sum StateSummary
	if len(d["congress"]) > 0 {
		sum.Encerrada = jint(d["congress"][0]["encerrada"]) == 1
	}
	sum.LocaisTotal = len(d["local"])
	sum.TotalVotos = len(d["vote"])

	names := map[int64]string{}
	repr := map[int64]bool{}
	for _, e := range d["elector"] {
		names[jint64(e["id"])] = jstr(e["nome"])
		if jint(e["presente"]) == 1 {
			sum.Presentes++
			if lid := e["local_id"]; lid != nil {
				repr[jint64(lid)] = true
			}
		}
		if jint(e["credenciado"]) == 1 {
			sum.Credenciados++
		}
	}
	sum.LocaisRepr = len(repr)

	type pos struct {
		seq          int
		nome, status string
		eleito       string
	}
	var ps []pos
	for _, p := range d["position"] {
		pp := pos{seq: jint(p["seq"]), nome: jstr(p["nome"]), status: jstr(p["status"])}
		if eid := p["eleito_elector_id"]; eid != nil {
			pp.eleito = names[jint64(eid)]
		}
		ps = append(ps, pp)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].seq < ps[j].seq })
	for _, p := range ps {
		sum.Cargos = append(sum.Cargos, CargoState{Nome: p.nome, Status: p.status, Eleito: p.eleito})
	}
	return sum, nil
}

func jint(v any) int {
	if n, ok := v.(json.Number); ok {
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}
func jint64(v any) int64 {
	if n, ok := v.(json.Number); ok {
		i, _ := n.Int64()
		return i
	}
	return 0
}
func jstr(v any) string { s, _ := v.(string); return s }

// LastOperation devolve a operação mais recente (para o "Desfazer").
func (s *Store) LastOperation(ctx context.Context) (Operation, bool, error) {
	var o Operation
	err := s.db.QueryRowContext(ctx,
		`SELECT id, criado_em, descricao FROM operation ORDER BY id DESC LIMIT 1`).
		Scan(&o.ID, &o.CriadoEm, &o.Descricao)
	if errors.Is(err, sql.ErrNoRows) {
		return o, false, nil
	}
	return o, err == nil, err
}

// RestoreToOp recarrega o snapshot da operação `opID`. O estado atual é gravado
// antes (como nova operação), de modo que o próprio restaurar é reversível.
func (s *Store) RestoreToOp(ctx context.Context, opID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var desc, snap string
	err = tx.QueryRowContext(ctx, `SELECT descricao, snapshot FROM operation WHERE id = ?`, opID).
		Scan(&desc, &snap)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("operação não encontrada")
	}
	if err != nil {
		return err
	}
	if err := recordOp(ctx, tx, "Restaurou: "+desc); err != nil {
		return err
	}
	if err := restoreDomain(ctx, tx, snap); err != nil {
		return err
	}
	return tx.Commit()
}

// UndoLast desfaz a última operação (restaura o snapshot dela = estado anterior).
func (s *Store) UndoLast(ctx context.Context) error {
	op, ok, err := s.LastOperation(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("nada para desfazer")
	}
	return s.RestoreToOp(ctx, op.ID)
}

// ---------------------------------------------------------------------------
// Ciclo de vida da eleição
// ---------------------------------------------------------------------------

// ReiniciarEleicao apaga escrutínios+votos e zera presença/credenciamento,
// mantendo congresso, cargos, rol e tokens. Restaurável (grava operação antes).
func (s *Store) ReiniciarEleicao(ctx context.Context, congressID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := recordOp(ctx, tx, "Reiniciou a eleição"); err != nil {
		return err
	}
	stmts := []string{
		`DELETE FROM vote`,
		`DELETE FROM round_candidate`,
		`DELETE FROM round`,
		`UPDATE elector SET presente = 0, credenciado = 0 WHERE congress_id = ?`,
		`UPDATE token SET entregue = 0, entregue_em = NULL WHERE congress_id = ?`,
		`UPDATE position SET status = 'pendente', eleito_elector_id = NULL WHERE congress_id = ?`,
		`UPDATE congress SET abertura_declarada = 0, encerrada = 0 WHERE id = ?`,
	}
	for _, q := range stmts {
		if strings.Contains(q, "?") {
			if _, err := tx.ExecContext(ctx, q, congressID); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetEleicao esvazia a Eleição por completo — congresso, rol, cargos, tokens,
// votos — e volta ao assistente de configuração. Diferente do Reiniciar (que
// preserva rol e cargos), aqui só sobram o Histórico e o PIN: a operação grava
// o retrato anterior, então o reset é desfazível pelo Histórico.
func (s *Store) ResetEleicao(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := recordOp(ctx, tx, "Resetou a eleição (voltou ao estado inicial)"); err != nil {
		return err
	}
	for _, t := range domainTables { // filhos→pais
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EncerrarEleicao marca a eleição como encerrada (só leitura). Restaurável.
func (s *Store) EncerrarEleicao(ctx context.Context, congressID int64) error {
	return s.lifecycleFlag(ctx, congressID, "Encerrou a eleição", 1)
}

// ReabrirEleicao reabre uma eleição encerrada.
func (s *Store) ReabrirEleicao(ctx context.Context, congressID int64) error {
	return s.lifecycleFlag(ctx, congressID, "Reabriu a eleição", 0)
}

func (s *Store) lifecycleFlag(ctx context.Context, congressID int64, desc string, encerrada int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := recordOp(ctx, tx, desc); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE congress SET encerrada = ? WHERE id = ?`, encerrada, congressID); err != nil {
		return err
	}
	return tx.Commit()
}
