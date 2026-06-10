# App Android — spike de hospedagem (casca fina)

App Kotlin mínimo que **embute o servidor Go** e cuida da rede: cria um
**LocalOnlyHotspot** (SSID/senha gerados pelo sistema), executa o binário como
processo filho e exibe **dois QR codes** — (1) entrar no Wi-Fi, (2) abrir a URL
do sistema. Objetivo: zero configuração manual pra Mesa.

> **Status: SPIKE.** Valida duas incógnitas: (a) executar o binário Go fora do
> Termux, vindo do APK; (b) LocalOnlyHotspot com QR de pareamento. Resultados
> e limites medidos ficam documentados no fim deste arquivo.

## Como funciona

- `build-go.sh` compila o servidor da raiz (`GOOS=linux GOARCH=arm64
  -buildmode=pie`) e o grava como `app/src/main/jniLibs/arm64-v8a/libvotacao.so`.
  O W^X do Android só permite `exec()` de binários vindos do APK pelo caminho
  das bibliotecas nativas (`nativeLibraryDir`) — o mesmo truque do Termux.
  O script também corrige o alinhamento do segmento TLS para 64 bytes
  (exigência do Bionic ARM64; equivale ao `termux-elf-cleaner`).
- `ServerService` (foreground + wake lock): cria o hotspot, acha o IP da
  interface (`ap*`/`swlan*`), executa
  `libvotacao.so -addr :8090 -host <ip> -db <filesDir>/votacao.db`,
  faz health-check TCP e publica o estado pra Activity. Logs do Go saem no
  logcat com a tag `votacao-go`.
- `MainActivity`: permissões de runtime, botão liga/desliga, isenção de
  otimização de bateria e os dois QR codes (zxing).

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
| "Falha ao criar a rede: sem canal/modo incompatível" | Hotspot comum ligado, ou Wi-Fi desligado |
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
- [ ] Critério de sucesso (2º celular: QR Wi-Fi → QR URL → home): **pendente**
- [ ] Sobrevivência ≥30 min de tela apagada: **pendente**
- [ ] Limite de clientes simultâneos no LocalOnlyHotspot (aparelho: ____): **___ clientes**
- Se o restante validar: registrar a decisão de empacotamento como ADR em `docs/adr/`.

## Fora de escopo do spike

iOS, Play Store, `gomobile bind` (fica pra depois, se o spike validar) e
qualquer mudança no servidor Go além da flag `-host`, que já existia.
