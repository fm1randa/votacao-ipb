package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// Spike do Plano 004 — sigilo do voto.
//
// Estes testes são o INSTRUMENTO que mede o vazamento descrito no plano: dado
// alguém com o arquivo .db inteiro (o operador faz backups dele), é possível
// religar pessoa → cédula cruzando o `vote` com o `token` (Elo B) e o `token`
// com o log de operações (Elo A, "Credenciou Fulano")?
//
//   - TestSegredoDoVoto_VazamentoEnquantoAberto: a baseline. Com o escrutínio
//     AINDA ABERTO o vínculo existe — tanto no esquema original (vote.token cru)
//     quanto no protótipo da Direção 1 (a salt do round ainda está no banco, então
//     recomputa-se o HMAC). NÃO apague esta asserção: ela documenta por que o
//     esquema tem a forma que tem e qual é a exposição residual da Direção 1.
//   - TestSegredoDoVoto_NaoVazaAposEncerrar: a prova do fecho. Depois de
//     ENCERRAR o escrutínio (salt destruída) o vínculo cai a ZERO, e a queima do
//     token e a apuração continuam intactas.
//
// deanonLinkCount é o instrumento comum: abre o arquivo numa segunda conexão
// só-leitura e tenta TODO caminho plausível pessoa→cédula, tolerando colunas
// inexistentes (para servir aos dois esquemas). Devolve quantas cédulas
// distintas ficam ligadas a uma pessoa nomeada.

// deanonLinkCount conta as cédulas (vote.id) que um detentor do .db inteiro
// consegue religar a uma pessoa nomeada no log de operações.
func deanonLinkCount(t *testing.T, dbPath string) int {
	t.Helper()
	ro, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	ctx := context.Background()

	linked := map[int64]bool{}

	// --- Caminho A: token cru em vote (esquema original) -------------------
	// vote.token = token.token (Elo B), e token.entregue_em = operation.criado_em
	// numa op "Credenciou %" (Elo A). Tolera ausência da coluna vote.token.
	if rows, err := ro.QueryContext(ctx, `
		SELECT DISTINCT v.id
		FROM vote v
		JOIN token t     ON t.token = v.token
		JOIN operation o ON o.criado_em = t.entregue_em
		                AND o.descricao LIKE 'Credenciou %'`); err == nil {
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				linked[id] = true
			}
		}
		rows.Close()
	}

	// --- Caminho B: HMAC recomputável (Direção 1, escrutínio ABERTO) -------
	// Enquanto a salt do round vive no banco, recompõe-se vote_key = HMAC(salt,
	// token) para cada token entregue e casa-se com vote.vote_key; o token,
	// por sua vez, religa-se à pessoa pela op "Credenciou %". Tolera ausência
	// das colunas round.vote_key_salt / vote.vote_key.
	type roundSalt struct {
		id   int64
		salt []byte
	}
	var rounds []roundSalt
	if rows, err := ro.QueryContext(ctx,
		`SELECT id, vote_key_salt FROM round WHERE vote_key_salt IS NOT NULL`); err == nil {
		for rows.Next() {
			var rs roundSalt
			if err := rows.Scan(&rs.id, &rs.salt); err == nil && len(rs.salt) > 0 {
				rounds = append(rounds, rs)
			}
		}
		rows.Close()
	}
	for _, rs := range rounds {
		// vote_key -> vote.id deste round
		keyToVote := map[string]int64{}
		if rows, err := ro.QueryContext(ctx,
			`SELECT id, vote_key FROM vote WHERE round_id = ?`, rs.id); err == nil {
			for rows.Next() {
				var id int64
				var vk string
				if err := rows.Scan(&id, &vk); err == nil {
					keyToVote[vk] = id
				}
			}
			rows.Close()
		}
		if len(keyToVote) == 0 {
			continue
		}
		// Para cada token entregue E nomeável (Elo A), recomputa o HMAC.
		if rows, err := ro.QueryContext(ctx, `
			SELECT t.token FROM token t
			JOIN operation o ON o.criado_em = t.entregue_em
			                AND o.descricao LIKE 'Credenciou %'
			WHERE t.entregue = 1`); err == nil {
			for rows.Next() {
				var tok string
				if err := rows.Scan(&tok); err != nil {
					continue
				}
				mac := hmac.New(sha256.New, rs.salt)
				mac.Write([]byte(tok))
				vk := hex.EncodeToString(mac.Sum(nil))
				if id, ok := keyToVote[vk]; ok {
					linked[id] = true
				}
			}
			rows.Close()
		}
	}

	return len(linked)
}

// setupSecrecy monta o fluxo real (federação UMP, um cargo) com votantes
// NOMEADOS e devolve tokens/ids para votos conhecidos. Espelha o setup() de
// tally_test.go, mas com 5 presentes (>1 cargo satisfaz o gate de elegíveis) e
// nomes que o caminho de deanonimização precisa.
func setupSecrecy(t *testing.T) (*Store, string, int64, map[string]string, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	st, err := Open(dbPath)
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

	names := []string{"Ana", "Bruno", "Caio", "Dora", "Elias"}
	ids := map[string]int64{}
	toks := map[string]string{}
	for _, n := range names {
		id, err := st.AddElector(ctx, cong, n, &loc, nil, false, "1999-01-01")
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = id
		code, err := st.Credenciar(ctx, cong, id) // "Credenciou <n>" + entrega token
		if err != nil {
			t.Fatal(err)
		}
		toks[n] = code
	}
	if err := st.DeclararAbertura(ctx, cong); err != nil {
		t.Fatal(err)
	}
	return st, dbPath, cong, toks, ids
}

func TestSegredoDoVoto_VazamentoEnquantoAberto(t *testing.T) {
	ctx := context.Background()
	st, dbPath, cong, toks, ids := setupSecrecy(t)

	pos, _ := st.Positions(ctx, cong)
	r, err := st.AbrirCargo(ctx, cong, pos[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Votos conhecidos: Ana->Bruno, Caio->Bruno, Dora->branco.
	must(t, st.CastVote(ctx, r.ID, toks["Ana"], "candidato", ids["Bruno"]))
	must(t, st.CastVote(ctx, r.ID, toks["Caio"], "candidato", ids["Bruno"]))
	must(t, st.CastVote(ctx, r.ID, toks["Dora"], "branco", 0))

	linked := deanonLinkCount(t, dbPath)
	if linked == 0 {
		t.Fatal("esperava o vazamento presente (escrutínio aberto) — o join não ligou ninguém; " +
			"o Elo A por timestamp pode não ter casado (ver STOP do plano)")
	}
	t.Logf("VAZAMENTO (aberto): %d cédulas ligadas a uma pessoa pelo log de operações", linked)
}
