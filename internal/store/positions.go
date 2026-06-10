package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Position struct {
	ID         int64
	Nome       string
	Role       string // papel no preset (presidente, vice, ... — ambito.go)
	Seq        int
	Ativo      bool
	Optional   bool   // desativável neste âmbito (derivado do preset)
	Status     string // pendente | em_eleicao | decidido
	EleitoID   sql.NullInt64
	EleitoNome string
}

type Round struct {
	ID          int64
	PositionID  int64
	Numero      int
	Status      string // aberto | encerrado
	Runoff      bool
	EncerradoEm sql.NullString
}

const MaxRounds = 3 // 1º/2º plenos; 3º é runoff (Art. 91e)

// ---------------------------------------------------------------------------
// Cargos
// ---------------------------------------------------------------------------

func (s *Store) AddPosition(ctx context.Context, congressID int64, nome, role string, seq int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO position(congress_id, nome, role, seq) VALUES (?, ?, ?, ?)`,
		congressID, nome, role, seq)
	return err
}

// Positions lista os cargos ATIVOS (a eleição em si). Para configuração, use
// AllPositions. Nomes já vêm ajustados (1º Secretário → "Secretário" sem o 2º).
func (s *Store) Positions(ctx context.Context, congressID int64) ([]Position, error) {
	all, err := s.AllPositions(ctx, congressID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, p := range all {
		if p.Ativo {
			out = append(out, p)
		}
	}
	return out, nil
}

// AllPositions lista todos os cargos, ativos e desativados (telas de configuração).
func (s *Store) AllPositions(ctx context.Context, congressID int64) ([]Position, error) {
	var ambito, sociedade string
	if err := s.db.QueryRowContext(ctx,
		`SELECT ambito, sociedade FROM congress WHERE id = ?`, congressID).
		Scan(&ambito, &sociedade); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.nome, p.role, p.seq, p.ativo, p.status, p.eleito_elector_id, COALESCE(e.nome,'')
		FROM position p LEFT JOIN elector e ON e.id = p.eleito_elector_id
		WHERE p.congress_id = ? ORDER BY p.seq`, congressID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Position
	segundoAtivo := false
	for rows.Next() {
		var p Position
		var ativo int
		if err := rows.Scan(&p.ID, &p.Nome, &p.Role, &p.Seq, &ativo, &p.Status, &p.EleitoID, &p.EleitoNome); err != nil {
			return nil, err
		}
		p.Ativo = ativo == 1
		p.Optional = OptionalRole(ambito, sociedade, p.Role)
		if p.Role == RoleSegundoSec && p.Ativo {
			segundoAtivo = true
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sem o 2º Secretário, o 1º exibe-se apenas como "Secretário(a)" (SPEC §3.5).
	if !segundoAtivo {
		for i := range out {
			if out[i].Role == RolePrimeiroSec {
				out[i].Nome = secretarioSolo(out[i].Nome)
			}
		}
	}
	return out, nil
}

// secretarioSolo tira o ordinal do 1º Secretário ("1º Secretário"→"Secretário",
// "1ª Secretária"→"Secretária") quando o 2º está desativado.
func secretarioSolo(nome string) string {
	nome = strings.TrimPrefix(nome, "1º ")
	return strings.TrimPrefix(nome, "1ª ")
}

func (s *Store) GetPosition(ctx context.Context, id int64) (Position, error) {
	var p Position
	var ativo int
	var congressID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id, p.nome, p.role, p.seq, p.ativo, p.congress_id, p.status, p.eleito_elector_id, COALESCE(e.nome,'')
		FROM position p LEFT JOIN elector e ON e.id = p.eleito_elector_id
		WHERE p.id = ?`, id).
		Scan(&p.ID, &p.Nome, &p.Role, &p.Seq, &ativo, &congressID, &p.Status, &p.EleitoID, &p.EleitoNome)
	if err != nil {
		return p, err
	}
	p.Ativo = ativo == 1
	if p.Role == RolePrimeiroSec {
		var segundoAtivo int
		s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM position WHERE congress_id = ? AND role = ? AND ativo = 1`,
			congressID, RoleSegundoSec).Scan(&segundoAtivo)
		if segundoAtivo == 0 {
			p.Nome = secretarioSolo(p.Nome)
		}
	}
	return p, nil
}

