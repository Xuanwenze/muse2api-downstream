# muse2api Studio

一个双击即用的 **muse2api 下游客户端**：文生图、改图、视频生成、对话，支持账号池选号（自动 / 固定账号 / 轮询）。有 Windows 桌面版和 Android 应用（同一份页面、同一个 Go 后端）。

> 本项目是 [muse2api](https://github.com/czg86389-hub/muse2api) 的下游客户端，不含服务端。
> 你需要先有一个可用的 muse2api 服务（自己部署或他人提供的），再用它连接。

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go) ![License](https://img.shields.io/badge/License-MIT-green)

## 功能

- 🖼️ **文生图**：提示词 + 画幅（1:1 / 16:9 / 9:16 / 4:3 / 3:4），轮询到完成，预览 + 下载
- ✏️ **改图**：上传图片（最多 10 张）+ 修改提示词，图生图
- 🎬 **视频**：文生视频 / 图生视频（上传参考图或粘贴图片 URL），时长 5 / 6 / 8 / 10 秒
- 💬 **对话**（v1.5）：文本对话（`muse-spark`），可附图片，.txt/.md/.csv/.json 文件内容会拼进消息；历史只在内存里，关掉即丢
- 👥 **账号池**：自动列出服务端的全部账号（状态、备注、已用次数、上次使用），异常账号置灰；账号列表默认收起，点一下展开
- 🔀 **三种使用模式**
  - **自动**（默认）：走服务器默认调度；视频优先上次出过视频的账号，失败自动换号重试
  - **固定账号**：每次都首选指定的账号（失败时服务器仍会换号重试一次）
  - **轮询**：按健康账号顺序轮流使用，把生成分散到每个号上
- 🔑 **密钥不出浏览器**：所有上游请求都由本地 Go 后端代理，密钥只保存在 exe 同目录（桌面版）或应用内部存储（Android 版）的 `config.json`
- 📜 **本次会话记录**：每条结果标注实际使用的账号，可直接下载

## 快速开始（Windows）

1. 从 [Releases](../../releases) 下载 `muse2api-studio-vX.Y.exe`（或自己构建，见下）
2. 双击运行，浏览器会自动打开 `http://127.0.0.1:18777/`
   - 首次运行若被 Windows Defender SmartScreen 拦截：点「更多信息」→「仍要运行」
   - 若 18777 被占用，程序会自动换一个空闲端口，黑窗口里会显示实际地址
3. 在设置里填入你的 muse2api 地址和密钥（例如 `http://你的服务器:18610/v1`）
4. 选择图片 / 改图 / 视频页签，输入提示词，生成

> 关闭黑窗口即退出服务。密钥只保存在 exe 同目录的 `config.json`（0600 权限），不会发给任何第三方。

## Android 应用（v1.5）

从 [Releases](../../releases) 下载 `muse2api-studio-vX.Y.apk`（arm64，debug 签名），在手机上安装：

1. 允许「安装未知应用」后打开 APK 安装
2. 首次打开会显示设置页，填入你的 muse2api 地址和密钥（例如 `http://你的服务器:18610/v1`），和桌面版填法一样
3. 图片上传（改图 / 对话）走系统选图器，可多选

Android 版就是把桌面版的 Go 后端（`internal/studio`）用 gomobile 编进应用里，服务只绑手机本机的 127.0.0.1，页面和桌面版是同一份；密钥存在应用内部存储，不出本机。

### 从源码构建 Android 版

需要 Go 1.26+、JDK 17、Android SDK（platform 34、build-tools 34、NDK r25c）与 gomobile：

```bash
./bind.sh            # 生成 android/app/libs/studio.aar（gomobile bind）
cd android && ./build.sh   # Gradle assembleDebug，出 app-debug.apk
```

已知坑（都已写进 `android/build.sh` 与 `bind.sh`，不用手动处理）：Gradle 守护进程只绑 127.0.0.1，但本机 `localhost` 先解析到 `::1`，要强制 IPv4；Gradle 不读环境代理变量，代理参数必须以 `-DsystemProp.*` 传入；gomobile 要求 `go.mod` 里有 `tool golang.org/x/mobile/cmd/gobind`，且 NDK r25c 只支持 API 19–33（bind 时传 `-androidapi 24`）；AAR 依赖必须走 `flatDir` 仓库，`files("libs/studio.aar")` 编译时找不到类。

## 从源码构建

需要 Go 1.26+：

```bash
# Windows exe（在任意系统上交叉编译）
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o muse2api-studio.exe .

# Linux / macOS 直接运行
go build -o muse2api-studio .
./muse2api-studio
```

默认端口 18777（被占用时自动顺延）。设置 `NO_BROWSER=1` 可禁止自动打开浏览器。

> exe 的图标是渐变紫方块 + 白色 ✦（`make_icon.py` 生成，`muse2api-studio.ico`）。角色形象的图标只留在本地，不进这个仓库。

## 账号池与选号（需要服务端补丁）

固定账号和轮询模式依赖服务端支持 `account_id` 参数。上游 muse2api 的公开 API 目前没有这个参数，本项目提供了一个很小的补丁（见 [`server-patch/`](server-patch/)，仅改 `app.py`，15 处，其中 1 处是禁止静默自动升级）：

```bash
# 在 muse2api 服务器上（以 v1.6.10 为例）
cd ~/muse2api
cp app.py app.py.bak-$(date +%Y%m%d)
git apply server-patch/account-select.patch
sudo docker-compose up -d --build   # app.py 是 COPY 进镜像的，改完必须重新构建
```

- 补丁打好后，Studio 启动时会自动探测（读 `/openapi.json`），没打补丁时会提示并回退到自动模式
- 上游升级会覆盖 `app.py`，升级后重新打一次补丁并重启即可；回滚：`git checkout -- app.py`
- 注意：上游有静默自动升级（新版本发布后会自动装上并覆盖补丁），补丁的第 15 处把它默认关掉了，管理页「检查更新」只会报告、不会自动安装；升级请手动操作

## 常见问题

| 现象 | 说明 |
| --- | --- |
| 页面还是旧版 / 看不到新功能 | 旧的黑窗口还在运行，占着 18777 端口。先把所有黑窗口关掉，再运行新版 |
| 图片生成超时失败 | 服务器忙或账号状态差时会出现（`等待生成超时，未出现新的生成结果`），闲时重试通常能成；也可在服务端调大 `MUSE2API_IMAGE_TIMEOUT` |
| 视频很慢 | 视频单任务通常 1–2 分钟，超时预算 600 秒；某个账号页面无响应时会自动换号重试 |
| 某个账号显示异常 | 去 muse2api 管理页点该账号的「测试」，按提示重新导入会话 |
| 完成的视频下载 404 | 服务端媒体文件可能被清理，完成后请及时下载 |

## 风险与声明

- 本工具通过你自己的 Muse 网页会话额度生成内容（muse2api 的逆向封装方式），上游可能限制账号，请自行评估风险
- 仅供学习交流使用；生成内容请遵守当地法律法规与 Muse 的使用条款
- 本项目与 Muse / Meta 无关，也与上游 muse2api 作者无关

## 版本历史

| 版本 | 时间 | 变化 |
| --- | --- | --- |
| v1.5 | 2026-11-12 | 新增对话页；Android 应用（gomobile + WebView 壳）；服务端补丁升到 v1.6.10 并默认禁止静默自动升级；服务端逻辑抽成 `internal/studio` 供桌面与 Android 共用 |
| v1.4 | 2026-11-11 | 账号列表默认收起，点一下展开 |
| v1.3 | 2026-11-05 | 新增账号池面板与自动 / 固定 / 轮询三种模式；历史记录标注使用账号 |
| v1.2 | 2026-11-05 | 顶栏 GitHub / LINUX DO 图标入口 |
| v1.1 | 2026-11-05 | 修复旧版缓存问题（`Cache-Control: no-store`）；页脚版本号 v1.1；端口被占用时提示 |

## 链接

- 上游服务端：[czg86389-hub/muse2api](https://github.com/czg86389-hub/muse2api)
- 社区：[LINUX DO](https://linux.do)

## License

[MIT](LICENSE)
