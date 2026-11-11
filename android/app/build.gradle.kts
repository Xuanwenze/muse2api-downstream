plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.muse2api.studio"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.muse2api.studio"
        minSdk = 24
        targetSdk = 34
        // versionCode 5 对应 Studio v1.5；以后每版 +1
        versionCode = 5
        versionName = "1.5"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
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

dependencies {
    implementation("androidx.activity:activity-ktx:1.8.2")
    // Go 服务（gomobile bind 生成）：先跑 ../bind.sh 生成 app/libs/studio.aar。
    // 注意：files("libs/studio.aar") 不能用——Gradle 不会解开 AAR 里的 classes.jar，
    // 编译时找不到 com.muse2api.studio.Mobile；必须走 flatDir 仓库。
    implementation(group = "", name = "studio", ext = "aar")
}
