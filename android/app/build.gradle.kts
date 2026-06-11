plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "app.votacao.host"
    compileSdk = 34

    defaultConfig {
        applicationId = "app.votacao.host"
        minSdk = 26 // LocalOnlyHotspot exige API 26+
        targetSdk = 34
        versionCode = (project.findProperty("votacaoVersionCode") as String?)?.toInt() ?: 1
        versionName = (project.findProperty("votacaoVersionName") as String?) ?: "0.0-dev"
        ndk { abiFilters += "arm64-v8a" }
    }

    // O binário Go vai como "lib nativa" e precisa ser EXTRAÍDO para o disco
    // (nativeLibraryDir) para podermos dar exec() nele — W^X do Android só
    // permite executar binários vindos do APK por esse caminho.
    packaging {
        jniLibs { useLegacyPackaging = true }
    }

    val releaseKeystore = System.getenv("ANDROID_KEYSTORE_FILE")
    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                storeFile = file(releaseKeystore)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug { }
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("release")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    // Geração dos QR codes (Wi-Fi e URL) — zxing core puro, sem UI.
    implementation("com.google.zxing:core:3.5.3")
}