// SetPositionAtivo liga/desliga um cargo opcional. Desativar exige cargo pendente
// (decidido/em eleição → desfaça pelo Histórico antes).
func (s *Store) SetPositionAtivo(ctx context.Context, positionID int64, ativo bool) error {
	var nome, role, status, ambito, sociedade string
	var atual int
	err := s.db.QueryRowContext(ctx, `
		SELECT p.nome, p.role, p.status, p.ativo, c.ambito, c.sociedade
		FROM position p JOIN congress c ON c.id = p.congress_id WHERE p.id = ?`, positionID).
		Scan(&nome, &role, &status, &atual, &ambito, &sociedade)
	if err != nil {
		return err
	}
	if (atual == 1) == ativo {
		return nil // sem mudança
	}
	if !OptionalRole(ambito, sociedade, role) {
		return errors.New(nome + " é obrigatório neste âmbito e não pode ser desativado")
	}
	if !ativo && status != "pendente" {
		return errors.New(nome + " está " + status + " — desfaça pelo Histórico antes de desativar")
	}
	desc := "Desativou o cargo " + nome
	if ativo {
		desc = "Reativou o cargo " + nome
	}
	if err := s.snapshotOp(ctx, desc); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE position SET ativo = ? WHERE id = ?`, boolToInt(ativo), positionID)
	return err
}

// ---------------------------------------------------------------------------
// Escrutínios
// ---------------------------------------------------------------------------

func (s *Store) GetRound(ctx context.Context, id int64) (Round, error) {
	var r Round
	var runoff int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, position_id, numero, status, runoff, encerrado_em FROM round WHERE id = ?`, id).
		Scan(&r.ID, &r.PositionID, &r.Numero, &r.Status, &runoff, &r.EncerradoEm)
	r.Runoff = runoff == 1
	return r, err
}

// CurrentRound devolve o escrutínio mais recente de um cargo (aberto ou não).
func (s *Store) CurrentRound(ctx context.Context, positionID int64) (Round, error) {
	var r Round
	var runoff int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, position_id, numero, status, runoff, encerrado_em FROM round
		 WHERE position_id = ? ORDER BY numero DESC LIMIT 1`, positionID).
		Scan(&r.ID, &r.PositionID, &r.Numero, &r.Status, &runoff, &r.EncerradoEm)
	r.Runoff = runoff == 1
	return r, err
}

// OpenRoundForElection devolve o escrutínio aberto de uma eleição/congresso, se houver.
func (s *Store) OpenRound(ctx context.Context, congressID int64) (Round, Position, bool, error) {
	var r Round
	var p Position
	var runoff int
	err := s.db.QueryRowContext(ctx, `
		SELECT r.id, r.position_id, r.numero, r.status, r.runoff, r.encerrado_em, p.nome
		FROM round r JOIN position p ON p.id = r.position_id
		WHERE p.congress_id = ? AND r.status = 'aberto'
		ORDER BY r.id DESC LIMIT 1`, congressID).
		Scan(&r.ID, &r.PositionID, &r.Numero, &r.Status, &runoff, &r.EncerradoEm, &p.Nome)
	if errors.Is(err, sql.ErrNoRows) {
		return r, p, false, nil
	}
	if err != nil {
		return r, p, false, err
	}
	r.Runoff = runoff == 1
	p.ID = r.PositionID
	return r, p, true, nil
}

// AbrirCargo coloca o cargo em eleição e abre o 1º escrutínio.
// `indicados` é opcional (Art. 91d); vazio => votável = todos os presentes.
func (s *Store) AbrirCargo(ctx context.Context, congressID, positionID int64, indicados []int64) (Round, error) {
	var declarada, encerrada int
	if err := s.db.QueryRowContext(ctx,
		`SELECT abertura_declarada, encerrada FROM congress WHERE id = ?`, congressID).
		Scan(&declarada, &encerrada); err != nil {
		return Round{}, err
	}
	if encerrada == 1 {
		return Round{}, errors.New("a eleição está encerrada")
	}
	if declarada == 0 {
		return Round{}, errors.New("declare a abertura (com quórum) antes de abrir um escrutínio")
	}
	var nome string
	var posAtivo int
	if err := s.db.QueryRowContext(ctx,
		`SELECT nome, ativo FROM position WHERE id = ?`, positionID).Scan(&nome, &posAtivo); err != nil {
		return Round{}, err
	}
	if posAtivo == 0 {
		return Round{}, errors.New("cargo desativado nas configurações")
	}
	if err := s.snapshotOp(ctx, "Abriu escrutínio: "+nome); err != nil {
		return Round{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Round{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE position SET status = 'em_eleicao' WHERE id = ? AND status != 'decidido'`,
		positionID); err != nil {
		return Round{}, err
	}
	rID, err := criarRound(ctx, tx, positionID, 1, false, indicados)
	if err != nil {
		return Round{}, err
	}
	if err := tx.Commit(); err != nil {
		return Round{}, err
	}
	return s.GetRound(ctx, rID)
}

