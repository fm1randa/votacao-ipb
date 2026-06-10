# Tempo real com htmx + SSE (sinal + re-fetch)

Atualizações ao vivo (telão, eleitor, mesa) via **SSE** (Server-Sent Events) +
**htmx**, ambos **embutidos** (`//go:embed`, sem CDN — o evento é offline).

**Modelo:** um hub pub/sub em memória; toda mutação bem-sucedida (voto, credenciar,
abrir/encerrar escrutínio, reiniciar, etc.) chama `hub.Broadcast()`. O endpoint
`GET /events` (SSE) emite um **sinal leve** `tick`. Cada tela tem uma região com
`hx-trigger="sse:tick"` que **re-busca seu fragmento** (`hx-get`) e troca o DOM.

**Por quê SSE (não WebSocket):** o fluxo é só servidor→cliente (ações continuam
POST). SSE é HTTP puro, reconecta sozinho e degrada melhor numa WiFi instável.

**Por quê "sinal + re-fetch" (não empurrar HTML/JSON):** o fragmento é renderizado
**no servidor**, então a regra de sigilo (ADR-0001: telão só mostra progresso
enquanto aberto) fica num lugar só; perda de evento é inofensiva (o próximo `tick`
re-sincroniza); um canal único serve as três telas.

**Por quê NÃO livetemplate:** avaliado (pedido original), mas é **alpha** (v0.11.x,
"não recomendado para produção") e o cliente vem de **CDN** — risco alto no caminho
crítico de um evento offline de uma chance. htmx é maduro, um arquivo, sem build.

**Degradação:** sem JS / SSE caído, as telas funcionam no carregamento normal
(progressive enhancement do htmx) e o telão mantém um `meta refresh` de reserva.
`/events` é público (só sinaliza "mudou"; nenhum dado sensível trafega por ele).
