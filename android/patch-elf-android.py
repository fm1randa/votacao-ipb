"""
Corrige o binário ELF (arm64) do servidor Go para execução direta no
Android/Bionic (compartilhado pelo APK e pelo artefato Termux da release):

  1. PT_INTERP — substitui /lib/ld-linux-aarch64.so.1 (não existe no Android)
     por /system/bin/linker64, que é o loader do Bionic.

  2. PT_TLS — o Go PIE alinha o segmento TLS em 8 bytes; a Bionic ARM64 exige
     64 bytes ("TLS segment is underaligned"). O campo p_align é ajustado para 64.

Uso: python3 patch-elf-android.py <caminho-do-elf>
"""
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
