# Plano 004 — Achados do spike e questões em aberto

Spike do sigilo do voto. Protótipo no branch `advisor/004-ballot-secrecy-spike`
(não fundido). Resultado da medição:

- **Baseline (escrutínio aberto):** o instrumento religa **3 de 3** cédulas a
  votantes nomeados (`TestSegredoDoVoto_VazamentoEnquantoAberto`).
- **Pós-encerramento:** **0** religações por qualquer caminho que o instrumento
  tenta (`TestSegredoDoVoto_NaoVazaAposEncerrar`).
- `go build ./... && go vet ./... && go test ./...` — tudo verde; a queima do
  token e a apuração (Bruno=2, 1 branco, 3 depositados) sobrevivem ao redesenho.

A correção (ADR-0013 proposta) sela o vazamento, mas **não deve ser fundida** sem
o mantenedor decidir os pontos abaixo.

## 1. Direção 1 vs Direção 2 (a decisão de fundo)

| | Direção 1 (salt no `.db`, apagada no fecho) | Direção 2 (salt só em memória) |
|---|---|---|
| Escrutínio encerrado | seguro | seguro |
| Backup **mid-round** | **correlacionável** (salt viva) | seguro |
| Recuperação de falha | trivial (salt no banco) | **voto duplo pós-crash** (salt perdida) |
| Complexidade | baixa (implementada) | exige mitigação de crash |

O protótipo implementa a **Direção 1**. A pergunta: a exposição de um backup
tirado no meio de um escrutínio é aceitável (D1), ou vale o custo de mitigar a
recuperação de falha (D2)? Mitigações possíveis para D2: ao reiniciar com
escrutínio aberto, **forçar o encerramento** dele; ou persistir a salt num
**arquivo lateral** apagado no fecho (e no boot, se houver salt órfã, encerrar
o escrutínio). **Decisão pendente.**

## 2. Migração de eleições já em arquivo

O SQLite não dropa coluna nem troca `UNIQUE` via `ALTER`. O protótipo só
**acrescenta** `round.vote_key_salt` e `vote.vote_key` (nullable) em bancos
antigos; o esquema novo só nasce inteiro num `.db` **novo**. Para um arquivo já
existente, a migração real precisa **recriar a tabela `vote`** (criar nova,
copiar dados, dropar a antiga, renomear). Questões:

- O que fazer com `vote.token` dos votos **já gravados**? Sem a salt do
  passado não há como derivar um `vote_key`. Recriar a tabela num banco com
  votos antigos implica ou descartá-los ou inventar uma salt retroativa (o que
  reabriria o Elo B). **Provável regra:** só migrar bancos **sem escrutínio em
  andamento** (entre eleições); recusar a migração com escrutínio aberto.
- E um escrutínio que estava **aberto** na hora do upgrade? Decidir: encerrar
  forçado antes de migrar, ou bloquear o upgrade.

## 3. Interação com Desfazer/Restaurar (ADR-0006)

`restoreDomain` (operations.go) reinsere as colunas que estão no snapshot JSON.
Dois problemas:

- **Snapshots pré-migração** contêm `vote.token` → o INSERT falha no esquema
  novo (coluna inexistente). Opções: (a) **versionar** o snapshot e transformar
  na restauração; (b) aceitar que o histórico **pré-migração** de uma eleição
  não é restaurável (documentar e avisar na UI); (c) migração transforma também
  os snapshots gravados. Recomendação do spike: (b) para o protótipo, (a) se for
  a produção.
- **Snapshots com escrutínio aberto carregam a salt.** Restaurar um ponto em que
  o escrutínio estava aberto **traz `vote_key_salt` de volta** — reabrindo a
  janela do Elo B para aquele escrutínio (coerente com "desfazer o encerramento
  reabre a rodada", ADR-0006, mas agora também ressuscita a salt). Decidir se é
  aceitável (é o mesmo estado de antes) ou se a restauração deve **regenerar**
  uma salt nova ao reabrir.

## 4. Reduzir também o Elo A? (defesa em profundidade)

Severado o Elo B, saber quem tem qual token não revela cédula — o Elo B sozinho
basta para o sigilo, e é o que o ADR-0013 recomenda. Como **defesa em
profundidade** opcional, poder-se-ia parar de gravar `token.entregue_em` ou
excluir a tabela `token` dos snapshots. **Não** é necessário para fechar o
vazamento; fica como decisão separada (custo: perde rastreabilidade da entrega
de tokens, útil à reconciliação da Mesa).

## 5. Backups já comprometidos

Nada nesta correção redige `.db` que já circulam. Arquivos de eleições passadas
gravados no esquema antigo continuam deanonimizáveis. Se isso importa, é uma
ação à parte (orientar operadores a destruir backups antigos, p.ex.).

## 6. Documentação a atualizar quando isto for fundido

`CLAUDE.md`, `SPEC.md`/`CONTEXT.md`: registrar que `vote` guarda um valor
**chaveado por escrutínio** (não o token) e que a chave é **destruída no
encerramento**.
