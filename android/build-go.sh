#!/usr/bin/env bash
# Compila o servidor Go da raiz do repo e o empacota como "biblioteca nativa"
# do app Android (jniLibs/arm64-v8a/libvotacao.so).
#
# Por que .so? O W^X do Android (API 29+) só permite exec() de binários que
# vieram do APK pelo caminho das bibliotecas nativas (nativeLibraryDir) — o
# mesmo truque do Termux. O "lib...so" é, na verdade, o executável Go.
#
# Duas pegadinhas do Android, corrigidas com patch ELF direto (em vez de
# depender do termux-elf-cleaner):
#  1. PT_INTERP: o Go PIE aponta para /lib/ld-linux-aarch64.so.1, que não
#     existe no Android (exec falha com ENOENT) → vira /system/bin/linker64;
#  2. PT_TLS: o Bionic ARM64 exige TLS alinhado em 64 bytes; o Go alinha em 8
#     ("TLS segment is underaligned") → p_align vira 64.
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="android/app/src/main/jniLibs/arm64-v8a"
mkdir -p "$OUT"

echo "==> go build (linux/arm64, PIE)"
LDFLAGS=""
if [[ -n "${VERSION:-}" ]]; then
  LDFLAGS="-X votacao-ipb/internal/web.Version=${VERSION}"
fi
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -buildmode=pie -trimpath \
  -ldflags "$LDFLAGS" -o "$OUT/libvotacao.so" .

echo "==> patch ELF p/ Android (PT_INTERP -> linker64; PT_TLS align -> 64)"
python3 "$(dirname "$0")/patch-elf-android.py" "$OUT/libvotacao.so"

ls -lh "$OUT/libvotacao.so"
echo "==> pronto. Agora: cd android && ./gradlew assembleDebug"
