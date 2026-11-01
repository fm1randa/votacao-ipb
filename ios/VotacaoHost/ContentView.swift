import SwiftUI
import UIKit

/// Paleta espelhada do app web (base.html, tema claro) — a mesma do Android.
enum Cor {
    static let papel = Color(hex: 0xF4F6F2)
    static let superficie = Color.white
    static let tinta = Color(hex: 0x1E231F)
    static let apagado = Color(hex: 0x5F6B62)
    static let linha = Color(hex: 0xE1E7DF)
    static let verde = Color(hex: 0x0D5131)
    static let ouro = Color(hex: 0xA8801F)
    static let ok = Color(hex: 0x1F7A4D)
    static let ruim = Color(hex: 0xC0392B)
}

extension Color {
    init(hex: UInt32) {
        self.init(red: Double((hex >> 16) & 0xFF) / 255,
                  green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255)
    }
}

/// Tela única: header com bolinha de status, seletor de modo, botão liga/desliga
/// e as abas QR Codes (os dois QRs) e Logs (servidor Go + eventos do app).
struct ContentView: View {
    @Bindable var host: ServidorHost

    enum Aba: String, CaseIterable { case qr = "QR Codes", logs = "Logs" }
    @State private var aba: Aba = .qr
    @State private var mesaAberta = false
    @State private var copiado = false

