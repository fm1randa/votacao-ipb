// Projeto Gradle standalone — o app Android mora dentro do monorepo Go
// (~/Repositories/votacao-ipb/android), mas não participa do go.mod.
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
    }
}
rootProject.name = "votacao-android"
include(":app")
