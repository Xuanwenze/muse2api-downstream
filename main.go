// muse2api Studio 桌面版入口：起本地服务并自动打开浏览器界面。
// 服务本体在 internal/studio（与 Android 版共用）。
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"muse-studio/internal/studio"
)

func openBrowser(target string) {
	if os.Getenv("NO_BROWSER") != "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}

func main() {
	port, ln, err := studio.Listen("")
	if err != nil {
		log.Fatal(err)
	}
	target := fmt.Sprintf("http://127.0.0.1:%d/", port)
	log.Printf("关闭本窗口即退出服务。密钥保存在 exe 同目录的 config.json。")
	openBrowser(target)
	if err := studio.Serve(ln); err != nil {
		log.Fatalf("服务异常退出：%v", err)
	}
}