    private let ticker = Timer.publish(every: 2, on: .main, in: .common).autoconnect()

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                cabecalho
                seletorModo
                if host.modo == .acessoPessoal { cartaoWifi }
                botoes
                if exibeDetalhe {
                    Text(host.estado.mensagem)
                        .font(.callout)
                        .foregroundStyle(host.estado.tipo == .erro ? Cor.ruim : Cor.apagado)
                        .multilineTextAlignment(.center)
                        .frame(maxWidth: .infinity)
                }
                Picker("Aba", selection: $aba) {
                    ForEach(Aba.allCases, id: \.self) { Text($0.rawValue).tag($0) }
                }
                .pickerStyle(.segmented)
                switch aba {
                case .qr: painelQR
                case .logs: painelLogs
                }
            }
            .padding(20)
        }
        .background(Cor.papel.ignoresSafeArea())
        .foregroundStyle(Cor.tinta)
        .onReceive(ticker) { _ in host.tick() }
        // Os QR codes ficam expostos durante o credenciamento — tela sempre acesa.
        .onChange(of: host.estado.rodando, initial: true) { _, rodando in
            UIApplication.shared.isIdleTimerDisabled = rodando
        }
        .sheet(isPresented: $mesaAberta) {
            if let url = URL(string: "http://127.0.0.1:\(host.porta)/board") {
                MesaView(url: url)
            }
        }
    }

    private var exibeDetalhe: Bool {
        !host.estado.mensagem.isEmpty && host.estado.tipo != .parado
    }

    // MARK: Header: logo · título · bolinha de status

    private var cabecalho: some View {
        HStack(spacing: 10) {
            Image("logo_ipb")
                .resizable().scaledToFit()
                .frame(width: 44, height: 44)
                .accessibilityLabel("Igreja Presbiteriana do Brasil")
            Text("Servidor de votação")
                .font(.system(size: 21, weight: .bold))
            Spacer()
            Circle().fill(corStatus).frame(width: 14, height: 14)
            Text(rotuloStatus).font(.footnote).foregroundStyle(Cor.apagado)
                .accessibilityIdentifier("status")
        }
    }

    private var corStatus: Color {
        switch host.estado.tipo {
        case .noAr: Cor.ok
        case .iniciando: Cor.ouro
        case .erro: Cor.ruim
        case .parado: Cor.apagado
        }
    }

    private var rotuloStatus: String {
        switch host.estado.tipo {
        case .noAr: "No ar"
        case .iniciando: "Iniciando…"
        case .erro: "Erro"
        case .parado: "Parado"
        }
    }

    // MARK: Modo de rede (não muda com o servidor no ar — pare antes de trocar)

    private var seletorModo: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(ModoRede.allCases) { modo in
                Button {
                    host.modo = modo
                } label: {
                    HStack(alignment: .top, spacing: 10) {
                        Image(systemName: host.modo == modo ? "largecircle.fill.circle" : "circle")
                            .foregroundStyle(Cor.verde)
                        Text(modo.titulo).multilineTextAlignment(.leading)
                        Spacer(minLength: 0)
                    }
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(host.modo == modo ? .isSelected : [])
            }
        }
        .disabled(host.estado.rodando || host.estado.tipo == .iniciando)
        .opacity(host.estado.rodando ? 0.5 : 1)
    }

    /// Nome e senha do Acesso Pessoal, para o QR 1. Opcional: sem eles, os
    /// aparelhos entram na rede digitando a senha.
    private var cartaoWifi: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Ligue em Ajustes › Acesso Pessoal. O iOS não deixa o app criar a rede nem ler a senha: preencha abaixo para gerar o QR do Wi-Fi.")
                .font(.footnote).foregroundStyle(Cor.apagado)
            TextField("Nome da rede (ex.: iPhone de Filipe)", text: $host.wifiNome)
                .textInputAutocapitalization(.never).autocorrectionDisabled()
            SecureField("Senha do Acesso Pessoal", text: $host.wifiSenha)
        }
        .textFieldStyle(.roundedBorder)
        .padding(12)
        .background(Cor.superficie, in: RoundedRectangle(cornerRadius: 12))
        .disabled(host.estado.rodando)
    }

    // MARK: Botões

    private var botoes: some View {
        VStack(spacing: 8) {
            Button {
                Task {
                    if host.estado.rodando { await host.parar() } else { await host.iniciar() }
                }
            } label: {
                Text(host.estado.rodando ? "Parar" : "Iniciar votação")
                    .bold().frame(maxWidth: .infinity).padding(.vertical, 12)
            }
            .background(Cor.verde, in: RoundedRectangle(cornerRadius: 12))
            .foregroundStyle(.white)
            .disabled(host.estado.tipo == .iniciando)

            if host.estado.rodando {
                Button {
                    mesaAberta = true
                } label: {
                    Text("Abrir a Mesa neste iPhone")
                        .frame(maxWidth: .infinity).padding(.vertical, 12)
                }
                .background(Cor.superficie, in: RoundedRectangle(cornerRadius: 12))
                .foregroundStyle(Cor.tinta)
            }
        }
    }

    // MARK: Aba QR Codes

    private var painelQR: some View {
        VStack(spacing: 18) {
            if let wifi = host.payloadWifi {
                cartaoQR(titulo: "1. Entre na rede Wi-Fi", conteudo: wifi,
                         legenda: "Rede: \(host.wifiNome)\nSenha: \(host.wifiSenha)")
            }
            if host.estado.rodando, let url = host.estado.url {
                cartaoQR(titulo: host.payloadWifi == nil ? "Abra o sistema" : "2. Abra o sistema",
                         conteudo: url, legenda: url)
            }
            if !host.estado.rodando {
                Text("Os QR codes aparecem quando o servidor estiver no ar.")
                    .font(.callout).foregroundStyle(Cor.apagado)
                    .frame(maxWidth: .infinity)
            }
        }
    }

    private func cartaoQR(titulo: String, conteudo: String, legenda: String) -> some View {
        VStack(spacing: 8) {
            Text(titulo).bold().foregroundStyle(Cor.verde)
            if let img = QR.imagem(conteudo) {
                Image(uiImage: img)
                    .interpolation(.none).resizable().scaledToFit()
                    .frame(width: 250, height: 250)
                    .accessibilityLabel(titulo)
            }
            Text(legenda).multilineTextAlignment(.center).textSelection(.enabled)
        }
        .frame(maxWidth: .infinity)
    }

    // MARK: Aba Logs

    private var painelLogs: some View {
        VStack(spacing: 8) {
            ScrollViewReader { proxy in
                ScrollView {
                    Text(host.logs.isEmpty ? "Nenhum log ainda — inicie a votação." : host.logs)
                        .font(.system(size: 12, design: .monospaced))
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(10)
                    Color.clear.frame(height: 1).id("fim")
                }
                .frame(height: 380)
                .background(Cor.superficie)
                .onChange(of: host.logs) { proxy.scrollTo("fim") } // auto-scroll
                .onAppear { proxy.scrollTo("fim") }
            }
            Button(copiado ? "Logs copiados." : "Copiar logs") {
                UIPasteboard.general.string = host.logs
                copiado = true
                Task { try? await Task.sleep(for: .seconds(2)); copiado = false }
            }
            .frame(maxWidth: .infinity).padding(.vertical, 10)
            .background(Cor.superficie, in: RoundedRectangle(cornerRadius: 12))
        }
        .onAppear { host.tick() }
    }
}
