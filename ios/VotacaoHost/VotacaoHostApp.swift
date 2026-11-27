import SwiftUI
import UIKit

@main
struct VotacaoHostApp: App {
    @State private var host = ServidorHost()
    @Environment(\.scenePhase) private var fase
    @State private var tarefaFundo: UIBackgroundTaskIdentifier = .invalid

    var body: some Scene {
        WindowGroup {
            ContentView(host: host)
                .preferredColorScheme(.light) // identidade do app web/Android (tema claro)
        }
        .onChange(of: fase) { _, nova in
            switch nova {
            case .background:
                // O iOS suspende apps em segundo plano: pedimos o tempo extra que
                // o sistema concede, mas o servidor para de responder depois dele.
                guard host.estado.rodando else { return }
                tarefaFundo = UIApplication.shared.beginBackgroundTask(withName: "servidor") {
                    UIApplication.shared.endBackgroundTask(tarefaFundo)
                    tarefaFundo = .invalid
                }
            case .active:
                if tarefaFundo != .invalid {
                    UIApplication.shared.endBackgroundTask(tarefaFundo)
                    tarefaFundo = .invalid
                }
                Task { await host.verificarAoVoltar() }
            default:
                break
            }
        }
    }
}
