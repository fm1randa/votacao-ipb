package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ---------------------------------------------------------------------------
// Tipos
// ---------------------------------------------------------------------------

type Congress struct {
	ID              int64
	Federacao       string
	Ano             int
	QuorumDeclarado bool
	Encerrada       bool
}

type Local struct {
	ID   int64
	Nome string
}

type Elector struct {
	ID          int64
	Nome        string
	LocalID     sql.NullInt64
	LocalNome   string
	Nato        bool
	Nascimento  sql.NullString
	Credenciado bool
	Presente    bool
}

// QuorumInfo alimenta o painel da Mesa: representação por UMP local (oficial,
// Art. 49a) e o headcount de presentes.
type QuorumInfo struct {
	Presentes           int // pessoas presentes (headcount)
	Credenciados        int // já receberam token alguma vez
	LocaisTotal         int
	LocaisRepresentadas int  // locais com ≥1 delegado presente
	LocaisOk            bool // mais da metade das locais
	TokensEntregues     int
	Reemissoes          int // tokens entregues além dos credenciados (perdas)
}

// ---------------------------------------------------------------------------
// Congresso, locais, rol
// ---------------------------------------------------------------------------

func (s *Store) CreateCongress(ctx context.Context, federacao string, ano int) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO congress(federacao, ano) VALUES (?, ?)`, federacao, ano)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FirstCongress devolve o congresso (assume-se um por banco).
func (s *Store) FirstCongress(ctx context.Context) (Congress, error) {
	var c Congress
	err := s.db.QueryRowContext(ctx,
		`SELECT id, federacao, ano, quorum_declarado, encerrada FROM congress ORDER BY id LIMIT 1`).
		Scan(&c.ID, &c.Federacao, &c.Ano, &c.QuorumDeclarado, &c.Encerrada)
	return c, err
}

func (s *Store) DeclararQuorum(ctx context.Context, congressID int64) error {
	if err := s.snapshotOp(ctx, "Declarou o quórum"); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE congress SET quorum_declarado = 1 WHERE id = ?`, congressID)
	return err
}

// Cargos da Diretoria da Federação UMP, na ordem de eleição (GTSI Art. 26a).
var DefaultPositions = []string{"Presidente", "Vice-presidente", "Secretário Executivo",
	"1º Secretário", "2º Secretário", "Tesoureiro"}

// SetupCongress cria o congresso com os 6 cargos do GTSI e a pilha inicial de
// tokens — o passo 2 do wizard. `disabledSeqs` desativa cargos opcionais
// (federações menores; SPEC §3.5).
func (s *Store) SetupCongress(ctx context.Context, federacao string, ano int, disabledSeqs []int) (int64, error) {
	if err := s.snapshotOp(ctx, "Configurou o congresso"); err != nil {
		return 0, err
	}
	id, err := s.CreateCongress(ctx, federacao, ano)
	if err != nil {
		return 0, err
	}
	off := map[int]bool{}
	for _, seq := range disabledSeqs {
		if optionalSeqs[seq] {
			off[seq] = true
		}
	}
	for i, nome := range DefaultPositions {
		if err := s.AddPosition(ctx, id, nome, i+1); err != nil {
			return 0, err
		}
		if off[i+1] {
			if _, err := s.db.ExecContext(ctx,
				`UPDATE position SET ativo = 0 WHERE congress_id = ? AND seq = ?`, id, i+1); err != nil {
				return 0, err
			}
		}
	}
	return id, s.GenerateTokens(ctx, id, 100)
}

