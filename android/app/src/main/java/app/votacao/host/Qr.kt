package app.votacao.host

import android.graphics.Bitmap
import android.graphics.Color
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter

/** Renderiza QR codes (zxing core) — usado para o QR do Wi-Fi e o da URL. */
object Qr {
    fun bitmap(content: String, size: Int = 720): Bitmap {
        val matrix = QRCodeWriter().encode(
            content, BarcodeFormat.QR_CODE, size, size,
            mapOf(EncodeHintType.MARGIN to 1),
        )
        val pixels = IntArray(size * size) { i ->
            if (matrix.get(i % size, i / size)) Color.BLACK else Color.WHITE
        }
        return Bitmap.createBitmap(pixels, size, size, Bitmap.Config.RGB_565)
    }

    /** Payload padrão de QR de Wi-Fi (formato "WIFI:"), com escapes. */
    fun wifiPayload(ssid: String, password: String): String {
        fun esc(s: String) = s.replace(Regex("([\\\\;,:\"'])"), "\\\\$1")
        return "WIFI:T:WPA;S:${esc(ssid)};P:${esc(password)};;"
    }
}
