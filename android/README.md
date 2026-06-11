# App Android — spike de hospedagem (casca fina)

App Kotlin mínimo que **embute o servidor Go** e cuida da rede: cria um
**LocalOnlyHotspot** (SSID/senha gerados pelo sistema), executa o binário como
processo filho e exibe **dois QR codes** — (1) entrar no Wi-Fi, (2) abrir a URL
do sistema. Objetivo: zero configuração manual pra Mesa.

> **Status: SPIKE.** Valida duas incógnitas: (a) executar o binário Go fora do
> Termux, vindo do APK; (b) LocalOnlyHotspot com QR de pareamento. Resultados
> e limites medidos ficam documentados no fim deste arquivo.

## Modos de rede

O app tem dois modos (seletor na tela inicial; a escolha é lembrada):

1. **Criar rede Wi-Fi (hotspot automático)** — o LocalOnlyHotspot do Android.
   Zero configuração, mas o **nome e a senha são gerados pelo sistema**
   (`AndroidShare_XXXX`) — a API pública não permite personalizá-los (a
   variante com `SoftApConfiguration` é restrita a apps de sistema).
2. **Usar a rede atual** — o app não cria nada: serve na rede em que o celular
   já está. Cobre dois cenários:
   - **Wi-Fi do local** (ex.: o da igreja): conecte o celular ao Wi-Fi e toque
     Iniciar. Só o QR da URL é exibido — os aparelhos entram na mesma rede por
     conta própria. ⚠️ Roteadores de igreja às vezes têm **isolamento de
     clientes (AP isolation)**: um celular não enxerga o outro e o sistema não
     abre. **Teste antes do evento**; se isolar, use o hotspot.
   - **Hotspot do sistema com nome personalizado** (ex.: `Congresso_9440` /
     senha `123456789`): configure uma vez em *Configurações → Ponto de acesso
     Wi-Fi* (nome e senha que quiser), ligue o hotspot, e use este modo — o app
     detecta a interface (`ap*`/`swlan*`) e serve nela. É o caminho para SSID
     customizado, já que a API não deixa o app criar hotspot com nome próprio.

## Como funciona

- `build-go.sh` compila o servidor da raiz (`GOOS=linux GOARCH=arm64
  -buildmode=pie`) e o grava como `app/src/main/jniLibs/arm64-v8a/libvotacao.so`.
  O W^X do Android só permite `exec()` de binários vindos do APK pelo caminho
  das bibliotecas nativas (`nativeLibraryDir`) — o mesmo truque do Termux.
  O script também corrige o alinhamento do segmento TLS para 64 bytes
  (exigência do Bionic ARM64; equivale ao `termux-elf-cleaner`).
- `ServerService` (foreground + wake lock): cria o hotspot, acha o IP da
  interface (`ap*`/`swlan*`), executa
  `libvotacao.so -addr :8090 -host <ip> -data <filesDir>` — a pasta inteira é o
  acervo de eleições (um `.db` por eleição, ADR-0012; o gerenciador da Mesa em
  `/board/eleicoes` cria/troca/reseta/exclui a quente) —, faz health-check TCP
  e publica o estado pra Activity. Logs do Go saem no logcat com a tag
  `votacao-go`.
- `MainActivity`: identidade visual do app web (verde IPB sobre branco-quente,
  logo oficial), header com **bolinha de status** (cinza Parado · âmbar
  Iniciando · verde No ar · vermelho Erro) e mensagem detalhada quando há algo
  a dizer; permissões de runtime por modo; botão de bateria que **some** quando
  a isenção já foi concedida; abas **QR Codes** (os dois QRs, zxing) e **Logs**
  (servidor Go + eventos do app, ring buffer de 500 linhas em memória,
  monoespaçado com auto-scroll e botão Copiar — útil pra suporte sem adb).

## Build

Pré-requisitos (uma vez):

```bash
brew install openjdk@17 gradle
brew install --cask android-commandlinetools
export JAVA_HOME=/opt/homebrew/opt/openjdk@17
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
yes | sdkmanager --licenses
sdkmanager "platforms;android-34" "build-tools;34.0.0"
echo "sdk.dir=$ANDROID_HOME" > android/local.properties
```

Build (sempre que o servidor Go mudar, rode o passo 1 de novo):

```bash
./android/build-go.sh                                   # 1. servidor Go → libvotacao.so
cd android && JAVA_HOME=/opt/homebrew/opt/openjdk@17 \
  gradle assembleDebug                                  # 2. APK
```

