# App Android "casca fina" hospedando o servidor Go (monorepo android/)

Para eliminar a fricção de setup (LAN, servidor, IP — o maior gap de adoção),
o sistema ganha um **app Android mínimo** que embute o binário Go e cuida da
rede: cria um **LocalOnlyHotspot**, executa o servidor como **processo filho**
e exibe **dois QR codes** (entrar no Wi-Fi; abrir a URL). A Mesa instala um
APK e toca um botão — validado em aparelho real: outro dispositivo escaneou os
dois QRs e chegou à home sem nenhuma configuração manual.

**Empacotamento (o truque do Termux):** o W^X do Android (API 29+) só permite
`exec()` de binários que vieram do APK pelo caminho das bibliotecas nativas.
O servidor é compilado da raiz do monorepo (`GOOS=linux GOARCH=arm64
-buildmode=pie`, sem cgo) e gravado como
`android/app/src/main/jniLibs/arm64-v8a/libvotacao.so`
(`useLegacyPackaging`/`extractNativeLibs` garantem a extração para
`nativeLibraryDir`, de onde o exec é permitido). O app é só uma casca: o motor
da eleição continua sendo o MESMO binário único de sempre.

**Dois patches ELF obrigatórios** (aplicados pelo `android/build-go.sh`, em
Python puro, sem depender do termux-elf-cleaner):
1. **PT_INTERP** — o Go PIE grava `/lib/ld-linux-aarch64.so.1` (glibc), que
   não existe no Android; sem reescrever para `/system/bin/linker64`, o exec
   falha com um `ENOENT` enganoso ("No such file" do interpretador, não do
   binário — o arquivo existe). Descoberto no teste físico do spike.
2. **PT_TLS p_align 8 → 64** — o Bionic ARM64 exige TLS alinhado em 64 bytes
   ("TLS segment is underaligned", já conhecido da era Termux).

**Rede:** `LocalOnlyHotspot` (API 26+, daí o minSdk) com SSID/senha geradas
pelo sistema; a interface (`ap*`/`swlan*`) é detectada e o IPv4 vai ao binário
via `-host` (flag que já existia para o Termux). Foreground service
(`connectedDevice`) + wake lock + isenção de otimização de bateria mantêm o
servidor vivo de tela apagada. Logs do Go saem no logcat (tag `votacao-go`).

**Alternativas descartadas:**
- *Termux* (status quo): funciona, mas exige instalar F-Droid + Termux + mover
  binário + wake-lock manual — exatamente a fricção que afasta usuários leigos.
- *gomobile bind* (servidor como biblioteca in-process): sem fronteira de
  processo, builds mais acoplados e perde-se o binário único multiuso; fica
  como evolução possível, não necessidade.
- *Toolchain NDK (CGO + linkmode external)*: resolveria o PT_INTERP "de
  fábrica", ao custo de exigir NDK em toda máquina de build; dois patches de
  20 linhas no ELF custam menos.

**Consequências e riscos aceitos:**
- O repo vira **monorepo**: `android/` é um projeto Gradle standalone
  (minSdk 26), fora do `go.mod`; `build-go.sh` deve rodar a cada mudança no
  servidor antes do `gradle assembleDebug`.
- **Limite de clientes do LocalOnlyHotspot não medido** (tipicamente ~10 por
  chipset). Até a medição em eleição real: roteador externo continua sendo o
  plano A para congressos BYOD grandes; o app é plano A para plenárias locais
  (rol pequeno) e contingência nas demais.
- O patch de PT_INTERP assume Android arm64 (`linker64`) — o APK só embarca
  `arm64-v8a`, coerente com isso.
- SSID/senha mudam a cada "Iniciar" (geradas pelo sistema) — clientes precisam
  re-escanear o QR 1 se o hotspot for reiniciado no meio do evento.
