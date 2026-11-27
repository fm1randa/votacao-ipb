import XCTest
@testable import VotacaoHost

/// Dublê do servidor Go: registra chamadas e simula subida, erro e queda.
final class MotorFake: MotorServidor, @unchecked Sendable {
    var argumentos: [String]?
    var rodando = false
    var ultimoErro = ""
    var falhaAoIniciar: String?
    var paradas = 0
    private var linhas: [String] = []

    func iniciar(argumentos: [String]) -> Bool {
        self.argumentos = argumentos
        if let erro = falhaAoIniciar {
            ultimoErro = erro
            return true
        }
        rodando = true
        return true
    }

    func parar() { rodando = false; paradas += 1 }
    func logs() -> String { linhas.joined(separator: "\n") }
    func registrar(_ linha: String) { linhas.append(linha) }
}

/// A porta "responde" enquanto o motor estiver rodando (ou conforme forçado).
struct SondaFake: SondaServidor {
    let motor: MotorFake
    func respondendo(porta: Int) async -> Bool { motor.rodando }
}

@MainActor
final class ServidorHostTests: XCTestCase {
    private var motor: MotorFake!
    private var defaults: UserDefaults!
    private var interfaces: [InterfaceRede] = []

    override func setUp() {
        motor = MotorFake()
        defaults = UserDefaults(suiteName: "testes-\(UUID().uuidString)")
        interfaces = [InterfaceRede(nome: "en0", ip: "192.168.0.20"),
                      InterfaceRede(nome: "bridge100", ip: "172.20.10.1")]
    }

    private func novoHost(sonda: SondaServidor? = nil) -> ServidorHost {
        ServidorHost(motor: motor, sonda: sonda ?? SondaFake(motor: motor),
                     interfaces: { [unowned self] in interfaces },
                     pastaDados: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString),
                     defaults: defaults, intervalo: .milliseconds(1))
    }

    func testIniciaNoAcessoPessoalComOsArgumentosDoBinario() async {
        let host = novoHost()
        host.modo = .acessoPessoal
        await host.iniciar()

        XCTAssertEqual(host.estado.tipo, .noAr)
        XCTAssertTrue(host.estado.rodando)
        XCTAssertEqual(host.estado.url, "http://172.20.10.1:8090/")
        let args = try! XCTUnwrap(motor.argumentos)
        XCTAssertEqual(Array(args.prefix(4)), ["-addr", ":8090", "-host", "172.20.10.1"])
        XCTAssertEqual(args[4], "-data")
    }

    func testRedeAtualServeNoWifiQuandoNaoHaHotspot() async {
        interfaces = [InterfaceRede(nome: "en0", ip: "192.168.0.20")]
        let host = novoHost()
        host.modo = .redeAtual
        await host.iniciar()
        XCTAssertEqual(host.estado.url, "http://192.168.0.20:8090/")
    }

    func testAcessoPessoalDesligadoViraErroSemSubirServidor() async {
        interfaces = [InterfaceRede(nome: "en0", ip: "192.168.0.20")]
        let host = novoHost()
        host.modo = .acessoPessoal
        await host.iniciar()

        XCTAssertEqual(host.estado.tipo, .erro)
        XCTAssertTrue(host.estado.mensagem.contains("Acesso Pessoal"))
        XCTAssertNil(motor.argumentos, "não deve subir o servidor sem rede")
    }

    func testErroDoServidorAparecaNaMensagem() async {
        motor.falhaAoIniciar = "listen: address already in use"
        let host = novoHost()
        await host.iniciar()

        XCTAssertEqual(host.estado.tipo, .erro)
        XCTAssertFalse(host.estado.rodando)
        XCTAssertTrue(host.estado.mensagem.contains("address already in use"))
    }

    func testPararVoltaAoEstadoParado() async {
        let host = novoHost()
        await host.iniciar()
        await host.parar()

        XCTAssertEqual(host.estado.tipo, .parado)
        XCTAssertFalse(host.estado.rodando)
        XCTAssertEqual(motor.paradas, 1)
        XCTAssertTrue(host.logs.contains("servidor parado pela Mesa"))
    }

    func testTickDetectaQuedaInesperada() async {
        let host = novoHost()
        await host.iniciar()
        motor.rodando = false
        motor.ultimoErro = "pânico no servidor"
        host.tick()

        XCTAssertEqual(host.estado.tipo, .erro)
        XCTAssertTrue(host.estado.mensagem.contains("pânico no servidor"))
    }

    func testQRDoWifiSoComNomeESenhaNoAcessoPessoal() async {
        let host = novoHost()
        host.modo = .acessoPessoal
        await host.iniciar()
        XCTAssertNil(host.payloadWifi)

        host.wifiNome = "Congresso"
        host.wifiSenha = "123456789"
        XCTAssertEqual(host.payloadWifi, "WIFI:T:WPA;S:Congresso;P:123456789;;")

        await host.parar()
        XCTAssertNil(host.payloadWifi, "sem servidor no ar, sem QR")
    }

    func testLembraModoENomeDaRede() {
        let host = novoHost()
        host.modo = .redeAtual
        host.wifiNome = "Congresso"
        let outro = novoHost()
        XCTAssertEqual(outro.modo, .redeAtual)
        XCTAssertEqual(outro.wifiNome, "Congresso")
    }

    func testVoltarDoSegundoPlanoNaoMexeSeAPortaResponde() async {
        let host = novoHost()
        await host.iniciar()
        await host.verificarAoVoltar()
        XCTAssertEqual(motor.paradas, 0)
        XCTAssertEqual(host.estado.tipo, .noAr)
    }

    func testVoltarDoSegundoPlanoReiniciaSeAPortaMorreu() async {
        let sonda = SondaControlada(motor: motor)
        let host = novoHost(sonda: sonda)
        await host.iniciar()
        // O iOS recolheu o socket de escuta durante a suspensão: a porta não
        // responde uma vez; o host derruba e sobe o servidor de novo.
        sonda.falhasRestantes = 1
        await host.verificarAoVoltar()

        XCTAssertEqual(motor.paradas, 1)
        XCTAssertEqual(host.estado.tipo, .noAr)
        XCTAssertTrue(host.logs.contains("reiniciando"))
    }
}

/// Sonda que falha um número controlado de vezes antes de seguir o motor.
final class SondaControlada: SondaServidor, @unchecked Sendable {
    let motor: MotorFake
    var falhasRestantes = 0
    init(motor: MotorFake) { self.motor = motor }
    func respondendo(porta: Int) async -> Bool {
        if falhasRestantes > 0 { falhasRestantes -= 1; return false }
        return motor.rodando
    }
}
