# Plano 004 — Achados do spike e decisões

Spike do sigilo do voto. Protótipo no branch `advisor/004-ballot-secrecy-spike`
(não fundido — a fusão é ato do mantenedor). Resultado da medição:

- **Baseline (escrutínio aberto):** o instrumento religa **3 de 3** cédulas a
  votantes nomeados (`TestSegredoDoVoto_VazamentoEnquantoAberto`).
- **Pós-encerramento:** **0** religações por qualquer caminho que o instrumento
  tenta (`TestSegredoDoVoto_NaoVazaAposEncerrar`).
- `go build ./... && go vet ./... && go test ./...` — tudo verde; a queima do
  token e a apuração (Bruno=2, 1 branco, 3 depositados) sobrevivem ao redesenho.

As questões em aberto foram **decididas pelo mantenedor** (registradas abaixo).

## 1. Direção 1 vs Direção 2 — DECIDIDO: Direção 1

| | Direção 1 (salt no `.db`, apagada no fecho) | Direção 2 (salt só em memória) |
|---|---|---|
| Escrutínio encerrado | seguro | seguro |
| Backup **mid-round** | correlacionável (salt viva) | seguro |
| Recuperação de falha | trivial (salt no banco) | **voto duplo pós-crash** (salt perdida) |
| Complexidade | baixa (implementada) | exige mitigação de crash |

**Decisão: Direção 1.** O modelo de ameaça é um terceiro mal-intencionado
inspecionando o `.db` **depois** da operação — D1 protege isso por inteiro. A
Mesa é confiável (eleição de diretoria eclesiástica; pessoas idôneas) e backup
não é fluxo de uso real hoje, então a janela mid-round é teórica. O custo da D2
(voto duplo pós-crash numa eleição ao vivo) não compensa o ganho marginal.
Reabrir a discussão só se o uso real passar a incluir backups contínuos
manipulados por terceiros não confiáveis.

## 2. Migração de eleições já em arquivo — DECIDIDO: sem migração (greenfield)

Não existe eleição real em arquivo (sistema em desenvolvimento). Logo **não há
legado para migrar**: o `schema.sql` novo já nasce com `vote.vote_key` e
`round.vote_key_salt`, e os `ALTER` que o protótipo havia adicionado em
`migrate()` foram **removidos**. Bancos `.db` de desenvolvimento são
descartáveis (apagar e recriar). A complexidade de recriar a tabela `vote` /
recusar upgrade com escrutínio aberto **deixa de existir**.

## 3. Desfazer/Restaurar (ADR-0006) — DECIDIDO: sem mudança

Sem legado (questão 2), não há snapshots de esquema antigo com `vote.token` para
transformar. Sobra apenas restaurar um ponto com escrutínio **aberto**: o
snapshot carrega a salt e restaurá-lo a traz de volta — que é o **estado
anterior legítimo** (coerente com "desfazer o encerramento reabre a rodada com
os votos intactos", ADR-0006). **Mantido como está**, sem versionar snapshot nem
regenerar salt.

## 4. Reduzir também o Elo A? — DECIDIDO: não

Severado o Elo B, saber quem tem qual token não revela cédula. Mantêm-se
`token.entregue_em` e a tabela `token` nos snapshots, úteis à reconciliação da
Mesa. É o que o ADR-0013 recomenda.

## 5. Backups já comprometidos — não se aplica

Não há `.db` de eleições reais no mundo. Sem ação necessária.

## 6. Documentação a atualizar quando isto for fundido

`CLAUDE.md`, `SPEC.md`/`CONTEXT.md`: registrar que `vote` guarda um valor
**chaveado por escrutínio** (não o token) e que a chave é **destruída no
encerramento**.
