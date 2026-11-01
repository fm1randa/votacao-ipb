#!/usr/bin/env bash
# Compila o servidor Go como biblioteca estática (c-archive) para iOS e monta
# ios/VotacaoGo.xcframework (aparelho arm64 + simulador arm64). O app linka
# esse framework e roda o servidor no próprio processo (ADR-0015).
#
# Rode de novo sempre que o servidor Go mudar, antes do xcodebuild.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/ios/build/go"
XCF="$ROOT/ios/VotacaoGo.xcframework"
MIN_IOS=17.0

# Versão no rodapé da Mesa e no log, igual aos binários de release (ADR-0014).
VERSION="${VOTACAO_VERSION:-dev+$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo local)}"
LDFLAGS="-s -w -X votacao-ipb/internal/web.Version=$VERSION"

build() { # $1 = sdk (iphoneos|iphonesimulator), $2 = alvo clang, $3 = pasta de saída
  local sdk="$1" target="$2" dir="$OUT/$3"
  local sysroot cc
  sysroot="$(xcrun --sdk "$sdk" --show-sdk-path)"
  cc="$(xcrun --sdk "$sdk" --find clang)"
  mkdir -p "$dir"
  echo "==> $3 ($target)"
  (cd "$ROOT" && \
    CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
    CC="$cc" \
    CGO_CFLAGS="-isysroot $sysroot -target $target -fembed-bitcode=off" \
    CGO_LDFLAGS="-isysroot $sysroot -target $target" \
    go build -buildmode=c-archive -trimpath -ldflags "$LDFLAGS" \
      -o "$dir/libvotacao.a" ./ios/gobridge)
  # Module map: o Swift importa a ponte como `import VotacaoGo`.
  cat > "$dir/module.modulemap" <<'EOF'
module VotacaoGo {
    header "libvotacao.h"
    link "resolv"
    export *
}
EOF
  mkdir -p "$dir/include"
  mv "$dir/libvotacao.h" "$dir/module.modulemap" "$dir/include/"
}

rm -rf "$OUT" "$XCF"
build iphoneos        "arm64-apple-ios$MIN_IOS"           ios-arm64
build iphonesimulator "arm64-apple-ios$MIN_IOS-simulator" ios-arm64-simulator

xcodebuild -create-xcframework \
  -library "$OUT/ios-arm64/libvotacao.a"           -headers "$OUT/ios-arm64/include" \
  -library "$OUT/ios-arm64-simulator/libvotacao.a" -headers "$OUT/ios-arm64-simulator/include" \
  -output "$XCF"

echo "ok: $XCF ($VERSION)"
