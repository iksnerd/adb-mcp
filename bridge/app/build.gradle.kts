plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// The bridge's version comes from the repo-root VERSION file (bridge/ is its own
// Gradle root, so VERSION is one level up). versionCode = major*10000 +
// minor*100 + patch, so 0.24.6 is 2406. internal/adb/bridge_contract_test.go
// fails if these become literals again.
val bridgeVersionName: String = rootProject.projectDir.resolve("../VERSION").let { file ->
    if (!file.isFile) throw GradleException("bridge: cannot read ${file.canonicalPath}")
    file.readText().trim()
}
val bridgeVersionCode: Int = Regex("""(\d+)\.(\d+)\.(\d+)""").matchEntire(bridgeVersionName)
    ?.destructured
    ?.let { (major, minor, patch) -> major.toInt() * 10000 + minor.toInt() * 100 + patch.toInt() }
    ?: throw GradleException("bridge: VERSION must be MAJOR.MINOR.PATCH, got '$bridgeVersionName'")

android {
    namespace = "com.iksnerd.adbmcp.bridge"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.iksnerd.adbmcp.bridge"
        minSdk = 26
        targetSdk = 36
        versionCode = bridgeVersionCode
        versionName = bridgeVersionName
    }

    signingConfigs {
        create("release") {
            storeFile = file("../release.keystore")
            storePassword = "adbmcpbridge"
            keyAlias = "adbmcpbridge"
            keyPassword = "adbmcpbridge"
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("release")
        }
        debug {
            signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}
