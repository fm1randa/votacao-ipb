import XCTest
@testable import VotacaoHost

/// Integração com o servidor Go DE VERDADE (VotacaoGo.xcframework), rodando
/// dentro do processo do app no simulador: sobe, responde HTTP, para e sobe de
/// novo — o ciclo que o botão Iniciar/Parar faz.
@MainActor
final class MotorGoTests: XCTestCase {
    func testServidorEmbutidoSobeRespondeEPara() async throws {
        let porta = 18090
        let pasta = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        let host = ServidorHost(
            motor: MotorGo(),
            interfaces: { [InterfaceRede(nome: "en0", ip: "127.0.0.1")] },
            pastaDados: pasta, porta: porta,
            defaults: UserDefaults(suiteName: "testes-\(UUID().uuidString)")!)
        host.modo = .redeAtual

        for rodada in 1...2 {
            await host.iniciar()
            XCTAssertEqual(host.estado.tipo, .noAr, "rodada \(rodada): \(host.estado.mensagem)")
            XCTAssertEqual(host.estado.url, "http://127.0.0.1:\(porta)/")

            // Sem eleição configurada, a home convida a Mesa para o wizard.
            let (dados, resp) = try await URLSession.shared.data(from: URL(string: "http://127.0.0.1:\(porta)/")!)
            XCTAssertEqual((resp as? HTTPURLResponse)?.statusCode, 200)
            XCTAssertTrue(String(decoding: dados, as: UTF8.self).contains("Configurar agora"))

            await host.parar()
            XCTAssertEqual(host.estado.tipo, .parado)
            XCTAssertFalse(MotorGo().rodando)
        }

        host.tick()
        XCTAssertTrue(host.logs.contains("[go] votação"), "logs do Go chegam ao buffer do app")
        XCTAssertTrue(host.logs.contains("[app] servidor parado pela Mesa"))
        XCTAssertTrue(FileManager.default.fileExists(atPath: pasta.appendingPathComponent("votacao.db").path),
                      "o acervo de eleições fica na pasta de dados")
    }
}
