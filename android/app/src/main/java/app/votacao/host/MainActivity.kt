package app.votacao.host

import android.Manifest
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.view.WindowManager
import android.widget.Button
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat

/**
 * Tela única do spike: botão liga/desliga + os DOIS QR codes
 * (1: entrar na rede Wi-Fi; 2: abrir a URL do sistema).
 */
class MainActivity : AppCompatActivity() {

    private lateinit var status: TextView
    private lateinit var toggle: Button
    private lateinit var wifiCard: LinearLayout
    private lateinit var urlCard: LinearLayout
    private lateinit var wifiQr: ImageView
    private lateinit var urlQr: ImageView
    private lateinit var wifiInfo: TextView
    private lateinit var urlInfo: TextView

    private var running = false

    private val askPermissions =
        registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { grants ->
            if (grants.values.all { it }) startServer()
            else status.text = getString(R.string.precisa_permissoes)
        }

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) = render()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        // Os QR codes ficam expostos durante o credenciamento — tela sempre acesa.
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        status = findViewById(R.id.status)
        toggle = findViewById(R.id.toggle)
        wifiCard = findViewById(R.id.wifiCard)
        urlCard = findViewById(R.id.urlCard)
        wifiQr = findViewById(R.id.wifiQr)
        urlQr = findViewById(R.id.urlQr)
        wifiInfo = findViewById(R.id.wifiInfo)
        urlInfo = findViewById(R.id.urlInfo)

        toggle.setOnClickListener {
            if (running) {
                stopService(Intent(this, ServerService::class.java))
            } else {
                ensurePermissionsThenStart()
            }
        }
        findViewById<Button>(R.id.battery).setOnClickListener { requestBatteryExemption() }
        render()
    }

    override fun onStart() {
        super.onStart()
        ContextCompat.registerReceiver(
            this, receiver, IntentFilter(ServerService.ACTION_STATE),
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        render()
    }

    override fun onStop() {
        unregisterReceiver(receiver)
        super.onStop()
    }

    private fun ensurePermissionsThenStart() {
        val needed = buildList {
            if (Build.VERSION.SDK_INT >= 33) {
                add(Manifest.permission.NEARBY_WIFI_DEVICES)
                add(Manifest.permission.POST_NOTIFICATIONS)
            } else {
                add(Manifest.permission.ACCESS_FINE_LOCATION)
            }
        }.filter {
            ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED
        }
        if (needed.isEmpty()) startServer() else askPermissions.launch(needed.toTypedArray())
    }

    private fun startServer() {
        status.text = getString(R.string.iniciando)
        ContextCompat.startForegroundService(this, Intent(this, ServerService::class.java))
    }

    /** Isenção de otimização de bateria — servidor vivo ≥30min de tela apagada. */
    private fun requestBatteryExemption() {
        val pm = getSystemService(PowerManager::class.java)
        if (pm.isIgnoringBatteryOptimizations(packageName)) {
            status.text = getString(R.string.bateria_ok)
            return
        }
        startActivity(
            Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
                .setData(Uri.parse("package:$packageName"))
        )
    }

    private fun render() {
        val st = ServerService.lastState
        running = st?.running == true
        toggle.text = getString(if (running) R.string.parar else R.string.iniciar)
        status.text = st?.status ?: getString(R.string.parado)

        val temWifi = st?.ssid != null && st.password != null
        wifiCard.visibility = if (temWifi) LinearLayout.VISIBLE else LinearLayout.GONE
        if (temWifi) {
            wifiQr.setImageBitmap(Qr.bitmap(Qr.wifiPayload(st!!.ssid!!, st.password!!)))
            wifiInfo.text = getString(R.string.wifi_info, st.ssid, st.password)
        }
        val temUrl = running && st?.url != null
        urlCard.visibility = if (temUrl) LinearLayout.VISIBLE else LinearLayout.GONE
        if (temUrl) {
            urlQr.setImageBitmap(Qr.bitmap(st!!.url!!))
            urlInfo.text = st.url
        }
    }
}
