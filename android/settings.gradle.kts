pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
        // 本地 AAR（gomobile bind 产物）：flatDir 只能声明在这里（FAIL_ON_PROJECT_REPOS）
        flatDir {
            dirs("app/libs")
        }
    }
}
rootProject.name = "muse2api-studio"
include(":app")
