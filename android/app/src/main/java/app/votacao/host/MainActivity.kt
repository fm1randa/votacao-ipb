package app.votacao.host

import android.Manifest
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.content.res.ColorStateList
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.provider.Settings
import android.view.View
import android.view.WindowManager
import android.widget.Button
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.RadioGroup
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat

/**
 * Tela única: header com bolinha de status, seletor de modo, botão liga/desliga
 * e as abas QR Codes (os dois QRs) e Logs (servidor Go + eventos do app).
 */
class MainActivity : AppCompatActivity() {

    private lateinit var statusDot: View
    private lateinit var statusLabel: TextView
    private lateinit var detail: TextView
    private lateinit var toggle: Button
    private lateinit var battery: Button
    private lateinit var modeGroup: RadioGroup
    private lateinit var tabQr: Button
    private lateinit var tabLogs: Button
    private lateinit var qrPane: LinearLayout
    private lateinit var logsPane: LinearLayout
    private lateinit var logsText: TextView
    private lateinit var logsScroll: ScrollView
    private lateinit var wifiCard: LinearLayout
    private lateinit var urlCard: LinearLayout
    private lateinit var wifiQr: ImageView
    private lateinit var urlQr: ImageView
    private lateinit var wifiInfo: TextView
    private lateinit var urlInfo: TextView

    private var running = false
    private val ticker = Handler(Looper.getMainLooper())

    /** Modo escolhido (lembrado entre usos). */
    private fun mode(): String =
        if (modeGroup.checkedRadioButtonId == R.id.modeExisting)
            ServerService.MODE_EXISTING else ServerService.MODE_HOTSPOT

