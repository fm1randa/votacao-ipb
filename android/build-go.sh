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
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -buildmode=pie -trimpath \
  -o "$OUT/libvotacao.so" .

echo "==> patch ELF p/ Android (PT_INTERP -> linker64; PT_TLS align -> 64)"
python3 - "$OUT/libvotacao.so" <<'EOF'
import struct, sys
ANDROID_LINKER = b'/system/bin/linker64\x00'
path = sys.argv[1]
with open(path, 'r+b') as f:
    data = bytearray(f.read())
    assert data[:4] == b'\x7fELF' and data[4] == 2, 'esperava ELF64'
    e_phoff = struct.unpack_from('<Q', data, 0x20)[0]
    e_phentsize = struct.unpack_from('<H', data, 0x36)[0]
    e_phnum = struct.unpack_from('<H', data, 0x38)[0]
    PT_INTERP, PT_TLS = 3, 7
    for i in range(e_phnum):
        off = e_phoff + i * e_phentsize
        p_type = struct.unpack_from('<I', data, off)[0]
        if p_type == PT_INTERP:
            p_offset = struct.unpack_from('<Q', data, off + 0x08)[0]
            p_filesz = struct.unpack_from('<Q', data, off + 0x20)[0]
            old = bytes(data[p_offset:p_offset + p_filesz])
            if old != ANDROID_LINKER.ljust(p_filesz, b'\x00'):
                assert len(ANDROID_LINKER) <= p_filesz, 'interp novo não cabe no segmento'
                # sobrescreve in-place com padding NUL — o kernel lê a C-string
                data[p_offset:p_offset + p_filesz] = ANDROID_LINKER.ljust(p_filesz, b'\x00')
                print(f'   PT_INTERP: {old.rstrip(bytes(1)).decode()} -> /system/bin/linker64')
        elif p_type == PT_TLS:
            p_align = struct.unpack_from('<Q', data, off + 0x30)[0]
            if p_align < 64:
                struct.pack_into('<Q', data, off + 0x30, 64)
                print(f'   PT_TLS p_align: {p_align} -> 64')
    f.seek(0); f.write(data)
    print('   ok')
EOF

ls -lh "$OUT/libvotacao.so"
echo "==> pronto. Agora: cd android && gradle assembleDebug"
