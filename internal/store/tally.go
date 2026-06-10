package store

import (
	"context"
	"database/sql"
)

// ResultLine é a contagem de um delegado votado num escrutínio.
type ResultLine struct {
	ElectorID  int64
	Nome       string
	LocalNome  string
	Nascimento sql.NullString
	Votos      int
	Eleito     bool
}

// Result é a apuração completa de um escrutínio, pronta pro telão e o relatório.
type Result struct {
	Round              Round
	PositionNome       string
	Presentes          int // pessoas presentes (denominador do quórum, não do voto)
	Depositados        int // candidato + branco + nulo
	Brancos            int
	Nulos              int
	Abstencoes         int // Presentes - Depositados
	Maioria            int // ⌊Depositados/2⌋ + 1
	Lines              []ResultLine
	Eleito             *ResultLine // nil se ninguém eleito ainda
	EmpateNaoResolvido bool        // runoff empatado sem data de nascimento p/ desempate
}

// Tally apura um escrutínio e aplica a regra de maioria (ver SPEC.md §4):
//   - maioria absoluta sobre TODOS os votos depositados (brancos/nulos incluídos);
//   - escrutínios 1 e 2: eleito quem tiver ≥ maioria;
//   - escrutínio 3 (runoff): vence o mais votado; empate exato → maior idade (Art. 91g).
func (s *Store) Tally(ctx context.Context, roundID int64) (Result, error) {
	round, err := s.GetRound(ctx, roundID)
	if err != nil {
		return Result{}, err
	}
	pos, err := s.GetPosition(ctx, round.PositionID)
	if err != nil {
		return Result{}, err
	}
	r := Result{Round: round, PositionNome: pos.Nome}

	var congressID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT congress_id FROM position WHERE id = ?`, round.PositionID).Scan(&congressID); err != nil {
		return Result{}, err
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM elector WHERE congress_id = ? AND presente = 1`, congressID).
		Scan(&r.Presentes); err != nil {
		return Result{}, err
	}

	// Brancos, nulos e total depositado.
	if err := s.db.QueryRowContext(ctx,
		`SELECT
		   COUNT(*),
		   COALESCE(SUM(kind='branco'),0),
		   COALESCE(SUM(kind='nulo'),0)
		 FROM vote WHERE round_id = ?`, roundID).
		Scan(&r.Depositados, &r.Brancos, &r.Nulos); err != nil {
		return Result{}, err
	}
	r.Abstencoes = r.Presentes - r.Depositados
	if r.Depositados > 0 {
		r.Maioria = r.Depositados/2 + 1
	}

	// Linhas por candidato.
	var restrito int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM round_candidate WHERE round_id = ?`, roundID).Scan(&restrito); err != nil {
		return Result{}, err
	}
	var query string
	if restrito > 0 {
		// Conjunto restrito (indicação/runoff): mostra todos, mesmo com 0 votos.
		query = `
		 SELECT e.id, e.nome, COALESCE(l.nome,''), e.nascimento, COUNT(v.id)
		 FROM round_candidate rc
		 JOIN elector e ON e.id = rc.elector_id
		 LEFT JOIN local l ON l.id = e.local_id
		 LEFT JOIN vote v ON v.votee_elector_id = e.id AND v.round_id = rc.round_id
		 WHERE rc.round_id = ?
		 GROUP BY e.id ORDER BY COUNT(v.id) DESC, e.nome`
	} else {
		// Aberto: só quem recebeu votos.
		query = `
		 SELECT e.id, e.nome, COALESCE(l.nome,''), e.nascimento, COUNT(v.id)
		 FROM vote v
		 JOIN elector e ON e.id = v.votee_elector_id
		 LEFT JOIN local l ON l.id = e.local_id
		 WHERE v.round_id = ? AND v.kind = 'candidato'
		 GROUP BY e.id ORDER BY COUNT(v.id) DESC, e.nome`
	}
	rows, err := s.db.QueryContext(ctx, query, roundID)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var l ResultLine
		if err := rows.Scan(&l.ElectorID, &l.Nome, &l.LocalNome, &l.Nascimento, &l.Votos); err != nil {
			return Result{}, err
		}
		r.Lines = append(r.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}

	decidirEleito(&r)
	return r, nil
}

// decidirEleito aplica a regra de vitória sobre as linhas já ordenadas por votos desc.
func decidirEleito(r *Result) {
	if len(r.Lines) == 0 || r.Depositados == 0 {
		return
	}
	if !r.Round.Runoff {
		// Escrutínios 1 e 2: precisa de maioria absoluta.
		for i := range r.Lines {
			if r.Lines[i].Votos >= r.Maioria {
				r.Lines[i].Eleito = true
				r.Eleito = &r.Lines[i]
				return
			}
		}
		return
	}

	// Runoff: vence o mais votado; empate exato → maior idade (Art. 91g).
	maxVotos := r.Lines[0].Votos
	if maxVotos == 0 {
		return
	}
	var empatados []int // índices das linhas com o máximo de votos
	for i := range r.Lines {
		if r.Lines[i].Votos == maxVotos {
			empatados = append(empatados, i)
		}
	}
	if len(empatados) == 1 {
		r.Lines[empatados[0]].Eleito = true
		r.Eleito = &r.Lines[empatados[0]]
		return
	}
	// Desempate por maior idade: menor data de nascimento (ISO) vence.
	melhor := -1
	for _, i := range empatados {
		if !r.Lines[i].Nascimento.Valid {
			r.EmpateNaoResolvido = true // falta nascimento → a Mesa resolve
			return
		}
		if melhor == -1 || r.Lines[i].Nascimento.String < r.Lines[melhor].Nascimento.String {
			melhor = i
		}
	}
	r.Lines[melhor].Eleito = true
	r.Eleito = &r.Lines[melhor]
}
