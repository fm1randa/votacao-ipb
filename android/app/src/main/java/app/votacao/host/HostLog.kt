package app.votacao.host

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Ring buffer dos logs visíveis na aba "Logs": linhas do servidor Go + eventos
 * do app (hotspot, IP, erros). ~500 linhas em memória; zera ao fechar o app.
 */
object HostLog {
    private const val MAX = 500
    private val lines = ArrayDeque<String>()
    private val fmt = SimpleDateFormat("HH:mm:ss", Locale.ROOT)

    @Synchronized
    fun add(origin: String, line: String) {
        if (lines.size >= MAX) lines.removeFirst()
        lines.addLast("${fmt.format(Date())} [$origin] $line")
    }

    @Synchronized
    fun text(): String = lines.joinToString("\n")
}
