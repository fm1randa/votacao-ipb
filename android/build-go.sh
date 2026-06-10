#!/usr/bin/env bash
# Compila o servidor Go da raiz do repo e o empacota como "biblioteca nativa"
# do app Android (jniLibs/arm64-v8a/libvotacao.so).
#
# Por que .so? O W^X do Android (API 29+) só permite exec() de binários que
# vieram do APK pelo caminho das bibliotecas nativas (nativeLibraryDir) — o
# mesmo truque do Termux. O "lib...so" é, na verdade, o executável Go.
#
# Pegadinha do Bionic ARM64: o linker exige segmento TLS alinhado em 64 bytes;
# o Go alinha em 8 ("TLS segment is underaligned"). Em vez de depender do
# termux-elf-cleaner, o patch_tls_align abaixo ajusta o p_align do PT_TLS
# direto no ELF (mesma correção, em ~20 linhas de Python).
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="android/app/src/main/jniLibs/arm64-v8a"
mkdir -p "$OUT"

echo "==> go build (linux/arm64, PIE)"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -buildmode=pie -trimpath \
  -o "$OUT/libvotacao.so" .

echo "==> patch: alinhamento do segmento TLS p/ 64 (Bionic ARM64)"
python3 - "$OUT/libvotacao.so" <<'EOF'
import struct, sys
path = sys.argv[1]
with open(path, 'r+b') as f:
    data = bytearray(f.read())
    assert data[:4] == b'\x7fELF' and data[4] == 2, 'esperava ELF64'
    e_phoff = struct.unpack_from('<Q', data, 0x20)[0]
    e_phentsize = struct.unpack_from('<H', data, 0x36)[0]
    e_phnum = struct.unpack_from('<H', data, 0x38)[0]
    PT_TLS = 7
    patched = False
    for i in range(e_phnum):
        off = e_phoff + i * e_phentsize
        p_type = struct.unpack_from('<I', data, off)[0]
        if p_type == PT_TLS:
            p_align = struct.unpack_from('<Q', data, off + 0x30)[0]
            if p_align < 64:
                struct.pack_into('<Q', data, off + 0x30, 64)
                patched = True
                print(f'   PT_TLS p_align: {p_align} -> 64')
    f.seek(0); f.write(data)
    print('   ok' + ('' if patched else ' (já alinhado — nada a fazer)'))
EOF

ls -lh "$OUT/libvotacao.so"
echo "==> pronto. Agora: cd android && gradle assembleDebug"
