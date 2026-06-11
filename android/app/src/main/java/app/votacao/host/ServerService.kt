package app.votacao.host

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.PowerManager
import android.util.Log
import java.io.File
import java.net.InetSocketAddress
import java.net.NetworkInterface
import java.net.Socket
import kotlin.concurrent.thread

/**
 * Foreground service do spike: cria o LocalOnlyHotspot, descobre o IP da
 * interface, executa o binário Go (empacotado como libvotacao.so) como
 * processo filho e mantém tudo vivo com wake lock + notificação persistente.
 *
 * As duas incógnitas do spike moram aqui:
 *  1. exec() do binário vindo do APK (só funciona a partir de nativeLibraryDir);
 *  2. LocalOnlyHotspot com SSID/senha geradas pelo sistema.
 */
class ServerService : Service() {

    companion object {
        const val TAG = "votacao-host"
        const val GO_TAG = "votacao-go"
        const val PORT = 8090
        const val ACTION_STATE = "app.votacao.host.STATE"

        // Modos de rede (extra MODE do Intent): criar hotspot ou usar a rede atual.
        const val EXTRA_MODE = "mode"
        const val MODE_HOTSPOT = "hotspot"
        const val MODE_EXISTING = "existing"

        // Estados da bolinha do header: cinza, âmbar, verde, vermelho.
        const val KIND_OFF = "off"
        const val KIND_STARTING = "starting"
        const val KIND_ON = "on"
        const val KIND_ERROR = "error"

        // Último estado conhecido — a Activity lê ao (re)abrir.
        @Volatile
        var lastState: HostState? = null
    }

    data class HostState(
        val kind: String,          // KIND_* (cor da bolinha)
        val status: String,        // mensagem detalhada em pt (linha abaixo do botão)
        val running: Boolean,      // servidor respondendo na porta
        val ssid: String? = null,
        val password: String? = null,
        val url: String? = null,
    )

    private var hotspot: WifiManager.LocalOnlyHotspotReservation? = null
    private var process: Process? = null
    private var wakeLock: PowerManager.WakeLock? = null
    private var stopping = false

    // Logs: logcat + ring buffer da aba "Logs" do app.
    private fun log(line: String) {
        Log.i(TAG, line)
        HostLog.add("app", line)
    }

    private fun logErr(line: String, e: Throwable? = null) {
        Log.e(TAG, line, e)
        HostLog.add("app", "ERRO: $line")
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startInForeground("Iniciando…")

        wakeLock = (getSystemService(POWER_SERVICE) as PowerManager)
            .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "$TAG:server")
            .apply { acquire() }

