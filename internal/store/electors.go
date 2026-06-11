package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Tipos
// ---------------------------------------------------------------------------

type Congress struct {
	ID                int64
	Ambito            string // local | federacao | sinodal | nacional
	Sociedade         string // UMP | UPA | UPH | SAF | UCP
	Nome              string // nome da entidade (ex.: "Federação de UMP do PRNT")
	Ano               int
	AberturaDeclarada bool
	Encerrada         bool
}

type Local struct {
	ID   int64
	Nome string
}

type Elector struct {
	ID           int64
	Nome         string
	LocalID      sql.NullInt64
	LocalNome    string
	SubLocalID   sql.NullInt64
	SubLocalNome string // Federação do delegado (só âmbito nacional)
	Nato         bool
	Nascimento   sql.NullString
	Credenciado  bool
	Presente     bool
}

// QuorumInfo alimenta o painel da Mesa. A regra varia por âmbito (Art. 12 §2º, 49):
//   - local:      mais da metade dos sócios ativos do ROL (pessoas);
//   - federacao:  mais da metade das UMPs locais representadas;
//   - sinodal:    mais da metade das Federações representadas;
//   - nacional:   mais da metade das Sinodais + o critério de Federações da
//     sociedade (NacionalSubRegra — UMP ⅓, UPH/SAF metade, UPA nenhum).
//
// `Ok` é o gate computado da Declaração de Abertura (ADR-0010).
type QuorumInfo struct {
	Ambito          string
	Presentes       int    // pessoas presentes (headcount)
	Credenciados    int    // já receberam token alguma vez
	RolTotal        int    // total do rol (denominador no âmbito local)
	UnidadesTotal   int    // unidades primárias cadastradas (nivel 0)
	UnidadesRepr    int    // unidades com ≥1 delegado presente
	SubTotal        int    // só nacional: federações cadastradas (nivel 1)
	SubRepr         int    // só nacional: federações com ≥1 delegado presente
	SubRegra        string // só nacional: SubRegraTerco | SubRegraMetade | SubRegraNada
	Ok              bool
	Elegiveis       int  // presentes aptos a SER votados (dentro do limite de idade)
	CargosAtivos    int  // cargos ativos a eleger
	ElegiveisOk     bool // Elegiveis ≥ CargosAtivos (cada cargo exige uma pessoa distinta)
	TokensEntregues int
	Reemissoes      int // tokens entregues além dos credenciados (perdas)
}

// ---------------------------------------------------------------------------
// Congresso/Plenária, unidades, rol
// ---------------------------------------------------------------------------

func (s *Store) CreateCongress(ctx context.Context, ambito, sociedade, nome string, ano int) (int64, error) {
	if err := ValidateAmbitoSociedade(ambito, sociedade); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO congress(ambito, sociedade, nome, ano) VALUES (?, ?, ?, ?)`,
		ambito, sociedade, nome, ano)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FirstCongress devolve o congresso (assume-se um por banco).
func (s *Store) FirstCongress(ctx context.Context) (Congress, error) {
	var c Congress
	err := s.db.QueryRowContext(ctx,
		`SELECT id, ambito, sociedade, nome, ano, abertura_declarada, encerrada
		 FROM congress ORDER BY id LIMIT 1`).
		Scan(&c.ID, &c.Ambito, &c.Sociedade, &c.Nome, &c.Ano, &c.AberturaDeclarada, &c.Encerrada)
	return c, err
}

// DeclararAbertura é o gate computado (ADR-0010): só declara com quórum
// atingido E presentes elegíveis suficientes para todos os cargos. Sem
// override — rol incorreto corrige-se editando o rol.
func (s *Store) DeclararAbertura(ctx context.Context, congressID int64) error {
	q, err := s.Quorum(ctx, congressID)
	if err != nil {
		return err
	}
	if !q.Ok {
		return errors.New("quórum não atingido — confira a presença e o rol")
	}
	if !q.ElegiveisOk {
		return fmt.Errorf("presentes elegíveis insuficientes: %d para %d cargos a eleger",
			q.Elegiveis, q.CargosAtivos)
	}
	if err := s.snapshotOp(ctx, "Declarou a abertura (quórum verificado)"); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE congress SET abertura_declarada = 1 WHERE id = ?`, congressID)
	return err
}

// SetupCongress cria o evento com o preset de cargos do âmbito×sociedade e a
// pilha inicial de tokens — o passo 2 do wizard. `disabledRoles` desativa
// cargos opcionais do preset (SPEC §3.5, §10).
func (s *Store) SetupCongress(ctx context.Context, ambito, sociedade, nome string, ano int, disabledRoles []string) (int64, error) {
	if err := s.snapshotOp(ctx, "Configurou a eleição"); err != nil {
		return 0, err
	}
	id, err := s.CreateCongress(ctx, ambito, sociedade, nome, ano)
	if err != nil {
		return 0, err
	}
	if err := s.applyPositionPreset(ctx, id, ambito, sociedade, disabledRoles); err != nil {
		return 0, err
	}
	return id, s.GenerateTokens(ctx, id, 100)
}