// UpdateCongress altera federação/ano (tela Ajustes).
func (s *Store) UpdateCongress(ctx context.Context, id int64, federacao string, ano int) error {
	if err := s.snapshotOp(ctx, "Alterou dados do congresso"); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE congress SET federacao = ?, ano = ? WHERE id = ?`, federacao, ano, id)
	return err
}

// Locals lista as UMPs locais (para o datalist dos formulários).
func (s *Store) Locals(ctx context.Context, congressID int64) ([]Local, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, nome FROM local WHERE congress_id = ? ORDER BY nome`, congressID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Local
	for rows.Next() {
		var l Local
		if err := rows.Scan(&l.ID, &l.Nome); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LocalByNameOrCreate acha a UMP local pelo nome (sem case) ou cria uma nova.
func (s *Store) LocalByNameOrCreate(ctx context.Context, congressID int64, nome string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM local WHERE congress_id = ? AND lower(nome) = lower(?)`,
		congressID, nome).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.AddLocal(ctx, congressID, nome)
	}
	return id, err
}

func (s *Store) AddLocal(ctx context.Context, congressID int64, nome string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO local(congress_id, nome) VALUES (?, ?)`, congressID, nome)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddElector cadastra um delegado. localID nulo + nato=true para membros natos.
func (s *Store) AddElector(ctx context.Context, congressID int64, nome string, localID *int64, nato bool, nascimento string) (int64, error) {
	var loc interface{}
	if localID != nil {
		loc = *localID
	}
	var nasc interface{}
	if nascimento != "" {
		nasc = nascimento
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO elector(congress_id, nome, local_id, nato, nascimento) VALUES (?,?,?,?,?)`,
		congressID, nome, loc, boolToInt(nato), nasc)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ElectorInput é uma linha de cadastro (form individual ou colar lista).
type ElectorInput struct {
	Nome, LocalNome, Nascimento string
	Nato                        bool
}

// ImportElectors cadastra delegados numa só operação do log (opDesc descreve:
// "Adicionou delegado X" ou "Importou N delegados"). Igrejas inexistentes são
// criadas; garante a pilha de tokens ao final.
func (s *Store) ImportElectors(ctx context.Context, congressID int64, items []ElectorInput, opDesc string) error {
	if len(items) == 0 {
		return errors.New("nenhum delegado para cadastrar")
	}
	if err := s.snapshotOp(ctx, opDesc); err != nil {
		return err
	}
	for _, it := range items {
		var localID *int64
		if !it.Nato && strings.TrimSpace(it.LocalNome) != "" {
			id, err := s.LocalByNameOrCreate(ctx, congressID, strings.TrimSpace(it.LocalNome))
			if err != nil {
				return err
			}
			localID = &id
		}
		if _, err := s.AddElector(ctx, congressID, strings.TrimSpace(it.Nome), localID, it.Nato, it.Nascimento); err != nil {
			return err
		}
	}
	return s.EnsureTokens(ctx, congressID)
}

// UpdateElector edita nome/igreja/nato/nascimento de um delegado.
func (s *Store) UpdateElector(ctx context.Context, congressID, id int64, in ElectorInput) error {
	if err := s.snapshotOp(ctx, "Editou delegado "+strings.TrimSpace(in.Nome)); err != nil {
		return err
	}
	var localID interface{}
	if !in.Nato && strings.TrimSpace(in.LocalNome) != "" {
		lid, err := s.LocalByNameOrCreate(ctx, congressID, strings.TrimSpace(in.LocalNome))
		if err != nil {
			return err
		}
		localID = lid
	}
	var nasc interface{}
	if in.Nascimento != "" {
		nasc = in.Nascimento
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE elector SET nome = ?, local_id = ?, nato = ?, nascimento = ? WHERE id = ?`,
		strings.TrimSpace(in.Nome), localID, boolToInt(in.Nato), nasc, id)
	return err
}

// DeleteElector remove um delegado que NUNCA foi credenciado (depois disso,
// usa-se o registro de saída — a presença é parte da história da eleição).
func (s *Store) DeleteElector(ctx context.Context, id int64) error {
	var nome string
	var credenciado int
	err := s.db.QueryRowContext(ctx,
		`SELECT nome, credenciado FROM elector WHERE id = ?`, id).Scan(&nome, &credenciado)
	if err != nil {
		return err
	}
	if credenciado == 1 {
		return errors.New("delegado já credenciado — registre a saída em vez de remover")
	}
	if err := s.snapshotOp(ctx, "Removeu delegado "+nome); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM elector WHERE id = ?`, id)
	return err
}

// EnsureTokens garante folga na pilha: livres ≥ não-credenciados + 20.
func (s *Store) EnsureTokens(ctx context.Context, congressID int64) error {
	var livres, pendentes int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM token WHERE congress_id = ? AND entregue = 0),
		  (SELECT COUNT(*) FROM elector WHERE congress_id = ? AND credenciado = 0)`,
		congressID, congressID).Scan(&livres, &pendentes); err != nil {
		return err
	}
	if falta := pendentes + 20 - livres; falta > 0 {
		return s.GenerateTokens(ctx, congressID, falta)
	}
	return nil
}

// Electors lista o rol com o nome da UMP local.
func (s *Store) Electors(ctx context.Context, congressID int64) ([]Elector, error) {
	return s.queryElectors(ctx,
		`SELECT e.id, e.nome, e.local_id, COALESCE(l.nome,''), e.nato, e.nascimento, e.credenciado, e.presente
		 FROM elector e LEFT JOIN local l ON l.id = e.local_id
		 WHERE e.congress_id = ? ORDER BY e.nome`, congressID)
}

// GetElector devolve um delegado pelo id (com o nome da UMP local).
func (s *Store) GetElector(ctx context.Context, id int64) (Elector, error) {
	els, err := s.queryElectors(ctx,
		`SELECT e.id, e.nome, e.local_id, COALESCE(l.nome,''), e.nato, e.nascimento, e.credenciado, e.presente
		 FROM elector e LEFT JOIN local l ON l.id = e.local_id WHERE e.id = ?`, id)
	if err != nil {
		return Elector{}, err
	}
	if len(els) == 0 {
		return Elector{}, sql.ErrNoRows
	}
	return els[0], nil
}

// PresentElectors lista só os presentes — o conjunto votável padrão.
func (s *Store) PresentElectors(ctx context.Context, congressID int64) ([]Elector, error) {
	return s.queryElectors(ctx,
		`SELECT e.id, e.nome, e.local_id, COALESCE(l.nome,''), e.nato, e.nascimento, e.credenciado, e.presente
		 FROM elector e LEFT JOIN local l ON l.id = e.local_id
		 WHERE e.congress_id = ? AND e.presente = 1 ORDER BY e.nome`, congressID)
}

func (s *Store) queryElectors(ctx context.Context, query string, args ...interface{}) ([]Elector, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Elector
	for rows.Next() {
		var e Elector
		var nato, cred, pres int
		if err := rows.Scan(&e.ID, &e.Nome, &e.LocalID, &e.LocalNome, &nato, &e.Nascimento, &cred, &pres); err != nil {
			return nil, err
		}
		e.Nato, e.Credenciado, e.Presente = nato == 1, cred == 1, pres == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Credenciamento e presença (ADR-0002: presença é da pessoa, não do token)
// ---------------------------------------------------------------------------

// Credenciar marca o delegado como presente/credenciado E entrega um token cego,
// numa transação. Devolve o código do token.
func (s *Store) Credenciar(ctx context.Context, congressID, electorID int64) (string, error) {
	var nome string
	s.db.QueryRowContext(ctx, `SELECT nome FROM elector WHERE id = ?`, electorID).Scan(&nome)
	if err := s.snapshotOp(ctx, "Credenciou "+nome); err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE elector SET credenciado = 1, presente = 1 WHERE id = ? AND congress_id = ?`,
		electorID, congressID)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", errors.New("delegado não encontrado")
	}

	var code string
	err = tx.QueryRowContext(ctx, `
		UPDATE token SET entregue = 1, entregue_em = datetime('now')
		WHERE token = (SELECT token FROM token WHERE congress_id = ? AND entregue = 0 LIMIT 1)
		RETURNING token`, congressID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("não há tokens disponíveis na pilha")
	}
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return code, nil
}

// SetPresente registra saída (false) ou reentrada (true) de um delegado já credenciado.
func (s *Store) SetPresente(ctx context.Context, electorID int64, presente bool) error {
	var nome string
	s.db.QueryRowContext(ctx, `SELECT nome FROM elector WHERE id = ?`, electorID).Scan(&nome)
	desc := "Registrou reentrada de " + nome
	if !presente {
		desc = "Registrou saída de " + nome
	}
	if err := s.snapshotOp(ctx, desc); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE elector SET presente = ? WHERE id = ?`, boolToInt(presente), electorID)
	return err
}

// Quorum calcula a representação por UMP local e o headcount.
func (s *Store) Quorum(ctx context.Context, congressID int64) (QuorumInfo, error) {
	var q QuorumInfo
	row := s.db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM elector WHERE congress_id=? AND presente=1),
		  (SELECT COUNT(*) FROM elector WHERE congress_id=? AND credenciado=1),
		  (SELECT COUNT(*) FROM local   WHERE congress_id=?),
		  (SELECT COUNT(DISTINCT local_id) FROM elector WHERE congress_id=? AND presente=1 AND local_id IS NOT NULL),
		  (SELECT COUNT(*) FROM token WHERE congress_id=? AND entregue=1)`,
		congressID, congressID, congressID, congressID, congressID)
	if err := row.Scan(&q.Presentes, &q.Credenciados, &q.LocaisTotal, &q.LocaisRepresentadas, &q.TokensEntregues); err != nil {
		return q, err
	}
	q.LocaisOk = q.LocaisRepresentadas*2 > q.LocaisTotal
	q.Reemissoes = q.TokensEntregues - q.Credenciados
	if q.Reemissoes < 0 {
		q.Reemissoes = 0
	}
	return q, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
