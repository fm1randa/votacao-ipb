import SwiftUI
import WebKit

/// A Mesa aberta DENTRO do app. No iOS, trocar para o Safari põe o app em
/// segundo plano e o sistema suspende o servidor embutido; com o navegador
/// aqui dentro, quem opera a Mesa no próprio iPhone mantém o servidor vivo.
struct MesaView: View {
    let url: URL
    @Environment(\.dismiss) private var fechar

    var body: some View {
        NavigationStack {
            Navegador(url: url)
                .ignoresSafeArea(edges: .bottom)
                .navigationTitle("Mesa")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Fechar") { fechar() }
                    }
                }
        }
    }
}

private struct Navegador: UIViewRepresentable {
    let url: URL

    func makeUIView(context: Context) -> WKWebView {
        let web = WKWebView()
        web.allowsBackForwardNavigationGestures = true
        web.load(URLRequest(url: url))
        return web
    }

    func updateUIView(_ web: WKWebView, context: Context) {}
}