// applyPositionPreset cria os cargos do preset (apagando os existentes).
func (s *Store) applyPositionPreset(ctx context.Context, congressID int64, ambito, sociedade string, disabledRoles []string) error {
	off := map[string]bool{}
	for _, r := range disabledRoles {
		if OptionalRole(ambito, sociedade, r) {
			off[r] = true
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM position WHERE congress_id = ?`, congressID); err != nil {
		return err
	}
	for i, p := range PresetPositions(ambito, sociedade) {
		if err := s.AddPosition(ctx, congressID, p.Nome, p.Role, i+1); err != nil {
			return err
		}
		if off[p.Role] {
			if _, err := s.db.ExecContext(ctx,
				`UPDATE position SET ativo = 0 WHERE congress_id = ? AND role = ?`, congressID, p.Role); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateCongress altera os dados do evento (tela Ajustes). Trocar âmbito ou
// sociedade re-aplica o preset de cargos e só é permitido antes da Declaração
// de Abertura e sem escrutínios (SPEC §10.1).
func (s *Store) UpdateCongress(ctx context.Context, id int64, ambito, sociedade, nome string, ano int) error {
	if err := ValidateAmbitoSociedade(ambito, sociedade); err != nil {
		return err
	}
	cur, err := s.FirstCongress(ctx)
	if err != nil {
		return err
	}
	mudouPreset := cur.Ambito != ambito || cur.Sociedade != sociedade
	if mudouPreset {
		if cur.AberturaDeclarada {
			return errors.New("a abertura já foi declarada — âmbito e sociedade não podem mais mudar")
		}
		var rounds int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM round r JOIN position p ON p.id = r.position_id
			WHERE p.congress_id = ?`, id).Scan(&rounds); err != nil {
			return err
		}
		if rounds > 0 {
			return errors.New("já houve escrutínios — desfaça pelo Histórico antes de trocar o âmbito")
		}
	}
	desc := "Alterou dados da eleição"
	if mudouPreset {
		desc = "Alterou âmbito/sociedade (cargos redefinidos)"
	}
	if err := s.snapshotOp(ctx, desc); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE congress SET ambito = ?, sociedade = ?, nome = ?, ano = ? WHERE id = ?`,
		ambito, sociedade, nome, ano, id); err != nil {
		return err
	}
	if mudouPreset {
		return s.applyPositionPreset(ctx, id, ambito, sociedade, nil)
	}
	return nil
}

// Locals lista as unidades de representação primárias (datalist dos formulários).
func (s *Store) Locals(ctx context.Context, congressID int64) ([]Local, error) {
	return s.queryLocals(ctx, congressID, 0)
}

// SubLocals lista as subunidades (Federações; só no âmbito nacional).
func (s *Store) SubLocals(ctx context.Context, congressID int64) ([]Local, error) {
	return s.queryLocals(ctx, congressID, 1)
}

func (s *Store) queryLocals(ctx context.Context, congressID int64, nivel int) ([]Local, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, nome FROM local WHERE congress_id = ? AND nivel = ? ORDER BY nome`,
		congressID, nivel)
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

