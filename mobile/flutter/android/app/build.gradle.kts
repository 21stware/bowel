import java.util.Properties

plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

// android/key.properties (storeFile, storePassword, keyAlias, keyPassword) holds
// the release key. The release workflow writes it; it is never committed.
val releaseKey = rootProject.file("key.properties").takeIf { it.exists() }?.let { file ->
    Properties().apply { file.inputStream().use { load(it) } }
}

android {
    namespace = "com.oboo.bowel"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "com.oboo.bowel"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        // Process.destroyForcibly and isAlive need 26.
        minSdk = 26
        targetSdk = flutter.targetSdkVersion
        // Uses the version code from pubspec.yaml. When using split APKs, 1000 * ABI_VERSION
        // is added automatically by Flutter. (https://developer.android.com/studio/build/configure-apk-splits#configure-APK-versions)
        // You can force using the value of versionCode by specifying the `-P force-version-code-ignoring-abi=true`
        // flag during build.
        versionCode = flutter.versionCode
        versionName = flutter.versionName
        // mihomo and the bridge are only built for arm64 phones.
        ndk { abiFilters += "arm64-v8a" }
    }

    packaging {
        // libmihomo.so and liblauncher.so are executables: they have to exist
        // as files in the app's native library folder.
        jniLibs { useLegacyPackaging = true }
    }

    // The rule data mihomo needs in its folder, the same files as the desktop's.
    sourceSets["main"].assets.srcDir("../../../../resources/geo")

    signingConfigs {
        releaseKey?.let { key ->
            create("release") {
                storeFile = rootProject.file(key.getProperty("storeFile"))
                storePassword = key.getProperty("storePassword")
                keyAlias = key.getProperty("keyAlias")
                keyPassword = key.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            // The debug key keeps `flutter run --release` working without the release key.
            signingConfig = signingConfigs.getByName(if (releaseKey != null) "release" else "debug")
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}

dependencies {
    // Built by mobile/android/build.sh. It does not link mihomo.
    implementation(files("libs/bridge.aar"))
    // QR scanning without Google Play services, which many phones here lack.
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
}