        if (intent?.getStringExtra(EXTRA_MODE) == MODE_EXISTING) {
            // Modo "usar a rede atual": Wi-Fi da igreja ou o hotspot do sistema
            // (este último permite SSID/senha personalizados nas configurações
            // do Android — ex.: "Congresso_9440"). O app não cria nada.
            publish(HostState(KIND_STARTING, "Procurando a rede atual…", running = false))
            log("modo: usar a rede atual")
            thread { bootServer(ssid = null, pass = null, preferHotspotIface = false) }
        } else {
            publish(HostState(KIND_STARTING, "Criando a rede Wi-Fi…", running = false))
            log("modo: criar hotspot")
            startHotspot()
        }
        return START_NOT_STICKY
    }

    // ------------------------------------------------------------------
    // Incógnita 2: LocalOnlyHotspot
    // ------------------------------------------------------------------

    private fun startHotspot() {
        val wifi = applicationContext.getSystemService(WIFI_SERVICE) as WifiManager
        try {
            wifi.startLocalOnlyHotspot(object : WifiManager.LocalOnlyHotspotCallback() {
                override fun onStarted(reservation: WifiManager.LocalOnlyHotspotReservation) {
                    hotspot = reservation
                    val (ssid, pass) = credentials(reservation)
                    log("hotspot no ar: ssid=$ssid")
                    publish(HostState(KIND_STARTING, "Rede criada. Localizando o IP…", false, ssid, pass))
                    thread { bootServer(ssid, pass, preferHotspotIface = true) }
                }

                override fun onFailed(reason: Int) {
                    val why = when (reason) {
                        ERROR_NO_CHANNEL -> "sem canal Wi-Fi disponível"
                        ERROR_GENERIC -> "erro genérico do Wi-Fi"
                        ERROR_INCOMPATIBLE_MODE -> "modo Wi-Fi incompatível (desligue o tethering)"
                        ERROR_TETHERING_DISALLOWED -> "tethering bloqueado no aparelho"
                        else -> "erro $reason"
                    }
                    logErr("hotspot falhou: $why")
                    publish(HostState(KIND_ERROR, "Falha ao criar a rede: $why. " +
                        "Confira se a Localização está LIGADA e o hotspot comum desligado.", false))
                    stopSelf()
                }

                override fun onStopped() {
                    logErr("hotspot parado pelo sistema")
                    if (!stopping) {
                        publish(HostState(KIND_ERROR, "O sistema encerrou a rede Wi-Fi.", false))
                        stopSelf()
                    }
                }
            }, Handler(Looper.getMainLooper()))
        } catch (e: Exception) { // SecurityException (permissão/Localização) etc.
            logErr("startLocalOnlyHotspot: ${e.message}", e)
            publish(HostState(KIND_ERROR, "Não consegui criar a rede: ${e.message}. " +
                "Conceda as permissões e ligue a Localização.", false))
            stopSelf()
        }
    }

    /** SSID/senha geradas pelo sistema — API 30+ usa SoftApConfiguration. */
    private fun credentials(res: WifiManager.LocalOnlyHotspotReservation): Pair<String?, String?> {
        return if (Build.VERSION.SDK_INT >= 30) {
            val cfg = res.softApConfiguration
            val ssid = if (Build.VERSION.SDK_INT >= 33)
                cfg.wifiSsid?.toString()?.trim('"') else @Suppress("DEPRECATION") cfg.ssid
            ssid to cfg.passphrase
        } else {
            @Suppress("DEPRECATION") val cfg = res.wifiConfiguration
            cfg?.SSID?.trim('"') to cfg?.preSharedKey
        }
    }

    // Devolve o IPv4 da interface onde servir.
    //  - preferHotspotIface=true (modo hotspot): espera a iface do LocalOnlyHotspot
    //    subir (ap*, swlan*, wlan1) — nunca a wlan0.
    //  - false (modo rede atual): se houver hotspot do sistema ligado (ap*, swlan*),
    //    usa ele; senão a rede Wi-Fi em que o celular está (wlan0 ou qualquer
    //    IPv4 privado).
    private fun serverIp(preferHotspotIface: Boolean): String? {
        repeat(20) {
            val candidates = NetworkInterface.getNetworkInterfaces().toList()
                .filter { it.isUp && !it.isLoopback }
                .flatMap { ni -> ni.inetAddresses.toList().map { ni.name to it } }
                .filter { (_, a) -> a.isSiteLocalAddress && a.hostAddress?.contains('.') == true }
            val apIface = candidates.firstOrNull { (name, _) ->
                name.startsWith("ap") || name.startsWith("swlan") || name == "wlan1"
            }
            val pick = if (preferHotspotIface) {
                apIface ?: candidates.firstOrNull { (name, _) -> name != "wlan0" }
            } else {
                apIface ?: candidates.firstOrNull()
            }
            if (pick != null) {
                log("iface escolhida: ${pick.first} -> ${pick.second.hostAddress}")
                return pick.second.hostAddress
            }
            Thread.sleep(500)
        }
        return null
    }

    // ------------------------------------------------------------------
    // Incógnita 1: exec() do binário Go vindo do APK
    // ------------------------------------------------------------------

    private fun bootServer(ssid: String?, pass: String?, preferHotspotIface: Boolean) {
        val ip = serverIp(preferHotspotIface)
        if (ip == null) {
            val msg = if (preferHotspotIface)
                "Rede criada, mas não achei o IP da interface."
            else
                "Sem rede: conecte o celular a um Wi-Fi (ou ligue o hotspot do sistema) e tente de novo."
            logErr(msg)
            publish(HostState(KIND_ERROR, msg, false, ssid, pass))
            return
        }
        val exe = File(applicationInfo.nativeLibraryDir, "libvotacao.so")
        if (!exe.exists()) {
            publish(HostState(KIND_ERROR, "Binário do servidor não está no APK — rode android/build-go.sh antes do build.", false, ssid, pass))
            return
        }
        // -data: a pasta inteira é o acervo de eleições (um .db por eleição,
        // ADR-0012) — o gerenciador em /board/eleicoes troca a ativa a quente.
        try {
            process = ProcessBuilder(
                exe.absolutePath, "-addr", ":$PORT", "-host", ip, "-data", filesDir.absolutePath,
            ).redirectErrorStream(true).start()
        } catch (e: Exception) {
            logErr("exec falhou: ${e.message}", e)
            publish(HostState(KIND_ERROR, "Falha ao executar o servidor: ${e.message}", false, ssid, pass))
            return
        }
        // Logs do Go vão pro logcat com a tag votacao-go. Tudo em runCatching:
        // exceção solta em QUALQUER thread derruba o app no Android — e o
        // destroy() do Parar fecha o stream no meio da leitura (IOException).
        thread {
            runCatching {
                process?.inputStream?.bufferedReader()?.forEachLine {
                    Log.i(GO_TAG, it)
                    HostLog.add("go", it)
                }
            }
            val code = runCatching { process?.waitFor() }.getOrNull()
            log("servidor terminou (exit=$code)")
            if (!stopping) publish(HostState(KIND_ERROR, "O servidor encerrou inesperadamente (exit=$code).", false, ssid, pass))
        }

        val url = "http://$ip:$PORT/"
        if (waitHealthy()) {
            log("servidor saudável em $url")
            val msg = if (ssid != null)
                "Servidor no ar. Aponte a câmera para os QR codes."
            else
                "Servidor no ar na rede atual. Conecte os aparelhos à MESMA rede e aponte para o QR."
            publish(HostState(KIND_ON, msg, true, ssid, pass, url))
            updateNotification("Servindo em $url")
        } else {
            publish(HostState(KIND_ERROR, "O servidor não respondeu na porta $PORT.", false, ssid, pass, url))
        }
    }

    /** Health check: TCP connect local até o servidor aceitar (máx ~15s). */
    private fun waitHealthy(): Boolean {
        repeat(30) {
            try {
                Socket().use { s ->
                    s.connect(InetSocketAddress("127.0.0.1", PORT), 500)
                    return true
                }
            } catch (_: Exception) {
                Thread.sleep(500)
            }
        }
        return false
    }

    // ------------------------------------------------------------------
    // Foreground / ciclo de vida
    // ------------------------------------------------------------------

    private fun startInForeground(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(
            NotificationChannel("server", "Servidor de votação", NotificationManager.IMPORTANCE_LOW)
        )
        val notif = buildNotification(text)
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(1, notif, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE)
        } else {
            startForeground(1, notif)
        }
    }

    private fun buildNotification(text: String): Notification =
        Notification.Builder(this, "server")
            .setContentTitle("Votação no ar")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_sys_upload_done)
            .setOngoing(true)
            .build()

    private fun updateNotification(text: String) =
        getSystemService(NotificationManager::class.java).notify(1, buildNotification(text))

    private fun publish(state: HostState) {
        lastState = state
        sendBroadcast(Intent(ACTION_STATE).setPackage(packageName))
    }

    override fun onDestroy() {
        stopping = true
        runCatching { process?.destroy() }
        runCatching { hotspot?.close() }
        runCatching { wakeLock?.takeIf { it.isHeld }?.release() }
        log("servidor parado pela Mesa")
        publish(HostState(KIND_OFF, "Servidor parado.", false))
        super.onDestroy()
    }
}
