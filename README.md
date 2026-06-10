# Votação offline — Congresso de Federação UMP (IPB)

Sistema eletrônico de votação por escrutínio para eleger a **Diretoria de uma
Federação da UMP**, rodando **sem internet** numa LAN local (notebook da mesa +
roteador de viagem). Binário único em Go, banco SQLite. BYOD (celular) + quiosque
+ telão.

> Especificação completa em [SPEC.md](SPEC.md); glossário em [CONTEXT.md](CONTEXT.md);
> decisões em [docs/adr/](docs/adr/). Base normativa: GTSI 2015 (Art. 26, 49–52, 91).

## Decisões fechadas

| Tema | Decisão |
|------|---------|
| Quem é votado | **Qualquer delegado presente** (indicação opcional, Art. 91d) |
| Maioria | **Absoluta sobre os votos depositados** — `⌊depositados/2⌋ + 1` |
| Escrutínios | Até **3 por cargo**; o 3º é runoff entre os 2 mais votados (Art. 91e) |
| Empate no 3º | Desempate por **maior idade** (Art. 91g) |
| Cargos | 6, em sequência (Art. 26a): Presidente → Vice → Sec. Exec. → 1º Sec. → 2º Sec. → Tesoureiro |
| Sigilo | `vote` referencia o votado, nunca o votante; token cego |
| Presença | Da **pessoa**, não do token ([ADR-0002](docs/adr/0002-presenca-desacoplada-do-token.md)); reversível |
| Telão | Só progresso enquanto aberto ([ADR-0001](docs/adr/0001-sem-placar-parcial-em-escrutinio-secreto.md)) |
| Frontend | `html/template` + JS mínimo, num binário só |

## Como rodar

```bash
go run . -seed                 # cria congresso de exemplo + 200 tokens
go run .                       # sobe em :8080; mostra IP da LAN e PIN da mesa no log
go run . -addr :9000 -pin 4729 # porta e PIN custom
go test ./...                  # testes do motor de apuração
```

- **Eleitores (BYOD):** abrem `http://<ip-da-lan>:8080` no celular (mesma WiFi).
- **Quiosque:** mesma app com `?kiosk=1` (não guarda o código entre votos).
- **Mesa:** `/board`, protegida por **PIN** (mostrado no log ao subir).
- **Telão:** `/screen/{id}` — progresso ao vivo; resultado após encerrar.
- **Relatório/ata:** `/report` — imprimível, com Verificação de Poderes + resultados.

## Fluxo do dia

1. **Credenciar** cada delegado (marca presente + entrega token cego). Perda de
   código → **Reemitir token** (não infla quórum).
2. **Declarar quórum** (gate; painel mostra UMPs locais representadas + headcount).
3. **Abrir escrutínio** do cargo da vez → delegados votam (celular/quiosque).
4. **Encerrar** → telão mostra apuração e reconciliação. Sem eleito → próximo
   escrutínio (3º = runoff). Cargo decidido → próximo cargo.

## Modelo de dados

```
congress ─┬─ local (UMP local; base do quórum)
          ├─ elector (delegado: presente reversível, nato, nascimento?)
          ├─ token (pilha cega; NÃO mede presença)
          └─ position (cargo, seq, status) ── round (escrutínio 1..3, runoff)
                                                 ├─ round_candidate (indicação/top-2)
                                                 └─ vote (token, kind, votee)  ← sem votante
```

- **Sigilo:** `vote` nunca referencia o votante; só o token cego.
- **Queima atômica:** `UNIQUE(round_id, token)` em `vote`.
- **Presença ≠ token** (ADR-0002): quórum/reconciliação contam `elector.presente`.

## Estrutura

```
main.go                       flags, bootstrap, IP da LAN, PIN, seed
internal/store/schema.sql     schema (embutido via //go:embed)
internal/store/store.go       Open+WAL, tokens, CastVote (queima atômica)
internal/store/electors.go    congresso, locais, rol, credenciar, presença, quórum
internal/store/positions.go   cargos, escrutínios, máquina de estados, runoff
internal/store/tally.go       apuração: maioria, runoff, desempate por idade
internal/store/tally_test.go  testes do motor
internal/web/web.go           servidor, rotas, PIN, handlers do eleitor/telão
internal/web/web_board.go     handlers da mesa + relatório
internal/web/templates/       html/template embutidos
```

## Gotchas operacionais
- **Nobreak/baterias** no notebook e roteador.
- **Backup contínuo:** copie `votacao.db*` (os 3 arquivos) periodicamente. WAL já
  persiste cada voto e recupera após queda (testado com SIGKILL).
- **Roteador reserva** na mochila (~R$150).

## Cross-compile pro notebook da federação

```bash
GOOS=windows GOARCH=amd64 go build -o votacao.exe .
GOOS=linux   GOARCH=amd64 go build -o votacao .
```
Driver SQLite é puro-Go (`modernc.org/sqlite`) → binário estático, sem dependências.

## Rodando num celular Android (Termux)

Dá pra hospedar o sistema **no próprio celular**, servindo pelo hotspot — sem
notebook. Testado na prática; a receita tem pegadinhas:

1. **Build** (no computador):
   ```bash
   GOOS=linux GOARCH=arm64 go build -buildmode=pie -o votacao-android .
   ```
   O `-buildmode=pie` é obrigatório — sem ele o loader do Android rejeita o
   binário (`unexpected e_type: 2`).
2. **Instale o Termux pelo F-Droid** (a versão da Play Store é abandonada).
3. Copie o binário pro aparelho e **mova para o `$HOME` do Termux** — o storage
   compartilhado (`/sdcard`) é montado `noexec`:
   ```bash
   mv /sdcard/Download/votacao-android ~/ && chmod +x ~/votacao-android
   ```
4. **Conserte o alinhamento TLS** (o linker do Go alinha em 8 bytes; a Bionic
   ARM64 exige 64 — sem isso: `TLS segment is underaligned`):
   ```bash
   pkg install termux-elf-cleaner
   termux-elf-cleaner votacao-android
   ```
5. **Mantenha o processo vivo**: `termux-wake-lock` e desative a otimização de
   bateria do Termux (Configurações → Apps → Termux → Bateria → Sem restrições).
6. **Ligue o hotspot** do celular e suba o servidor:
   ```bash
   ./votacao-android -addr=:8090
   ```
   O IP do hotspot é detectado automaticamente (via `SIOCGIFCONF` — o Android
   13+ bloqueia netlink, então `ip addr` falha no Termux, mas o mecanismo do
   `ifconfig` funciona). **Se o log anunciar `localhost`**, descubra o IP e passe
   manualmente: conecte um celular cliente ao hotspot e veja o **gateway** nas
   informações da rede WiFi dele (a subnet é randomizada pelo Android — ex.
   `10.35.151.63`, não confie no clássico 192.168.43.1):
   ```bash
   ./votacao-android -addr=:8090 -host=10.35.151.63
   ```

O QR do telão, os logs e todos os links passam a anunciar o endereço certo.

## Itens não-software (com a federação)
Ver [SPEC.md §9](SPEC.md): aval da mesa ao voto eletrônico (há precedente — SEO/CSM
da IPB), parâmetros do edital (representantes/locais), e coletar data de nascimento
para automatizar o desempate por idade.
