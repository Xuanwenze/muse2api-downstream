#!/usr/bin/env bash
# 编 APK：./build.sh（在 ~/workspace/muse-studio/android 下）
# 依赖：~/workspace/setup-android-toolchain.sh 装好的 JDK 17 + SDK + Gradle 8.7，
# 以及 ../bind.sh 生成的 app/libs/studio.aar。
# 注意两点：
#  1) Gradle 守护进程只绑 127.0.0.1，但本机 localhost 先解析到 ::1，
#     客户端连不上守护进程——GRADLE_OPTS 强制只用 IPv4（守护进程侧见 gradle.properties）。
#  2) Gradle 不读环境代理变量，代理参数在这里从 $https_proxy 实时拼出来传入，
#     凭证不写进任何文件；没设代理时直接裸连。
set -e
cd "$(dirname "$0")"
export JAVA_HOME=/home/hatch/jdk17
export ANDROID_HOME=/home/hatch/Android
export GRADLE_USER_HOME=/home/hatch/.gradle
export GRADLE_OPTS="-Djava.net.preferIPv4Stack=true"

PROXY_ARGS=$(python3 -c "
import os, urllib.parse
u = urllib.parse.urlparse(os.environ.get('https_proxy') or os.environ.get('http_proxy') or '')
args = []
if u.hostname:
    # 用 -DsystemProp.* 而不是裸 -D：裸 JVM 系统属性到不了依赖解析用的 HttpClient；
    # 之前就是因此 CONNECT 在代理上被直接 reset（curl 直连没问题，Gradle 一直 reset）。
    for sch in ('http', 'https'):
        args.append(f'-DsystemProp.{sch}.proxyHost={u.hostname}')
        args.append(f'-DsystemProp.{sch}.proxyPort={u.port or 3128}')
        if u.username: args.append(f'-DsystemProp.{sch}.proxyUser={u.username}')
        if u.password: args.append(f'-DsystemProp.{sch}.proxyPassword={u.password}')
print(' '.join(args))
")
read -ra PARR <<< "$PROXY_ARGS"
exec /home/hatch/gradle-8.7/bin/gradle assembleDebug --no-daemon "${PARR[@]}" "$@"
