# App iOS "casca fina" com o servidor Go no próprio processo (ios/)

O app Android ([ADR-0011](0011-app-android-casca-fina.md)) ganha um porte para
iOS com o mesmo papel: a Mesa instala um app, toca **Iniciar** e mostra os QR
codes, sem notebook. O motor continua sendo o **mesmo binário único**. O app
não reimplementa nenhuma regra de negócio, só hospeda o servidor e cuida da rede.

**Execução no processo, não processo filho.** O iOS proíbe `exec()` de
binários, então o truque do Android não se aplica. O bootstrap que vivia no
`main.go` passou para `internal/cli.Run(args, stop)`, que devolve erros em vez
de `log.Fatal`, para não derrubar o app hospedeiro. Ele também passa a aceitar
um canal de parada. O `main.go` virou um invólucro de sinais. A ponte
`ios/gobridge` (package main + cgo, `-buildmode=c-archive`) expõe
`VotacaoIniciar/Parar/Rodando/Erro/Logs`, e o `ios/build-go.sh` empacota
aparelho e simulador em `VotacaoGo.xcframework`. O app chama `Run` com os
**mesmos argumentos** que o Android passa ao processo filho
(`-addr :8090 -host <ip> -data <pasta>`).

**Rede.** Não há API pública para o app criar um hotspot, e nenhum
equivalente ao LocalOnlyHotspot. Os modos ficam assim:
- *Acesso Pessoal*: a Mesa liga em Ajustes, e o app serve na interface
  `bridge*`.
- *Rede atual*: o Wi-Fi do local.

O iOS também não expõe o SSID nem a senha do Acesso Pessoal. Para gerar o QR
do Wi-Fi, a Mesa digita os dois uma vez (opcional).

**Alternativas descartadas:**
- *gomobile bind*: gera uma API Objective-C mais rica, mas traz
  `golang.org/x/mobile` como ferramenta de build. A ponte C tem 6 funções e
  dispensa isso.
- *Cliente nativo (votante/Mesa em SwiftUI com API JSON)*: duplicaria telas
  e fluxos do web e exigiria uma API nova. O pedido foi portar a casca
  Android.
- *Background audio/VoIP para fugir da suspensão*: é uso indevido de modos
  de segundo plano e seria rejeitado na revisão da App Store.

**Consequências e riscos aceitos:**
- **Só em primeiro plano.** O iOS suspende o app em segundo plano e o
  servidor para de responder. Mitigações: a tela fica acesa com o servidor no
  ar; a Mesa abre num WKWebView dentro do app ("Abrir a Mesa neste iPhone")
  para não ir ao Safari; ao voltar ao primeiro plano, o app testa a porta e
  reinicia o servidor se o socket tiver sido recolhido. O iPhone é plano A
  para plenárias curtas e contingência; notebook ou Android seguem como plano
  A nos congressos.
- O xcframework (~33 MB) **não é versionado**: é gerado pelo `build-go.sh`
  (ou pela pre-action do scheme), como o `libvotacao.so` do Android.
- O `.xcodeproj` é gerado pelo XcodeGen a partir de `ios/project.yml` e fica
  versionado, para abrir sem instalar nada além do Xcode.
- Sem release automática do iOS na ADR-0014: distribuir exige conta Apple
  Developer (TestFlight), o que fica para depois.
- O limite de clientes do Acesso Pessoal não foi medido (mesma pendência do
  Android).
