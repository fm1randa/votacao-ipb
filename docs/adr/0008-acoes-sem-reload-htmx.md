# Ações da mesa sem recarregar a página (htmx)

As ações da mesa deixam de ser POST→redirect (reload de página inteira) e passam a
`hx-post` (htmx). Padrão geral: o handler faz a mutação e responde **`204 No
Content` + header `HX-Trigger`** (dispara um toast no cliente); o **SSE** (ADR-0007)
re-renderiza as regiões vivas, inclusive a do próprio autor. Some o `?toast=` e o
redirect.

**Degradação:** o handler checa `HX-Request`. Com htmx → 204 + HX-Trigger. **Sem JS**
→ faz a mutação e redireciona (comportamento antigo preservado).

**Exceções (devolvem HTML, não 204):**
- **Credenciar / reemitir:** mostram o código ao operador num **modal**; a linha do
  delegado e os contadores atualizam no lugar via **htmx OOB** (`hx-swap-oob`),
  preservando a busca/filtro do credenciamento.
- **Presença (saída/reentrada):** troca **só aquela linha** (hx-target na linha).

**Toast:** o header `HX-Trigger: {"toast":{"msg":...,"undo":...}}` dispara um evento
no cliente; um listener mostra o toast (com "Desfazer" → `hx-post /board/undo` nas
ações destrutivas). Substitui o toast por query param, que só existia por falta de
reatividade.

**Histórico:** a lista de operações vira região viva (re-fetch no `sse:tick`), então
desfazer/restaurar/reiniciar atualizam o log sem reload; os modais fecham no sucesso.
