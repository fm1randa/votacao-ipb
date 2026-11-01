import XCTest
@testable import VotacaoHost

final class RedeTests: XCTestCase {
    private let wifi = InterfaceRede(nome: "en0", ip: "192.168.0.20")
    private let hotspot = InterfaceRede(nome: "bridge100", ip: "172.20.10.1")
    private let celular = InterfaceRede(nome: "pdp_ip0", ip: "10.12.0.4")

    func testAcessoPessoalSoServeNaInterfaceDoHotspot() {
        XCTAssertEqual(Rede.escolher([wifi, hotspot], modo: .acessoPessoal), hotspot)
        XCTAssertNil(Rede.escolher([wifi, celular], modo: .acessoPessoal),
                     "sem Acesso Pessoal ligado não há rede para servir")
    }

    func testRedeAtualPrefereHotspotDepoisWifi() {
        XCTAssertEqual(Rede.escolher([celular, wifi, hotspot], modo: .redeAtual), hotspot)
        XCTAssertEqual(Rede.escolher([celular, wifi], modo: .redeAtual), wifi)
        XCTAssertEqual(Rede.escolher([celular], modo: .redeAtual), celular)
        XCTAssertNil(Rede.escolher([], modo: .redeAtual))
    }

    func testIPv4Privado() {
        for ip in ["10.0.0.1", "172.16.0.1", "172.31.255.1", "192.168.1.9", "172.20.10.1"] {
            XCTAssertTrue(Rede.ehPrivado(ip), ip)
        }
        for ip in ["8.8.8.8", "172.32.0.1", "172.15.0.1", "192.169.0.1", "127.0.0.1", "fe80::1", "lixo"] {
            XCTAssertFalse(Rede.ehPrivado(ip), ip)
        }
    }

    func testURLDoSistema() {
        XCTAssertEqual(Rede.url(ip: "172.20.10.1", porta: 8090), "http://172.20.10.1:8090/")
    }

    func testInterfacesAtivasSoTrazemIPv4Privado() {
        for i in Rede.interfacesAtivas() {
            XCTAssertTrue(Rede.ehPrivado(i.ip), "\(i.nome) \(i.ip)")
            XCTAssertFalse(i.nome.hasPrefix("lo"))
        }
    }
}

final class QRTests: XCTestCase {
    func testPayloadWifiSimples() {
        XCTAssertEqual(QR.payloadWifi(nome: "Congresso_9440", senha: "123456789"),
                       "WIFI:T:WPA;S:Congresso_9440;P:123456789;;")
    }

    func testPayloadWifiEscapaCaracteresEspeciais() {
        XCTAssertEqual(QR.payloadWifi(nome: #"iPhone de "Ana"; 2"#, senha: #"a:b,c\d'e"#),
                       #"WIFI:T:WPA;S:iPhone de \"Ana\"\; 2;P:a\:b\,c\\d\'e;;"#)
    }

    func testGeraImagemQuadrada() throws {
        let img = try XCTUnwrap(QR.imagem("http://172.20.10.1:8090/"))
        XCTAssertEqual(img.size.width, img.size.height)
        XCTAssertGreaterThan(img.size.width, 100)
    }
}
