import Foundation
import Observation

/// Estado publicado para a tela (espelha o HostState do Android).
struct EstadoHost: Equatable {
    /// Cor da bolinha do header: cinza, âmbar, verde, vermelho.
    enum Tipo: Equatable { case parado, iniciando, noAr, erro }

    var tipo: Tipo
    /// Mensagem detalhada em pt (linha abaixo do botão).
    var mensagem: String
    /// Servidor respondendo na porta.
    var rodando: Bool
    var url: String? = nil

    static let inicial = EstadoHost(tipo: .parado, mensagem: "", rodando: false)
}

/// Orquestra a casca: escolhe a interface, sobe o servidor Go embutido, faz o
/// health check e publica o estado. Equivale ao ServerService do Android.
@MainActor
@Observable
final class ServidorHost {
    nonisolated static let portaPadrao = 8090

    private(set) var estado = EstadoHost.inicial
    private(set) var logs = ""

    /// Modo escolhido (lembrado entre usos).
    var modo: ModoRede {
        didSet { defaults.set(modo.rawValue, forKey: "modo") }
    }
    /// Nome e senha do Acesso Pessoal, só para montar o QR do Wi-Fi: o iOS não
    /// expõe essas credenciais ao app (no Android vêm do LocalOnlyHotspot).
    var wifiNome: String {
        didSet { defaults.set(wifiNome, forKey: "wifiNome") }
    }
    var wifiSenha: String {
        didSet { defaults.set(wifiSenha, forKey: "wifiSenha") }
    }

    /// QR do Wi-Fi: no ar, no modo Acesso Pessoal, com nome e senha preenchidos.
    var payloadWifi: String? {
        guard estado.rodando, modo == .acessoPessoal,
              !wifiNome.isEmpty, !wifiSenha.isEmpty else { return nil }
        return QR.payloadWifi(nome: wifiNome, senha: wifiSenha)
    }

    let porta: Int
    private let motor: MotorServidor
    private let sonda: SondaServidor
    private let interfaces: () -> [InterfaceRede]
    private let pastaDados: URL
    private let defaults: UserDefaults
    private let intervalo: Duration
    private var parando = false

    init(
        motor: MotorServidor = MotorGo(),
        sonda: SondaServidor = SondaTCP(),
        interfaces: @escaping () -> [InterfaceRede] = Rede.interfacesAtivas,
        pastaDados: URL = ServidorHost.pastaPadrao(),
        porta: Int = ServidorHost.portaPadrao,
        defaults: UserDefaults = .standard,
        intervalo: Duration = .milliseconds(500)
    ) {
        self.motor = motor
        self.sonda = sonda
        self.interfaces = interfaces
        self.pastaDados = pastaDados
        self.porta = porta
        self.defaults = defaults
        self.intervalo = intervalo
        modo = ModoRede(rawValue: defaults.string(forKey: "modo") ?? "") ?? .acessoPessoal
        wifiNome = defaults.string(forKey: "wifiNome") ?? ""
        wifiSenha = defaults.string(forKey: "wifiSenha") ?? ""
    }

