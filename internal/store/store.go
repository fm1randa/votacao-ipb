// Package store concentra o acesso ao SQLite: abertura com WAL, schema embutido e
// as operações de domínio (credenciar, votar, abrir/encerrar escrutínio, apurar).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"math/big"
	"strings"

	_ "modernc.org/sqlite" // driver SQLite puro-Go (sem cgo) -> binário estático
)

//go:embed schema.sql
var schemaSQL string

// Erros de domínio que os handlers HTTP traduzem em mensagens pro eleitor.
var (
	ErrRoundClosed  = errors.New("escrutínio não está aberto")
	ErrInvalidToken = errors.New("token inválido ou não entregue")
	ErrAlreadyVoted = errors.New("este token já votou neste escrutínio")
	ErrInvalidVotee = errors.New("delegado não pode receber voto neste escrutínio")
)

type Store struct {
	db *sql.DB
}

// Open abre (ou cria) o banco, liga WAL e aplica o schema.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir banco: %w", err)
	}
	db.SetMaxOpenConns(1) // 1 escritor; WAL libera leituras paralelas (telão)

	if _, err := db.ExecContext(context.Background(), schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("aplicar schema: %w", err)
	}
	// Migração leve para bancos criados antes da coluna (erro de coluna duplicada é ok).
	db.ExecContext(context.Background(),
		`ALTER TABLE position ADD COLUMN ativo INTEGER NOT NULL DEFAULT 1`)
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---------------------------------------------------------------------------
// Tokens cegos
// ---------------------------------------------------------------------------

const tokenAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // sem I,L,O,0,1 (ambíguos)

// tokenLen: 4 caracteres em caixas OTP — risco de brute-force aceito (ADR-0003).
const tokenLen = 4

func newCode(n int) (string, error) {
	b := make([]byte, n)
	limit := big.NewInt(int64(len(tokenAlphabet)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b[i] = tokenAlphabet[idx.Int64()]
	}
	return string(b), nil
}

// GenerateTokens cria `n` tokens cegos não entregues. Roda uma vez, antes do congresso.
func (s *Store) GenerateTokens(ctx context.Context, congressID int64, n int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := 0; i < n; i++ {
		code, err := newCode(tokenLen)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO token(token, congress_id) VALUES (?, ?)`, code, congressID); err != nil {
			i-- // colisão raríssima: tenta o próximo
			continue
		}
	}
	return tx.Commit()
}

// IssueToken entrega o próximo token livre da pilha e devolve o código. Não mexe
// em presença (ADR-0002) — usado tanto no credenciamento quanto na reemissão.
func (s *Store) IssueToken(ctx context.Context, congressID int64) (string, error) {
	if err := s.snapshotOp(ctx, "Reemitiu token (perda)"); err != nil {
		return "", err
	}
	var code string
	err := s.db.QueryRowContext(ctx, `
		UPDATE token SET entregue = 1, entregue_em = datetime('now')
		WHERE token = (SELECT token FROM token WHERE congress_id = ? AND entregue = 0 LIMIT 1)
		RETURNING token`, congressID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("não há tokens disponíveis na pilha")
	}
	return code, err
}

// TokenIssued diz se o token existe e foi entregue (login da área do delegado).
func (s *Store) TokenIssued(ctx context.Context, token string) (bool, error) {
	var entregue int
	err := s.db.QueryRowContext(ctx, `SELECT entregue FROM token WHERE token = ?`, token).Scan(&entregue)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return entregue == 1, err
}

// HasVoted diz se o token já depositou voto neste escrutínio (estado "já votou").
func (s *Store) HasVoted(ctx context.Context, roundID int64, token string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vote WHERE round_id = ? AND token = ?`, roundID, token).Scan(&n)
	return n > 0, err
}

// ---------------------------------------------------------------------------
// Voto — queima atômica
// ---------------------------------------------------------------------------

// CastVote grava um voto queimando o token. votee==0 com kind "branco"/"nulo".
func (s *Store) CastVote(ctx context.Context, roundID int64, token, kind string, votee int64) error {
	tx, err := s.db.BeginTx(ctx, nil) // _txlock=immediate => lock de escrita já aqui
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1) Escrutínio precisa estar aberto e a eleição não pode estar encerrada.
	var status string
	var encerrada int
	err = tx.QueryRowContext(ctx, `
		SELECT r.status, c.encerrada FROM round r
		JOIN position p ON p.id = r.position_id
		JOIN congress c ON c.id = p.congress_id WHERE r.id = ?`, roundID).Scan(&status, &encerrada)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (status != "aberto" || encerrada == 1)) {
		return ErrRoundClosed
	}
	if err != nil {
		return err
	}

	// 2) Token precisa existir e ter sido entregue.
	var entregue int
	err = tx.QueryRowContext(ctx, `SELECT entregue FROM token WHERE token = ?`, token).Scan(&entregue)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && entregue == 0) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}

	// 3) Se for candidato: precisa estar presente E (se o escrutínio é restrito)
	//    constar no conjunto votável (indicação ou runoff).
	var voteeArg interface{}
	if kind == "candidato" {
		ok, err := voteeIsAllowed(ctx, tx, roundID, votee)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidVotee
		}
		voteeArg = votee
	}

	// 4) INSERT — a UNIQUE(round_id, token) é a queima.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO vote(round_id, token, kind, votee_elector_id) VALUES (?,?,?,?)`,
		roundID, token, kind, voteeArg)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyVoted
		}
		return err
	}
	return tx.Commit()
}

// voteeIsAllowed: o delegado votado precisa estar presente, não pode já ter sido
// eleito para outro cargo (não se acumula cargos na diretoria) e, se o escrutínio
// tem conjunto restrito (round_candidate), precisa pertencer a ele.
func voteeIsAllowed(ctx context.Context, tx *sql.Tx, roundID, votee int64) (bool, error) {
	var presente int
	err := tx.QueryRowContext(ctx, `SELECT presente FROM elector WHERE id = ?`, votee).Scan(&presente)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && presente == 0) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var jaEleito int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM position
		WHERE eleito_elector_id = ? AND congress_id = (
		  SELECT p.congress_id FROM round r JOIN position p ON p.id = r.position_id WHERE r.id = ?)`,
		votee, roundID).Scan(&jaEleito); err != nil {
		return false, err
	}
	if jaEleito > 0 {
		return false, nil
	}
	var restrito int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM round_candidate WHERE round_id = ?`, roundID).Scan(&restrito); err != nil {
		return false, err
	}
	if restrito == 0 {
		return true, nil // sem restrição: qualquer presente pode receber voto
	}
	var inSet int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM round_candidate WHERE round_id = ? AND elector_id = ?`,
		roundID, votee).Scan(&inSet); err != nil {
		return false, err
	}
	return inSet > 0, nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
