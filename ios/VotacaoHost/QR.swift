import CoreImage.CIFilterBuiltins
import UIKit

/// QR codes do Wi-Fi e da URL (CoreImage — sem dependência externa).
enum QR {
    static func imagem(_ conteudo: String, escala: CGFloat = 12) -> UIImage? {
        let filtro = CIFilter.qrCodeGenerator()
        filtro.message = Data(conteudo.utf8)
        filtro.correctionLevel = "M"
        guard let saida = filtro.outputImage?
            .transformed(by: CGAffineTransform(scaleX: escala, y: escala)),
            let cg = CIContext().createCGImage(saida, from: saida.extent)
        else { return nil }
        return UIImage(cgImage: cg)
    }

    /// Payload padrão de QR de Wi-Fi (formato "WIFI:"), com escapes — igual ao Android.
    static func payloadWifi(nome: String, senha: String) -> String {
        func esc(_ s: String) -> String {
            s.replacingOccurrences(of: #"([\\;,:"'])"#, with: #"\\$1"#, options: .regularExpression)
        }
        return "WIFI:T:WPA;S:\(esc(nome));P:\(esc(senha));;"
    }
}
