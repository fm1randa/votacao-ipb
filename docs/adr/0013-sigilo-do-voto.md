# Sigilo do voto: severar o elo voto↔token (vote_key com salt por escrutínio)

> **Status: PROPOSTA (spike do Plano 004) — aguarda decisão do mantenedor.**
> Protótipo no branch `advisor/004-ballot-secrecy-spike`; não fundido. As
> questões em aberto (Direção 1 vs 2, migração, desfazer/restaurar) estão em
> `plans/004-findings.md` e precisam ser resolvidas antes de ir a produção.

## Contexto — o segredo não era segredo

A promessa central do sistema é o voto secreto: "`vote` referencia o votado,
nunca o votante" (schema.sql). Essa promessa era **quebrável por quem tivesse o
arquivo `.db` inteiro** — exatamente o que o operador copia como backup. A
deanonimização precisava de **dois elos**, e ambos estavam no banco:

- **Elo B (voto↔token):** `vote` guardava o `token` cru e a queima era
  `UNIQUE(round_id, token)`.
- **Elo A (token↔pessoa):** o log de operações (ADR-0006) grava, antes de
  entregar o token, uma operação nomeada e datada ("Credenciou Fulano"), e o
  `token.entregue_em` cai no mesmo instante. Juntando `vote → token → operation`
  recompõe-se quem votou em quem.

O teste-instrumento `internal/store/secrecy_repro_test.go` prova o vazamento:
no esquema antigo, com o escrutínio aberto, religava 3 de 3 cédulas a votantes
nomeados. Hash do token com salt **fixa armazenada** não resolve: quem tem o
`.db` tem a tabela `token` e a salt, recomputa o hash e refaz o Elo B.

## Decisão — quebrar o Elo B com uma chave que se destrói

Basta romper **um** dos elos para fechar o vazamento; escolhemos o Elo B porque
o Elo A (a operação "Credenciou Fulano") é deliberadamente auditável e útil à
Mesa, e porque, sem o Elo B, saber quem tem qual token não diz nada sobre cédula.

**Direção 1 — segredo por escrutínio, apagado no encerramento:**

- Cada `round` ganha uma salt aleatória de 32 bytes (`round.vote_key_salt`,
  `crypto/rand`), criada ao abrir o escrutínio.
- `vote` guarda `vote_key = HMAC-SHA256(salt, token)` em vez do token; a queima
  vira `UNIQUE(round_id, vote_key)`. Como o HMAC é determinístico enquanto a
  salt existe, o mesmo token rende o mesmo `vote_key` e colide — a queima
  atômica do token sobrevive intacta.
- No **encerramento** do escrutínio, `vote_key_salt = NULL`. A partir daí os
  `vote_key` são HMACs sob uma chave perdida: o Elo B fica **irreversivelmente
  severado**, mesmo para quem detém o `.db` inteiro.

`CastVote`/`HasVoted` mantêm a assinatura (recebem `token`): derivam o `vote_key`
internamente. Nenhum handler web muda. `HasVoted` só responde com o escrutínio
**aberto** — depois do encerramento a salt sumiu e "este token votou?" é, por
desenho, irrespondível (é o ponto do sigilo).

## Não há outro caminho pessoa→cédula

Confirmado no código em `c74fd19` e reafirmado pelo teste pós-encerramento (0
religações por **qualquer** caminho que o instrumento tenta):

- `vote` **não tem coluna de votante**; `votee_elector_id` é o **votado**
  (identidade pública, por construção).
- `vote.criado_em` registra **quando**, não **quem**; a ordem de inserção não
  codifica identidade do votante (os votos chegam pela área do delegado sem
  vínculo com o rol).
- `Tally` conta por `votee_elector_id`/`kind` e **nunca** tocava o token — a
  apuração não muda (teste: Bruno=2, 1 branco, 3 depositados, intactos).

## Exposição residual conhecida (Direção 1)

Enquanto um escrutínio está **aberto**, a salt vive no banco. Um backup tirado
**no meio** de um escrutínio (uma cédula aberta, alguns minutos) somado à tabela
`token` ainda é correlacionável **para aquele escrutínio**. Escrutínios já
encerrados ficam seguros. O teste `TestSegredoDoVoto_VazamentoEnquantoAberto`
documenta essa janela de propósito (recompõe o HMAC enquanto a salt existe).

**Direção 2** (salt só em memória, nunca no `.db`) fecharia também a janela do
backup mid-round, ao custo de recuperação de falha: se o processo morre no meio,
a salt se perde e, ao reiniciar, `HasVoted` não reconhece votos já depositados →
risco de voto duplo pós-crash. Precisaria de mitigação (forçar encerramento de
escrutínio aberto ao reiniciar, ou salt num arquivo lateral apagado no fecho). O
trade-off — janela de backup mid-round (D1) vs voto duplo pós-crash (D2) — é a
decisão de fundo que o mantenedor precisa cravar (ver findings).

## Consequências

- **Migração:** o SQLite não dropa coluna nem troca `UNIQUE` por `ALTER`; a
  migração real exige **recriar a tabela `vote`**. O protótipo só acrescenta as
  colunas novas via `migrate()` (banco novo nasce certo pelo `schema.sql`).
- **Desfazer/Restaurar (ADR-0006):** snapshots antigos guardam `vote.token` e
  falham ao reinserir no esquema novo; e um snapshot tirado com escrutínio
  **aberto** carrega a salt — restaurá-lo **traz a salt de volta**. Ambos
  pedem decisão (versionar o snapshot, transformar na restauração, ou aceitar
  perda de restauração pré-migração). Detalhado em `plans/004-findings.md`.
- **Backups já vazados:** esta mudança **não redige** `.db` já existentes no
  mundo; isso é uma decisão à parte.
- **Regra permanente:** qualquer feature futura que precise consultar "este
  token votou?" **depois** do encerramento é incompatível com o sigilo e deve
  ser rejeitada.
