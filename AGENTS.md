# AGENTS.md

Guia mínimo para agentes. Sistema de votação **offline** por escrutínio secreto
da IPB (Go + SQLite, roda numa LAN sem internet).

## Fluxo de trabalho

- **Nunca commite direto na `main`** — toda mudança via branch + PR.
- Planos e propostas são **issues no GitHub** (índice: issue #8), não arquivos
  locais; não crie diretório `plans/`.
- Decisões de arquitetura (ADRs) ficam em `docs/adr/` e entram via PR.
- Tudo em **português**: UI, comentários, nomes de teste, mensagens de commit.

## Onde ler

- `CLAUDE.md` — guia completo (build, testes, layout, convenções, gotchas).
- `CONTEXT.md` — glossário do domínio (termos do GTSI da IPB).
- `SPEC.md` — especificação + log de decisões.
