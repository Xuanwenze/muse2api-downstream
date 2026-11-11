#!/usr/bin/env bash
# 生成 app/libs/studio.aar（gomobile bind）：Go 服务编成 Android 库给壳工程用。
# 用法：在 ~/workspace/muse-studio 下跑 ./bind.sh
# 依赖：~/workspace/setup-android-toolchain.sh 装好的 SDK/NDK + gomobile。
set -e
cd "$(dirname "$0")"
export PATH=/home/hatch/go-sdk/bin:/home/hatch/go/bin:/home/hatch/jdk17/bin:$PATH
export JAVA_HOME=/home/hatch/jdk17
# gomobile 生的 Mobile.java 含 Go 注释文本；强制 javac 用 UTF-8，避免默认 US-ASCII 下中文注释编不过
export JAVA_TOOL_OPTIONS=-Dfile.encoding=UTF-8
export ANDROID_HOME=/home/hatch/Android
export ANDROID_SDK_ROOT=$ANDROID_HOME
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/25.2.9519653
gomobile bind -androidapi 24 -target=android/arm64 -javapkg com.muse2api.studio -o android/app/libs/studio.aar ./mobile
ls -la android/app/libs/studio.aar
