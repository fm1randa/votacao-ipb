# Sigilo do voto: severar o elo voto↔token (vote_key com salt por escrutínio)

> **Status: ACEITA (Plano 004) — decisões cravadas pelo mantenedor; aguarda fusão.**
> Protótipo no branch `advisor/004-ballot-secrecy-spike`; ainda não fundido (a
> fusão é ato do mantenedor). Decisões: **Direção 1**; **sem migração**
> (greenfield — não há eleição real em arquivo); **Desfazer/Restaurar** sem
> mudança; **Elo A** intocado. Registro detalhado em `plans/004-findings.md`.

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

## Exposição residual conhecida (Direção 1) e por que é aceitável

Enquanto um escrutínio está **aberto**, a salt vive no banco. Um backup tirado
**no meio** de um escrutínio (uma cédula aberta, alguns minutos) somado à tabela
`token` ainda é correlacionável **para aquele escrutínio**. Escrutínios já
encerrados ficam seguros. O teste `TestSegredoDoVoto_VazamentoEnquantoAberto`
documenta essa janela de propósito (recompõe o HMAC enquanto a salt existe).

**Decisão: aceitamos esse resíduo.** O modelo de ameaça é um terceiro
mal-intencionado inspecionando o `.db` **depois** da operação — e contra isso a
Direção 1 protege por inteiro (a salt já foi anulada). A Mesa, durante a sessão,
é confiável (eleição de diretoria em ambiente eclesiástico; quem ocupa a Mesa
são pessoas idôneas) e já tem o processo vivo em mãos de qualquer modo. Backup
não é, hoje, um fluxo de uso real do sistema — logo a janela mid-round é teórica.

**Direção 2** (salt só em memória, nunca no `.db`) fecharia também essa janela,
ao custo de recuperação de falha: se o processo morre no meio, a salt se perde e,
ao reiniciar, `HasVoted` não reconhece votos já depositados → risco de voto duplo
pós-crash. O preço (fragilizar a eleição ao vivo justamente num crash) não
compensa o ganho marginal contra um cenário que não está no modelo de ameaça.
Fica registrada como alternativa caso o uso real passe a incluir backups
contínuos por terceiros não confiáveis.

## Consequências

- **Sem migração (greenfield):** não há eleição real em arquivo, então o
  `schema.sql` novo já nasce com `vote.vote_key` e `round.vote_key_salt`; não há
  caminho de migração nem `ALTER` no `migrate()`. Bancos `.db` de
  desenvolvimento são descartáveis (apagar e recriar).
- **Desfazer/Restaurar (ADR-0006): sem mudança.** Como não há snapshots de
  esquema antigo, sobra só o caso de restaurar um ponto com escrutínio **aberto**:
  o snapshot carrega a salt e restaurá-lo **a traz de volta** — que é exatamente
  o estado anterior legítimo (coerente com "desfazer o encerramento reabre a
  rodada com os votos intactos"). Mantido como está.
- **Regra permanente:** qualquer feature futura que precise consultar "este
  token votou?" **depois** do encerramento é incompatível com o sigilo e deve
  ser rejeitada.
- **Ao fundir:** atualizar `CLAUDE.md` e `SPEC.md`/`CONTEXT.md` — `vote` guarda
  um valor chaveado por escrutínio, não o token, e a chave é destruída no fecho.
