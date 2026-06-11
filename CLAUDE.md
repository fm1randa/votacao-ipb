# CLAUDE.md

Guia rápido para agentes. Sistema de votação **offline** por escrutínio secreto
para eleger a Diretoria de uma Sociedade Interna da IPB (UMP/UPA/UPH/SAF/UCP) em
qualquer âmbito (local, Federação, Sinodal, Nacional). Binário único em Go +
SQLite (WAL), roda numa LAN sem internet.

## Build, testes, execução

```bash
go build ./...        # compila
go vet ./...          # análise estática
go test ./...         # testes (hoje só internal/store)
go run . -seed        # cria eleição de exemplo + tokens, e sai
go run .              # sobe em :8080 (mostra IP da LAN e PIN no log)
```

Cross-compile (notebook da mesa / celular):
```bash
GOOS=windows GOARCH=amd64 go build -o votacao.exe .
GOOS=linux   GOARCH=arm64 go build -buildmode=pie -o votacao-android .   # Android/Termux
```
Driver SQLite é puro-Go (`modernc.org/sqlite`) — binário estático, sem cgo.

## Domínio (LEIA antes de mexer em regras)

A linguagem segue o GTSI da IPB e é toda em **português**. Não invente termos.

- `CONTEXT.md` — glossário (Âmbito, Sociedade, Delegado, Token, Escrutínio,
  Quórum, Indicação, Operação/Histórico…). Consulte antes de nomear qualquer coisa.
- `SPEC.md` — especificação + log de decisões (maioria, runoff, quórum por âmbito).
- `docs/adr/0001…0014` — decisões de arquitetura (ex.: ADR-0002 presença ≠ token;
  ADR-0006 log de operações restaurável; ADR-0009 motor único; ADR-0012 um .db
  por eleição).

## Layout

- `main.go` — flags (`-addr -host -data -db -pin -seed -tokens`), bootstrap.
- `internal/store/` — acesso ao SQLite + lógica de domínio. `schema.sql`
  (embutido), `store.go` (Open/WAL, tokens, CastVote), `electors.go`
  (rol/credenciamento/quórum), `positions.go` (cargos/escrutínios), `tally.go`
  (apuração), `operations.go` (Desfazer/Restaurar), `elections.go` (gerenciador
  de arquivos), `ambito.go` (presets por âmbito×sociedade), `atribuicoes.go`
  (texto do GTSI), `settings.go` (PIN).
- `internal/web/` — handlers HTTP, `html/template` embutido + htmx + SSE.
  Rotas em `Routes()` (`web.go`). A mesa fica atrás de PIN (`auth`/`authPIN`).
- `android/` — projeto Gradle **separado** (fora do go.mod): casca fina que
  hospeda o servidor no celular. Ver `android/README.md`.

## Convenções

- Tudo em português: UI, comentários, nomes de teste, mensagens de commit.
- Testes em pacote branco (`package store`), banco temporário via `t.TempDir()`
  e `Open(...)`. Exemplos: `internal/store/tally_test.go`,
  `internal/store/elections_test.go`.
- A eleição ativa é trocada a quente: handlers leem o `*store.Store` por
  `s.db()` (`atomic.Pointer`, `internal/web/web.go:40`).
- Segredo do voto é regra dura: `vote` referencia o **votado**, nunca o votante.
- Fluxo de trabalho: **nunca commite direto na `main`** — toda mudança via
  branch + PR. Planos e propostas são **issues no GitHub** (índice: issue #8),
  não arquivos locais; não crie diretório `plans/`. ADRs ficam em `docs/adr/`
  e entram via PR.

## Gotchas

- Nunca versione os bancos: `*.db`, `*.db-shm`, `*.db-wal` estão no `.gitignore`.
- O WAL persiste cada voto (recupera após queda). Backup = copiar os 3 arquivos.
- Android: `-buildmode=pie` é obrigatório; ver README para `termux-elf-cleaner`
  e detecção de IP do hotspot.
