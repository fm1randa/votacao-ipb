import Foundation
import VotacaoGo

/// O servidor Go embutido, visto pelo app. Protocolo para os testes trocarem o
/// motor real por um dublê.
protocol MotorServidor: AnyObject, Sendable {
    /// Sobe o servidor com os argumentos de linha de comando do binário único
    /// (`-addr`, `-host`, `-data`…). Falso se já estava rodando.
    func iniciar(argumentos: [String]) -> Bool
    /// Pede o encerramento e espera o servidor terminar (até ~6 s).
    func parar()
    var rodando: Bool { get }
    /// Último erro do servidor ("" se nenhum).
    var ultimoErro: String { get }
    /// Ring buffer de logs (servidor Go + eventos do app).
    func logs() -> String
    /// Registra um evento do app no mesmo buffer dos logs do Go.
    func registrar(_ linha: String)
}

/// Motor real: a ponte C de `ios/gobridge` (VotacaoGo.xcframework). O MESMO
/// bootstrap do binário de linha de comando roda numa goroutine deste processo
/// — o iOS não deixa dar exec() num processo filho como no Android (ADR-0015).
final class MotorGo: MotorServidor {
    func iniciar(argumentos: [String]) -> Bool {
        argumentos.joined(separator: "\n").withCString { VotacaoIniciar(UnsafeMutablePointer(mutating: $0)) } == 0
    }

    func parar() { VotacaoParar() }

    var rodando: Bool { VotacaoRodando() == 1 }

    var ultimoErro: String { Self.texto(VotacaoErro()) }

    func logs() -> String { Self.texto(VotacaoLogs()) }

    func registrar(_ linha: String) {
        linha.withCString { VotacaoLog(UnsafeMutablePointer(mutating: $0)) }
    }

    /// Copia e libera uma string alocada pela ponte.
    private static func texto(_ p: UnsafeMutablePointer<CChar>?) -> String {
        guard let p else { return "" }
        defer { VotacaoLiberar(p) }
        return String(cString: p)
    }
}