// LocalByNameOrCreate acha a unidade pelo nome (sem case) ou cria uma nova.
func (s *Store) LocalByNameOrCreate(ctx context.Context, congressID int64, nome string, nivel int) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM local WHERE congress_id = ? AND nivel = ? AND lower(nome) = lower(?)`,
		congressID, nivel, nome).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.AddLocal(ctx, congressID, nome, nivel)
	}
	return id, err
}

func (s *Store) AddLocal(ctx context.Context, congressID int64, nome string, nivel int) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO local(congress_id, nome, nivel) VALUES (?, ?, ?)`, congressID, nome, nivel)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddElector cadastra um votante. localID nulo + nato=true para membros natos;
// subLocalID = Federação do delegado (só âmbito nacional).
func (s *Store) AddElector(ctx context.Context, congressID int64, nome string, localID, subLocalID *int64, nato bool, nascimento string) (int64, error) {
	var loc, sub, nasc interface{}
	if localID != nil {
		loc = *localID
	}
	if subLocalID != nil {
		sub = *subLocalID
	}
	if nascimento != "" {
		nasc = nascimento
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO elector(congress_id, nome, local_id, sub_local_id, nato, nascimento) VALUES (?,?,?,?,?,?)`,
		congressID, nome, loc, sub, boolToInt(nato), nasc)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ElectorInput é uma linha de cadastro (form individual ou colar lista).
type ElectorInput struct {
	Nome, LocalNome, SubLocalNome, Nascimento string
	Nato                                      bool
}

// resolveUnits resolve (criando se preciso) unidade e subunidade de um input,
// já aplicando as regras do âmbito: local não tem unidades nem natos;
// subunidade só existe no nacional.
func (s *Store) resolveUnits(ctx context.Context, cong Congress, in *ElectorInput) (localID, subID *int64, err error) {
	if cong.Ambito == AmbitoLocal {
		in.Nato = false
		return nil, nil, nil
	}
	if !in.Nato && strings.TrimSpace(in.LocalNome) != "" {
		id, err := s.LocalByNameOrCreate(ctx, cong.ID, strings.TrimSpace(in.LocalNome), 0)
		if err != nil {
			return nil, nil, err
		}
		localID = &id
	}
	if cong.Ambito == AmbitoNacional && !in.Nato && strings.TrimSpace(in.SubLocalNome) != "" {
		id, err := s.LocalByNameOrCreate(ctx, cong.ID, strings.TrimSpace(in.SubLocalNome), 1)
		if err != nil {
			return nil, nil, err
		}
		subID = &id
	}
	return localID, subID, nil
}

// ImportElectors cadastra votantes numa só operação do log (opDesc descreve:
// "Adicionou X" ou "Importou N"). Unidades inexistentes são criadas; garante a
// pilha de tokens ao final.
func (s *Store) ImportElectors(ctx context.Context, congressID int64, items []ElectorInput, opDesc string) error {
	if len(items) == 0 {
		return errors.New("ninguém para cadastrar")
	}
	cong, err := s.FirstCongress(ctx)
	if err != nil {
		return err
	}
	if err := s.snapshotOp(ctx, opDesc); err != nil {
		return err
	}
	for i := range items {
		it := items[i]
		localID, subID, err := s.resolveUnits(ctx, cong, &it)
		if err != nil {
			return err
		}
		if _, err := s.AddElector(ctx, congressID, strings.TrimSpace(it.Nome), localID, subID, it.Nato, it.Nascimento); err != nil {
			return err
		}
	}
	return s.EnsureTokens(ctx, congressID)
}

// UpdateElector edita nome/unidade/nato/nascimento de um votante.
func (s *Store) UpdateElector(ctx context.Context, congressID, id int64, in ElectorInput) error {
	cong, err := s.FirstCongress(ctx)
	if err != nil {
		return err
	}
	if err := s.snapshotOp(ctx, "Editou "+strings.TrimSpace(in.Nome)); err != nil {
		return err
	}
	localID, subID, err := s.resolveUnits(ctx, cong, &in)
	if err != nil {
		return err
	}
	var loc, sub, nasc interface{}
	if localID != nil {
		loc = *localID
	}
	if subID != nil {
		sub = *subID
	}
	if in.Nascimento != "" {
		nasc = in.Nascimento
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE elector SET nome = ?, local_id = ?, sub_local_id = ?, nato = ?, nascimento = ? WHERE id = ?`,
		strings.TrimSpace(in.Nome), loc, sub, boolToInt(in.Nato), nasc, id)
	return err
}