// AbrirProximoEscrutinio cria a próxima rodada de um cargo ainda indeciso. Se for
// o 3º, é runoff: restringe aos 2 mais votados do escrutínio anterior.
func (s *Store) AbrirProximoEscrutinio(ctx context.Context, positionID int64) (Round, error) {
	prev, err := s.CurrentRound(ctx, positionID)
	if err != nil {
		return Round{}, err
	}
	if prev.Status != "encerrado" {
		return Round{}, errors.New("encerre o escrutínio atual primeiro")
	}
	next := prev.Numero + 1
	if next > MaxRounds {
		return Round{}, errors.New("já houve os três escrutínios")
	}
	runoff := next == MaxRounds

	var nome string
	s.db.QueryRowContext(ctx, `SELECT nome FROM position WHERE id = ?`, positionID).Scan(&nome)
	if err := s.snapshotOp(ctx, "Abriu próximo escrutínio: "+nome); err != nil {
		return Round{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Round{}, err
	}
	defer tx.Rollback()

	var indicados []int64
	if runoff {
		// Top-2 do escrutínio anterior entram no segundo turno.
		indicados, err = topNVotees(ctx, tx, prev.ID, 2)
		if err != nil {
			return Round{}, err
		}
	}
	rID, err := criarRound(ctx, tx, positionID, next, runoff, indicados)
	if err != nil {
		return Round{}, err
	}
	if err := tx.Commit(); err != nil {
		return Round{}, err
	}
	return s.GetRound(ctx, rID)
}

func criarRound(ctx context.Context, tx *sql.Tx, positionID int64, numero int, runoff bool, candidatos []int64) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO round(position_id, numero, runoff) VALUES (?, ?, ?)`,
		positionID, numero, boolToInt(runoff))
	if err != nil {
		return 0, err
	}
	rID, _ := res.LastInsertId()
	for _, e := range candidatos {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO round_candidate(round_id, elector_id) VALUES (?, ?)`, rID, e); err != nil {
			return 0, err
		}
	}
	return rID, nil
}