APK em `android/app/build/outputs/apk/debug/app-debug.apk`.

## Instalar e testar no aparelho

```bash
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n app.votacao.host/.MainActivity

# acompanhar logs (app + servidor Go):
adb logcat -s votacao-host:V votacao-go:V
```

### Roteiro do teste físico

1. Abrir o app → **Iniciar votação** → conceder permissões.
   *Pré-requisitos no aparelho: Localização **ligada** (Android ≤12) e o
   hotspot/tethering comum **desligado** (o LocalOnlyHotspot não convive com ele).*
2. Tocar em **Permitir em segundo plano** e aceitar (isenção de bateria).
3. Critério de sucesso: noutro celular, escanear o **QR 1** (entra no Wi-Fi),
   escanear o **QR 2** (abre `http://<ip>:8090/`) e ver a home do sistema —
   sem digitar nada.
4. **Sobrevivência**: apagar a tela do aparelho-servidor e deixar ≥30 min;
   o cliente deve continuar navegando (a notificação persistente fica visível).
5. **Capacidade**: conectar quantos aparelhos conseguir no hotspot (família,
   amigos…) e anotar em qual número novas conexões passam a falhar. Esse
   número decide se o app serve pra congresso BYOD ou só com roteador externo.
   Dica: `adb shell ip neigh show` lista os clientes associados.

### Solução de problemas

| Sintoma | Causa provável |
|---|---|
| "Falha ao criar a rede: sem canal/modo incompatível" | Hotspot comum ligado, ou Wi-Fi desligado (modo "criar rede") |
| "Sem rede: conecte o celular a um Wi-Fi…" | Modo "rede atual" sem Wi-Fi conectado nem hotspot do sistema ligado |
| Cliente na mesma rede não abre a URL | AP isolation no roteador do local — teste outro roteador ou use o hotspot |
| `SecurityException` ao iniciar | Permissão negada ou Localização desligada (≤ Android 12) |
| "Binário do servidor não está no APK" | Faltou rodar `./android/build-go.sh` antes do `gradle` |
| Servidor morre com a tela apagada | Isenção de bateria não concedida (botão na tela) |
| Erro `TLS segment is underaligned` no logcat | Patch do build-go.sh não rodou — recompile |

## Descobertas do spike (teste físico em 2026-06-10)

- [x] **Incógnita 1 — exec do binário do APK: VALIDADA.** O Go PIE exige DOIS
  patches ELF (ambos no `build-go.sh`):
  1. `PT_INTERP` `/lib/ld-linux-aarch64.so.1` → `/system/bin/linker64` — sem
     isso o exec falha com `ENOENT` *mesmo com o arquivo presente* (o "No such
     file" é do interpretador, não do binário; no Termux quem fazia isso era o
     `termux-elf-cleaner`);
  2. `PT_TLS p_align` 8 → 64 (exigência do Bionic ARM64).
  Com os dois, o servidor sobe como processo filho e responde na porta
  (`votacao-go: votação no ar...` no logcat).
- [x] **Incógnita 2 — LocalOnlyHotspot: VALIDADA.** SSID/senha geradas
  (`AndroidShare_*`), interface `ap0` detectada com IPv4 (ex.
  `10.247.171.219`) e repassada ao binário via `-host`.
- [x] **Critério de sucesso: VALIDADO.** Cliente (MacBook) escaneou o QR 1,
  entrou na rede, escaneou o QR 2 e abriu a home do sistema — zero configuração
  manual.
- [~] Sobrevivência ≥30 min de tela apagada: **não cronometrado** (foreground
  service + wake lock + isenção de bateria tornam o risco baixo; observar na
  primeira eleição real).
- [ ] **Limite de clientes simultâneos: NÃO MEDIDO** (sem aparelhos
  suficientes) — será observado em produção. ⚠️ Planejamento: a literatura
  aponta ~10 clientes como teto típico de LocalOnlyHotspot em muitos chipsets.
  Até medir, congresso BYOD com dezenas de delegados deve manter o **roteador
  externo** como plano A; o app é plano A para plenárias locais (rol pequeno)
  e plano B/contingência nas demais.
- Decisão de empacotamento registrada em
  [ADR-0011](../docs/adr/0011-app-android-casca-fina.md).

## Fora de escopo do spike

iOS, Play Store, `gomobile bind` (fica pra depois, se o spike validar) e
qualquer mudança no servidor Go além da flag `-host`, que já existia.
