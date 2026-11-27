import XCTest

/// Ciclo da Mesa na tela: escolher a rede, Iniciar, ver o QR, Parar.
///
/// Com `TEST_RUNNER_VOTACAO_TOUR_DIR=<pasta>` no xcodebuild, vira o roteiro
/// dos screenshots do PR: salva as telas na pasta e, com o servidor no ar,
/// espera o arquivo `<pasta>/fluxo-ok` (o fluxo de votação roda de fora, pela
/// rede, como os celulares dos votantes) antes de abrir a Mesa embutida.
final class VotacaoHostUITests: XCTestCase {
    private var app: XCUIApplication!
    private let tour = ProcessInfo.processInfo.environment["VOTACAO_TOUR_DIR"]

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launch()
    }

    private func foto(_ nome: String) {
        guard let tour else { return }
        let png = XCUIScreen.main.screenshot().pngRepresentation
        try? png.write(to: URL(fileURLWithPath: tour).appendingPathComponent("\(nome).png"))
    }

    private func status(_ texto: String, timeout: TimeInterval = 20) -> Bool {
        let e = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label == %@", texto),
                                          object: app.staticTexts["status"])
        return XCTWaiter.wait(for: [e], timeout: timeout) == .completed
    }

    func testIniciaNaRedeAtualMostraQREPara() throws {
        XCTAssertTrue(status("Parado", timeout: 5))
        app.buttons["Usar a rede atual (Wi-Fi do local)"].tap()
        foto("01-parado")

        app.buttons["Iniciar votação"].tap()
        XCTAssertTrue(status("No ar"), "servidor não subiu")
        XCTAssertTrue(app.images["Abra o sistema"].exists, "QR da URL não apareceu")
        XCTAssertTrue(app.buttons["Abrir a Mesa neste iPhone"].exists)
        foto("02-no-ar")

        if let tour {
            let ok = URL(fileURLWithPath: tour).appendingPathComponent("fluxo-ok").path
            let fim = Date().addingTimeInterval(300)
            while !FileManager.default.fileExists(atPath: ok), Date() < fim { sleep(1) }

            app.buttons["Logs"].tap()
            sleep(3) // o ticker de 2 s traz os logs do Go
            foto("03-logs")
            app.buttons["QR Codes"].tap()

            app.buttons["Abrir a Mesa neste iPhone"].tap()
            let pin = app.webViews.secureTextFields.firstMatch
            if pin.waitForExistence(timeout: 10) {
                pin.tap()
                pin.typeText("4729\n")
            }
            sleep(4)
            foto("04-mesa")
            app.buttons["Fechar"].tap()
        }

        app.buttons["Parar"].tap()
        XCTAssertTrue(status("Parado"))
    }
}