// topNVotees devolve os IDs dos N delegados mais votados de um escrutínio.
func topNVotees(ctx context.Context, tx *sql.Tx, roundID int64, n int) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT votee_elector_id FROM vote
		WHERE round_id = ? AND kind = 'candidato'
		GROUP BY votee_elector_id
		ORDER BY COUNT(*) DESC, votee_elector_id
		LIMIT ?`, roundID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CurrentPosition devolve o cargo em eleição; se nenhum, o próximo pendente (menor
// seq). ok=false quando todos estão decididos (eleição concluída).
func (s *Store) CurrentPosition(ctx context.Context, congressID int64) (Position, bool, error) {
	positions, err := s.Positions(ctx, congressID)
	if err != nil {
		return Position{}, false, err
	}
	for _, p := range positions {
		if p.Status == "em_eleicao" {
			return p, true, nil
		}
	}
	for _, p := range positions {
		if p.Status == "pendente" {
			return p, true, nil
		}
	}
	return Position{}, false, nil
}

// LatestRound devolve o escrutínio mais recente do congresso (para a aba Telão).
func (s *Store) LatestRound(ctx context.Context, congressID int64) (Round, bool, error) {
	var r Round
	var runoff int
	err := s.db.QueryRowContext(ctx, `
		SELECT r.id, r.position_id, r.numero, r.status, r.runoff, r.encerrado_em
		FROM round r JOIN position p ON p.id = r.position_id
		WHERE p.congress_id = ? ORDER BY r.id DESC LIMIT 1`, congressID).
		Scan(&r.ID, &r.PositionID, &r.Numero, &r.Status, &runoff, &r.EncerradoEm)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r.Runoff = runoff == 1
	return r, true, nil
}

// Rounds lista os escrutínios de um cargo, em ordem.
func (s *Store) Rounds(ctx context.Context, positionID int64) ([]Round, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, position_id, numero, status, runoff, encerrado_em FROM round
		 WHERE position_id = ? ORDER BY numero`, positionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Round
	for rows.Next() {
		var r Round
		var runoff int
		if err := rows.Scan(&r.ID, &r.PositionID, &r.Numero, &r.Status, &runoff, &r.EncerradoEm); err != nil {
			return nil, err
		}
		r.Runoff = runoff == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// VotableElectors devolve quem pode receber voto num escrutínio: o conjunto
// restrito (indicação/runoff) se houver, senão todos os presentes — sempre
// EXCLUINDO quem já foi eleito para outro cargo (não se acumula cargos) e quem
// excede a idade máxima do âmbito (Art. 4º §3–4; SPEC §3.1).
func (s *Store) VotableElectors(ctx context.Context, roundID int64) ([]Elector, error) {
	var restrito int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM round_candidate WHERE round_id = ?`, roundID).Scan(&restrito); err != nil {
		return nil, err
	}
	var ambito, sociedade string
	if err := s.db.QueryRowContext(ctx, `
		SELECT c.ambito, c.sociedade FROM round r
		JOIN position p ON p.id = r.position_id
		JOIN congress c ON c.id = p.congress_id WHERE r.id = ?`, roundID).
		Scan(&ambito, &sociedade); err != nil {
		return nil, err
	}
	naoEleito := ` AND e.id NOT IN (
		SELECT eleito_elector_id FROM position
		WHERE congress_id = e.congress_id AND eleito_elector_id IS NOT NULL)` +
		ageEligibleSQL(ambito, sociedade)
	if restrito > 0 {
		return s.queryElectors(ctx, `
			SELECT `+electorCols+`
			FROM round_candidate rc
			JOIN elector e ON e.id = rc.elector_id`+electorJoins+`
			WHERE rc.round_id = ?`+naoEleito+` ORDER BY e.nome`, roundID)
	}
	return s.queryElectors(ctx, `
		SELECT `+electorCols+`
		FROM elector e`+electorJoins+`
		WHERE e.presente = 1 AND e.congress_id = (
		  SELECT p.congress_id FROM round r JOIN position p ON p.id = r.position_id WHERE r.id = ?)`+
		naoEleito+` ORDER BY e.nome`, roundID)
}

// EncerrarEscrutinio fecha o escrutínio, apura e aplica a máquina de estados:
// se houver eleito (maioria; ou, no runoff, mais votado + desempate por idade),
// o cargo fica decidido. Devolve o resultado apurado.
func (s *Store) EncerrarEscrutinio(ctx context.Context, roundID int64) (Result, error) {
	var nome string
	s.db.QueryRowContext(ctx,
		`SELECT p.nome FROM round r JOIN position p ON p.id = r.position_id WHERE r.id = ?`,
		roundID).Scan(&nome)
	if err := s.snapshotOp(ctx, "Encerrou escrutínio: "+nome); err != nil {
		return Result{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE round SET status = 'encerrado', encerrado_em = datetime('now')
		 WHERE id = ? AND status = 'aberto'`, roundID); err != nil {
		return Result{}, err
	}
	res, err := s.Tally(ctx, roundID)
	if err != nil {
		return Result{}, err
	}
	if res.Eleito != nil {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE position SET status = 'decidido', eleito_elector_id = ? WHERE id = ?`,
			res.Eleito.ElectorID, res.Round.PositionID); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}