// DeleteElector remove um votante que NUNCA foi credenciado (depois disso,
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
		return errors.New("já credenciado — registre a saída em vez de remover")
	}
	if err := s.snapshotOp(ctx, "Removeu "+nome); err != nil {
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

// electorCols são as colunas padrão de leitura do rol (alias e/l/sl).
const electorCols = `e.id, e.nome, e.local_id, COALESCE(l.nome,''), e.sub_local_id, COALESCE(sl.nome,''),
	 e.nato, e.nascimento, e.credenciado, e.presente`

const electorJoins = ` LEFT JOIN local l ON l.id = e.local_id LEFT JOIN local sl ON sl.id = e.sub_local_id`

// Electors lista o rol com os nomes das unidades.
func (s *Store) Electors(ctx context.Context, congressID int64) ([]Elector, error) {
	return s.queryElectors(ctx,
		`SELECT `+electorCols+` FROM elector e`+electorJoins+`
		 WHERE e.congress_id = ? ORDER BY e.nome`, congressID)
}

// GetElector devolve um votante pelo id (com os nomes das unidades).
func (s *Store) GetElector(ctx context.Context, id int64) (Elector, error) {
	els, err := s.queryElectors(ctx,
		`SELECT `+electorCols+` FROM elector e`+electorJoins+` WHERE e.id = ?`, id)
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
		`SELECT `+electorCols+` FROM elector e`+electorJoins+`
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
		if err := rows.Scan(&e.ID, &e.Nome, &e.LocalID, &e.LocalNome, &e.SubLocalID, &e.SubLocalNome,
			&nato, &e.Nascimento, &cred, &pres); err != nil {
			return nil, err
		}
		e.Nato, e.Credenciado, e.Presente = nato == 1, cred == 1, pres == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Credenciamento/Chamada e presença (ADR-0002: presença é da pessoa, não do token)
// ---------------------------------------------------------------------------

// Credenciar marca o votante como presente/credenciado E entrega um token cego,
// numa transação. Devolve o código do token. No âmbito local o ato é a Chamada
// (Art. 86) — muda só a descrição no Histórico.
func (s *Store) Credenciar(ctx context.Context, congressID, electorID int64) (string, error) {
	var nome string
	s.db.QueryRowContext(ctx, `SELECT nome FROM elector WHERE id = ?`, electorID).Scan(&nome)
	desc := "Credenciou " + nome
	if cong, err := s.FirstCongress(ctx); err == nil && cong.Ambito == AmbitoLocal {
		desc = "Registrou na chamada: " + nome
	}
	if err := s.snapshotOp(ctx, desc); err != nil {
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
		return "", errors.New("não encontrado no rol")
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

// SetPresente registra saída (false) ou reentrada (true) de alguém já credenciado.
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

// Quorum computa a regra do âmbito (ver QuorumInfo). Natos não contam como
// unidade representada nem entram no denominador (federados).
func (s *Store) Quorum(ctx context.Context, congressID int64) (QuorumInfo, error) {
	cong, err := s.FirstCongress(ctx)
	if err != nil {
		return QuorumInfo{}, err
	}
	q := QuorumInfo{Ambito: cong.Ambito}
	row := s.db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM elector WHERE congress_id=? AND presente=1),
		  (SELECT COUNT(*) FROM elector WHERE congress_id=? AND credenciado=1),
		  (SELECT COUNT(*) FROM elector WHERE congress_id=?),
		  (SELECT COUNT(*) FROM local   WHERE congress_id=? AND nivel=0),
		  (SELECT COUNT(DISTINCT local_id) FROM elector WHERE congress_id=? AND presente=1 AND local_id IS NOT NULL),
		  (SELECT COUNT(*) FROM local   WHERE congress_id=? AND nivel=1),
		  (SELECT COUNT(DISTINCT sub_local_id) FROM elector WHERE congress_id=? AND presente=1 AND sub_local_id IS NOT NULL),
		  (SELECT COUNT(*) FROM token WHERE congress_id=? AND entregue=1)`,
		congressID, congressID, congressID, congressID, congressID, congressID, congressID, congressID)
	if err := row.Scan(&q.Presentes, &q.Credenciados, &q.RolTotal,
		&q.UnidadesTotal, &q.UnidadesRepr, &q.SubTotal, &q.SubRepr, &q.TokensEntregues); err != nil {
		return q, err
	}
	switch cong.Ambito {
	case AmbitoLocal:
		// Art. 12 §2º: mais da metade dos sócios ativos do rol.
		q.Ok = q.RolTotal > 0 && q.Presentes*2 > q.RolTotal
	case AmbitoNacional:
		// Mais da metade das Sinodais + critério de Federações da sociedade
		// (UMP Art. 49c: ⅓; UPH Art. 134 / SAF Art. 91: metade; UPA Art. 62: nenhum).
		q.SubRegra = NacionalSubRegra(cong.Sociedade)
		q.Ok = q.UnidadesTotal > 0 && q.UnidadesRepr*2 > q.UnidadesTotal
		switch q.SubRegra {
		case SubRegraTerco:
			q.Ok = q.Ok && q.SubTotal > 0 && q.SubRepr*3 >= q.SubTotal
		case SubRegraMetade:
			q.Ok = q.Ok && q.SubTotal > 0 && q.SubRepr*2 > q.SubTotal
		}
	default:
		// Art. 49a–b: mais da metade das unidades (UMPs locais / Federações).
		q.Ok = q.UnidadesTotal > 0 && q.UnidadesRepr*2 > q.UnidadesTotal
	}
	q.Reemissoes = q.TokensEntregues - q.Credenciados
	if q.Reemissoes < 0 {
		q.Reemissoes = 0
	}
	// Elegíveis × cargos: como eleito não acumula cargo, a eleição só é possível
	// com ao menos tantos presentes votáveis (limite de idade incluso) quantos
	// cargos ativos. É o segundo gate da Declaração de Abertura.
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM elector e WHERE e.congress_id=? AND e.presente=1`+
			ageEligibleSQL(cong.Ambito, cong.Sociedade), congressID).Scan(&q.Elegiveis); err != nil {
		return q, err
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM position WHERE congress_id=? AND ativo=1`,
		congressID).Scan(&q.CargosAtivos); err != nil {
		return q, err
	}
	q.ElegiveisOk = q.Elegiveis >= q.CargosAtivos
	return q, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