    /// Pasta de dados: o acervo de eleições (um .db por eleição, ADR-0012).
    /// Fica em Application Support — sai no backup do iCloud/Finder.
    nonisolated static func pastaPadrao() -> URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        return base.appendingPathComponent("eleicoes", isDirectory: true)
    }

    private func log(_ linha: String) {
        motor.registrar(linha)
        logs = motor.logs()
    }

    private func publicar(_ novo: EstadoHost) { estado = novo }

    func iniciar() async {
        guard !estado.rodando, estado.tipo != .iniciando else { return }
        parando = false
        publicar(EstadoHost(tipo: .iniciando, mensagem: "Procurando a rede…", rodando: false))
        log("modo: \(modo == .acessoPessoal ? "Acesso Pessoal" : "usar a rede atual")")

        guard let iface = await procurarInterface() else {
            let msg = modo == .acessoPessoal
                ? "Acesso Pessoal desligado: ligue em Ajustes › Acesso Pessoal (\"Permitir Outras Conexões\") e tente de novo."
                : "Sem rede: conecte o iPhone a um Wi-Fi (ou ligue o Acesso Pessoal) e tente de novo."
            log("ERRO: \(msg)")
            publicar(EstadoHost(tipo: .erro, mensagem: msg, rodando: false))
            return
        }
        log("iface escolhida: \(iface.nome) -> \(iface.ip)")

        do {
            try FileManager.default.createDirectory(at: pastaDados, withIntermediateDirectories: true)
        } catch {
            publicar(EstadoHost(tipo: .erro, mensagem: "Não consegui criar a pasta de dados: \(error.localizedDescription)", rodando: false))
            return
        }
        publicar(EstadoHost(tipo: .iniciando, mensagem: "Iniciando o servidor…", rodando: false))
        // -data: a pasta inteira é o acervo de eleições (ADR-0012) — o gerenciador
        // em /board/eleicoes troca a ativa a quente. Mesmos argumentos do Android.
        let args = ["-addr", ":\(porta)", "-host", iface.ip, "-data", pastaDados.path]
        _ = motor.iniciar(argumentos: args)

        let url = Rede.url(ip: iface.ip, porta: porta)
        if await esperarSaudavel() {
            log("servidor saudável em \(url)")
            let msg = modo == .acessoPessoal
                ? "Servidor no ar. Aponte a câmera para os QR codes. Mantenha este app aberto."
                : "Servidor no ar na rede atual. Conecte os aparelhos à MESMA rede e aponte para o QR. Mantenha este app aberto."
            publicar(EstadoHost(tipo: .noAr, mensagem: msg, rodando: true, url: url))
        } else {
            let erro = motor.ultimoErro
            let msg = erro.isEmpty
                ? "O servidor não respondeu na porta \(porta)."
                : "Falha ao iniciar o servidor: \(erro)"
            log("ERRO: \(msg)")
            motor.parar()
            publicar(EstadoHost(tipo: .erro, mensagem: msg, rodando: false))
        }
    }

    func parar() async {
        parando = true
        let motor = self.motor
        await Task.detached { motor.parar() }.value
        log("servidor parado pela Mesa")
        publicar(EstadoHost(tipo: .parado, mensagem: "Servidor parado.", rodando: false))
    }

    /// Chamado a cada ~2 s pela tela: atualiza os logs e detecta queda do servidor.
    func tick() {
        logs = motor.logs()
        if estado.rodando, !parando, !motor.rodando {
            let erro = motor.ultimoErro
            log("servidor terminou inesperadamente")
            publicar(EstadoHost(tipo: .erro,
                                mensagem: "O servidor encerrou inesperadamente\(erro.isEmpty ? "" : ": \(erro)").",
                                rodando: false))
        }
    }

    /// Ao voltar para o primeiro plano: o iOS suspende o app em segundo plano e
    /// pode recolher o socket de escuta. Se a porta não responder, reinicia.
    func verificarAoVoltar() async {
        guard estado.rodando, !parando else { return }
        if await sonda.respondendo(porta: porta) { return }
        log("porta sem resposta após voltar do segundo plano — reiniciando o servidor")
        let motor = self.motor
        await Task.detached { motor.parar() }.value
        publicar(EstadoHost(tipo: .parado, mensagem: "", rodando: false))
        await iniciar()
    }

    /// Espera a interface aparecer (até ~5 s), como o Android faz com a do hotspot.
    private func procurarInterface() async -> InterfaceRede? {
        for _ in 0..<10 {
            if let i = Rede.escolher(interfaces(), modo: modo) { return i }
            try? await Task.sleep(for: intervalo)
        }
        return nil
    }

    /// Health check: TCP local até o servidor aceitar (máx ~15 s), abortando
    /// cedo se a goroutine do servidor já morreu com erro.
    private func esperarSaudavel() async -> Bool {
        for _ in 0..<30 {
            if await sonda.respondendo(porta: porta) { return true }
            if !motor.rodando, !motor.ultimoErro.isEmpty { return false }
            try? await Task.sleep(for: intervalo)
        }
        return false
    }
}
