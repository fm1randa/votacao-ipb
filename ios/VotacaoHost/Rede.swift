import Foundation
import Network

/// Modos de rede — os mesmos do app Android, adaptados ao que o iOS permite.
enum ModoRede: String, CaseIterable, Identifiable {
    /// Acesso Pessoal do iPhone. Diferente do Android, o iOS não tem API para o
    /// app criar a rede: a Mesa liga o Acesso Pessoal em Ajustes e o app serve
    /// na interface dele (`bridge*`, normalmente 172.20.10.1).
    case acessoPessoal
    /// Wi-Fi em que o iPhone já está (ex.: o da igreja).
    case redeAtual

    var id: String { rawValue }

    var titulo: String {
        switch self {
        case .acessoPessoal: "Acesso Pessoal do iPhone (hotspot)"
        case .redeAtual: "Usar a rede atual (Wi-Fi do local)"
        }
    }
}

/// Uma interface de rede com IPv4 privado.
struct InterfaceRede: Equatable {
    let nome: String
    let ip: String

    /// Interface do Acesso Pessoal (`bridge100` no iPhone).
    var ehAcessoPessoal: Bool { nome.hasPrefix("bridge") }
}

enum Rede {
    /// Escolhe onde servir (espelha o `serverIp` do Android):
    ///  - Acesso Pessoal: só a interface do hotspot — sem ela, não há rede;
    ///  - rede atual: o hotspot se estiver ligado; senão o Wi-Fi (`en0`) ou
    ///    qualquer IPv4 privado.
    static func escolher(_ candidatas: [InterfaceRede], modo: ModoRede) -> InterfaceRede? {
        let hotspot = candidatas.first { $0.ehAcessoPessoal }
        switch modo {
        case .acessoPessoal:
            return hotspot
        case .redeAtual:
            return hotspot ?? candidatas.first { $0.nome == "en0" } ?? candidatas.first
        }
    }

    /// Interfaces ativas, não-loopback, com IPv4 privado (10/8, 172.16/12, 192.168/16).
    static func interfacesAtivas() -> [InterfaceRede] {
        var lista: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&lista) == 0, let primeira = lista else { return [] }
        defer { freeifaddrs(lista) }
        var saida: [InterfaceRede] = []
        for p in sequence(first: primeira, next: { $0.pointee.ifa_next }) {
            let ifa = p.pointee
            let flags = Int32(ifa.ifa_flags)
            guard flags & IFF_UP != 0, flags & IFF_LOOPBACK == 0,
                  let addr = ifa.ifa_addr, addr.pointee.sa_family == UInt8(AF_INET)
            else { continue }
            var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            guard getnameinfo(addr, socklen_t(addr.pointee.sa_len), &host, socklen_t(host.count),
                              nil, 0, NI_NUMERICHOST) == 0 else { continue }
            let ip = String(cString: host)
            if ehPrivado(ip) { saida.append(InterfaceRede(nome: String(cString: ifa.ifa_name), ip: ip)) }
        }
        return saida
    }

    static func ehPrivado(_ ip: String) -> Bool {
        let o = ip.split(separator: ".").compactMap { Int($0) }
        guard o.count == 4 else { return false }
        return o[0] == 10 || (o[0] == 172 && (16...31).contains(o[1])) || (o[0] == 192 && o[1] == 168)
    }

    /// URL que os aparelhos abrem (QR 2).
    static func url(ip: String, porta: Int) -> String { "http://\(ip):\(porta)/" }
}

/// Health check: o servidor aceita conexão TCP em 127.0.0.1:porta?
protocol SondaServidor: Sendable {
    func respondendo(porta: Int) async -> Bool
}

struct SondaTCP: SondaServidor {
    func respondendo(porta: Int) async -> Bool {
        guard let p = NWEndpoint.Port(rawValue: UInt16(porta)) else { return false }
        let conexao = NWConnection(host: "127.0.0.1", port: p, using: .tcp)
        let fila = DispatchQueue(label: "votacao.sonda")
        return await withCheckedContinuation { cont in
            let feito = Feito()
            @Sendable func fim(_ ok: Bool) {
                guard feito.marcar() else { return }
                conexao.cancel()
                cont.resume(returning: ok)
            }
            conexao.stateUpdateHandler = { estado in
                switch estado {
                case .ready: fim(true)
                case .failed, .waiting, .cancelled: fim(false)
                default: break
                }
            }
            conexao.start(queue: fila)
            fila.asyncAfter(deadline: .now() + 0.5) { fim(false) }
        }
    }

    /// Garante um único resume da continuation.
    private final class Feito: @unchecked Sendable {
        private let lock = NSLock()
        private var ja = false
        func marcar() -> Bool {
            lock.lock(); defer { lock.unlock() }
            if ja { return false }
            ja = true
            return true
        }
    }
}