    private val askPermissions =
        registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { grants ->
            if (grants.values.all { it }) startServer()
            else detail.text = getString(R.string.precisa_permissoes)
        }

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) = render()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        // Os QR codes ficam expostos durante o credenciamento — tela sempre acesa.
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        statusDot = findViewById(R.id.statusDot)
        statusLabel = findViewById(R.id.statusLabel)
        detail = findViewById(R.id.detail)
        toggle = findViewById(R.id.toggle)
        battery = findViewById(R.id.battery)
        modeGroup = findViewById(R.id.modeGroup)
        tabQr = findViewById(R.id.tabQr)
        tabLogs = findViewById(R.id.tabLogs)
        qrPane = findViewById(R.id.qrPane)
        logsPane = findViewById(R.id.logsPane)
        logsText = findViewById(R.id.logsText)
        logsScroll = findViewById(R.id.logsScroll)
        wifiCard = findViewById(R.id.wifiCard)
        urlCard = findViewById(R.id.urlCard)
        wifiQr = findViewById(R.id.wifiQr)
        urlQr = findViewById(R.id.urlQr)
        wifiInfo = findViewById(R.id.wifiInfo)
        urlInfo = findViewById(R.id.urlInfo)

        // Lembra o último modo escolhido.
        val prefs = getSharedPreferences("host", MODE_PRIVATE)
        if (prefs.getString("mode", ServerService.MODE_HOTSPOT) == ServerService.MODE_EXISTING) {
            modeGroup.check(R.id.modeExisting)
        }
        modeGroup.setOnCheckedChangeListener { _, _ ->
            prefs.edit().putString("mode", mode()).apply()
        }

        toggle.setOnClickListener {
            if (running) stopService(Intent(this, ServerService::class.java))
            else ensurePermissionsThenStart()
        }
        battery.setOnClickListener { requestBatteryExemption() }
        tabQr.setOnClickListener { selectTab(qr = true) }
        tabLogs.setOnClickListener { selectTab(qr = false) }
        findViewById<Button>(R.id.copyLogs).setOnClickListener {
            val cm = getSystemService(ClipboardManager::class.java)
            cm.setPrimaryClip(ClipData.newPlainText("logs", HostLog.text()))
            Toast.makeText(this, R.string.logs_copiados, Toast.LENGTH_SHORT).show()
        }
        selectTab(qr = true)
        render()
    }

    override fun onStart() {
        super.onStart()
        ContextCompat.registerReceiver(
            this, receiver, IntentFilter(ServerService.ACTION_STATE),
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        ticker.post(object : Runnable {
            override fun run() {
                if (logsPane.visibility == View.VISIBLE) renderLogs()
                ticker.postDelayed(this, 2000)
            }
        })
        render()
    }

    override fun onStop() {
        unregisterReceiver(receiver)
        ticker.removeCallbacksAndMessages(null)
        super.onStop()
    }

    private fun selectTab(qr: Boolean) {
        qrPane.visibility = if (qr) View.VISIBLE else View.GONE
        logsPane.visibility = if (qr) View.GONE else View.VISIBLE
        tabQr.alpha = if (qr) 1f else 0.45f
        tabLogs.alpha = if (qr) 0.45f else 1f
        if (!qr) renderLogs()
    }

    private fun renderLogs() {
        val text = HostLog.text().ifEmpty { getString(R.string.logs_vazios) }
        if (logsText.text.toString() != text) {
            logsText.text = text
            logsScroll.post { logsScroll.fullScroll(View.FOCUS_DOWN) } // auto-scroll
        }
    }

    private fun ensurePermissionsThenStart() {
        val criarRede = mode() == ServerService.MODE_HOTSPOT
        val needed = buildList {
            // Permissões de hotspot só no modo "criar rede"; usar a rede atual
            // não exige localização/NEARBY.
            if (criarRede) {
                if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.NEARBY_WIFI_DEVICES)
                else add(Manifest.permission.ACCESS_FINE_LOCATION)
            }
            if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
        }.filter {
            ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED
        }
        if (needed.isEmpty()) startServer() else askPermissions.launch(needed.toTypedArray())
    }

    private fun startServer() {
        ContextCompat.startForegroundService(
            this,
            Intent(this, ServerService::class.java)
                .putExtra(ServerService.EXTRA_MODE, mode()),
        )
    }

    private fun batteryExempt(): Boolean =
        getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(packageName)

    /** Isenção de otimização de bateria — servidor vivo com a tela apagada. */
    private fun requestBatteryExemption() {
        if (batteryExempt()) return
        startActivity(
            Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
                .setData(Uri.parse("package:$packageName"))
        )
    }

    private fun render() {
        val st = ServerService.lastState
        running = st?.running == true
        toggle.text = getString(if (running) R.string.parar else R.string.iniciar)

        // Bolinha + rótulo curto (cinza/âmbar/verde/vermelho).
        val kind = st?.kind ?: ServerService.KIND_OFF
        val (cor, rotulo) = when (kind) {
            ServerService.KIND_ON -> R.color.ok to R.string.st_no_ar
            ServerService.KIND_STARTING -> R.color.gold to R.string.st_iniciando
            ServerService.KIND_ERROR -> R.color.bad to R.string.st_erro
            else -> R.color.muted to R.string.st_parado
        }
        statusDot.backgroundTintList =
            ColorStateList.valueOf(ContextCompat.getColor(this, cor))
        statusLabel.text = getString(rotulo)

        // Mensagem detalhada só quando há algo a dizer (erro/transição/orientação).
        val msg = st?.status.orEmpty()
        detail.text = msg
        detail.visibility =
            if (msg.isNotEmpty() && kind != ServerService.KIND_OFF) View.VISIBLE else View.GONE

        // Some quando a isenção de bateria já foi concedida.
        battery.visibility = if (batteryExempt()) View.GONE else View.VISIBLE

        // Modo não muda com o servidor no ar (pare antes de trocar).
        for (i in 0 until modeGroup.childCount) modeGroup.getChildAt(i).isEnabled = !running

        val temWifi = st?.ssid != null && st.password != null
        wifiCard.visibility = if (temWifi) View.VISIBLE else View.GONE
        if (temWifi) {
            wifiQr.setImageBitmap(Qr.bitmap(Qr.wifiPayload(st!!.ssid!!, st.password!!)))
            wifiInfo.text = getString(R.string.wifi_info, st.ssid, st.password)
        }
        val temUrl = running && st?.url != null
        urlCard.visibility = if (temUrl) View.VISIBLE else View.GONE
        if (temUrl) {
            urlQr.setImageBitmap(Qr.bitmap(st!!.url!!))
            urlInfo.text = st.url
        }
    }
}
