// muse2api Studio 的核心服务：桌面版（main.go）和 Android 版（mobile/）共用。
// 所有对 muse2api 的请求都由本程序在后台发起，密钥只保存在 config.json，
// 不会出现在浏览器地址栏或发给任何第三方。
// 数据目录：桌面版用 exe 所在目录；Android 版由调用方传入应用的内部存储目录。
package studio

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed ui.html
var uiHTML []byte

// 版本号显示在页脚，方便确认手上是第几版。
const Version = "v1.5"

var renderedHTML []byte

type Config struct {
	Base      string `json:"base"`
	Key       string `json:"key"`
	Mode      string `json:"mode"`       // auto | fixed | roundrobin
	AccountID string `json:"account_id"` // 固定模式下使用的账号 id
}

// 默认不带任何服务器地址：分享给别人用时，对方第一次打开需自己在设置里填 API 地址和密钥。
const defaultBase = ""

var httpClient = &http.Client{Timeout: 90 * time.Second}
var mediaClient = &http.Client{Timeout: 15 * time.Minute}
var chatClient = &http.Client{Timeout: 330 * time.Second} // 对话走浏览器自动化，服务器端超时 300 秒

// dataDir 由 Listen 设定：非空时 config.json 存到该目录；
// 为空时（桌面版）存到 exe 所在目录（Android 上 os.Executable 不可用，必须传目录进来）。
var dataDir string

func configPath() string {
	if dataDir != "" {
		return filepath.Join(dataDir, "config.json")
	}
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func loadConfig() Config {
	cfg := Config{Base: defaultBase, Mode: "auto"}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.Mode != "fixed" && cfg.Mode != "roundrobin" {
		cfg.Mode = "auto"
	}
	return cfg
}

func saveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

// ---- 账号池（选号）支持 ----

type Account struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	OK       bool   `json:"ok"`
	Enabled  bool   `json:"enabled"`
	Note     string `json:"note"`
	UseCount int    `json:"use_count"`
	LastUsed any    `json:"last_used"`
}

var (
	accountsCache     []Account
	accountsCacheAt   time.Time
	accountsCacheLock sync.Mutex
	rrIndex           int
	rrLock            sync.Mutex
	supportedCache    *bool
	supportedCacheAt  time.Time
)

// serverSupportsAccountSelect：检查服务器是否打了选号补丁。
// 依据是 FastAPI 的 /openapi.json 里 ImageRequest 是否有 account_id 字段（免费探测，不消耗生成额度）。
func serverSupportsAccountSelect(cfg Config) bool {
	if supportedCache != nil && time.Since(supportedCacheAt) < 10*time.Minute {
		return *supportedCache
	}
	supported := false
	origin := strings.TrimSuffix(normalizeBase(cfg.Base), "/v1")
	req, err := http.NewRequest(http.MethodGet, origin+"/openapi.json", nil)
	if err == nil {
		resp, err := httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var spec struct {
				Components struct {
					Schemas map[string]struct {
						Properties map[string]any `json:"properties"`
					} `json:"schemas"`
				} `json:"components"`
			}
			if json.NewDecoder(resp.Body).Decode(&spec) == nil {
				if s, ok := spec.Components.Schemas["ImageRequest"]; ok {
					_, supported = s.Properties["account_id"]
				}
			}
		}
	}
	supportedCache = &supported
	supportedCacheAt = time.Now()
	return supported
}

// fetchAccounts：从服务器取账号池列表（60 秒缓存）。
func fetchAccounts(cfg Config) ([]Account, error) {
	accountsCacheLock.Lock()
	defer accountsCacheLock.Unlock()
	if accountsCache != nil && time.Since(accountsCacheAt) < 60*time.Second {
		return accountsCache, nil
	}
	origin := strings.TrimSuffix(normalizeBase(cfg.Base), "/v1")
	req, err := http.NewRequest(http.MethodGet, origin+"/admin/accounts", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("服务器返回 %d", resp.StatusCode)
	}
	var body struct {
		Accounts []Account `json:"accounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	accountsCache = body.Accounts
	accountsCacheAt = time.Now()
	return accountsCache, nil
}

func healthyAccounts(list []Account) []Account {
	var out []Account
	for _, a := range list {
		if a.OK && a.Enabled {
			out = append(out, a)
		}
	}
	return out
}

// resolveAccount：按当前模式决定本次任务用的账号 id（空表示不指定，走服务器默认调度）。
func resolveAccount(cfg Config) string {
	if cfg.Mode == "fixed" {
		return strings.TrimSpace(cfg.AccountID)
	}
	if cfg.Mode != "roundrobin" {
		return ""
	}
	list, err := fetchAccounts(cfg)
	if err != nil {
		return ""
	}
	healthy := healthyAccounts(list)
	if len(healthy) == 0 {
		return ""
	}
	rrLock.Lock()
	defer rrLock.Unlock()
	acc := healthy[rrIndex%len(healthy)]
	rrIndex++
	return acc.ID
}

// normalizeBase：接受带或不带 /v1 的地址，统一成以 /v1 结尾。
func normalizeBase(base string) string {
	base = strings.TrimSpace(base)
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// upstreamJSON：向 muse2api 发 JSON 请求，返回状态码和解析后的 JSON。
func upstreamJSON(method, path string, cfg Config, body any) (int, map[string]any, error) {
	return upstreamJSONWithClient(method, path, cfg, body, httpClient)
}

func upstreamJSONWithClient(method, path string, cfg Config, body any, client *http.Client) (int, map[string]any, error) {
	var buf io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		buf = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, normalizeBase(cfg.Base)+path, buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	var out map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &out)
	}
	if out == nil {
		out = map[string]any{}
	}
	return resp.StatusCode, out, nil
}

// handleChat：对话代理。POST /api/chat，{messages:[{role, content}]}，
// content 可以是字符串或 OpenAI 风格的内容块数组（含 image_url 图片块）。
// 对话不走选号（ChatRequest 本来就没有 account_id 字段），始终用服务器自动调度。
// 非流式：等服务器回完整回答再返回。
func handleChat(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	var in struct {
		Messages    []map[string]any `json:"messages"`
		Temperature *float64         `json:"temperature"`
		MaxTokens   *int             `json:"max_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Messages) == 0 {
		writeErr(w, http.StatusBadRequest, "对话记录为空")
		return
	}
	body := map[string]any{
		"model": "muse-spark", "messages": in.Messages, "stream": false,
	}
	if in.Temperature != nil {
		body["temperature"] = *in.Temperature
	}
	if in.MaxTokens != nil {
		body["max_tokens"] = *in.MaxTokens
	}
	code, out, err := upstreamJSONWithClient(http.MethodPost, "/chat/completions", cfg, body, chatClient)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	if code >= 400 {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("服务器返回 %d：%v", code, firstErr(out)))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func needConfig(w http.ResponseWriter) (Config, bool) {
	cfg := loadConfig()
	if strings.TrimSpace(cfg.Base) == "" {
		writeErr(w, http.StatusBadRequest, "未设置 API 地址：请先点右上角设置，填入你自己的 muse2api 地址和密钥")
		return cfg, false
	}
	if strings.TrimSpace(cfg.Key) == "" {
		writeErr(w, http.StatusBadRequest, "未设置密钥：请先在右上角设置里填入 API 地址和密钥")
		return cfg, false
	}
	return cfg, true
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cfg := loadConfig()
		writeJSON(w, http.StatusOK, map[string]any{
			"base":           cfg.Base,
			"key_configured": strings.TrimSpace(cfg.Key) != "",
			"mode":           cfg.Mode,
			"account_id":     cfg.AccountID,
		})
		return
	}
	if r.Method == http.MethodPost {
		// 用指针区分“没传”和“传空”：没传的字段保持原值，传空的 account_id 表示取消选择
		var in struct {
			Base      *string `json:"base"`
			Key       *string `json:"key"`
			Mode      *string `json:"mode"`
			AccountID *string `json:"account_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "请求格式错误")
			return
		}
		cfg := loadConfig()
		if in.Base != nil && strings.TrimSpace(*in.Base) != "" {
			cfg.Base = strings.TrimSpace(*in.Base)
		}
		if in.Key != nil && strings.TrimSpace(*in.Key) != "" {
			cfg.Key = strings.TrimSpace(*in.Key)
		}
		if in.Mode != nil && (*in.Mode == "auto" || *in.Mode == "fixed" || *in.Mode == "roundrobin") {
			cfg.Mode = *in.Mode
		}
		if in.AccountID != nil {
			cfg.AccountID = strings.TrimSpace(*in.AccountID)
		}
		if err := saveConfig(cfg); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败："+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "base": cfg.Base, "key_configured": true,
			"mode": cfg.Mode, "account_id": cfg.AccountID})
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleAccounts：账号池列表 + 服务器是否支持选号。
func handleAccounts(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	supported := serverSupportsAccountSelect(cfg)
	list, err := fetchAccounts(cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "获取账号池失败："+err.Error())
		return
	}
	if list == nil {
		list = []Account{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":   list,
		"supported":  supported,
		"mode":       cfg.Mode,
		"account_id": cfg.AccountID,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	// /readyz 在根路径，不在 /v1 下面
	origin := strings.TrimSuffix(normalizeBase(cfg.Base), "/v1")
	req, err := http.NewRequest(http.MethodGet, origin+"/readyz", nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := httpClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if out == nil {
		out = map[string]any{}
	}
	out["http_status"] = resp.StatusCode
	writeJSON(w, http.StatusOK, out)
}

func handleImage(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	var in struct {
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "请填写提示词")
		return
	}
	if in.Size == "" {
		in.Size = "1:1"
	}
	body := map[string]any{
		"model": "muse-image", "prompt": in.Prompt, "size": in.Size,
	}
	if acc := resolveAccount(cfg); acc != "" {
		body["account_id"] = acc
	}
	code, out, err := upstreamJSON(http.MethodPost, "/images/tasks", cfg, body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	if code >= 400 {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("服务器返回 %d：%v", code, firstErr(out)))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// absolutizeResultURLs：把任务 JSON 里 result.url / url / data[].url 的相对路径补成绝对地址。
func absolutizeResultURLs(out map[string]any, origin string) {
	fix := func(u any) any {
		s, ok := u.(string)
		if !ok || !strings.HasPrefix(s, "/") {
			return u
		}
		return origin + s
	}
	if r, ok := out["result"].(map[string]any); ok {
		r["url"] = fix(r["url"])
	}
	out["url"] = fix(out["url"])
	if data, ok := out["data"].([]any); ok {
		for _, item := range data {
			if m, ok := item.(map[string]any); ok {
				m["url"] = fix(m["url"])
			}
		}
	}
}

func handleImageEdit(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "解析上传失败："+err.Error())
		return
	}
	prompt := r.FormValue("prompt")
	if strings.TrimSpace(prompt) == "" {
		writeErr(w, http.StatusBadRequest, "请填写提示词")
		return
	}
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, "请上传至少一张图片")
		return
	}
	// 转发 multipart 到 /v1/images/edits（async=true 走异步任务）
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("prompt", prompt)
	if size := r.FormValue("size"); size != "" {
		_ = mw.WriteField("size", size)
	}
	_ = mw.WriteField("async", "true")
	if acc := resolveAccount(cfg); acc != "" {
		_ = mw.WriteField("account_id", acc)
	}
	for _, fh := range files {
		if fh.Size > 40<<20 {
			writeErr(w, http.StatusBadRequest, "单张图片不能超过 40MB："+fh.Filename)
			return
		}
		f, err := fh.Open()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "读取上传失败："+err.Error())
			return
		}
		part, err := mw.CreateFormFile("image", fh.Filename)
		if err != nil {
			f.Close()
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			f.Close()
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		f.Close()
	}
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, normalizeBase(cfg.Base)+"/images/edits", &buf)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := httpClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if out == nil {
		out = map[string]any{}
	}
	if resp.StatusCode >= 400 {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("服务器返回 %d：%v", resp.StatusCode, firstErr(out)))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func handleVideo(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	var in struct {
		Prompt   string   `json:"prompt"`
		Duration int      `json:"duration"`
		Size     string   `json:"size"`
		Refs     []string `json:"refs"` // 图片 URL 或 data URL（图生视频的参考图）
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "请填写提示词")
		return
	}
	if in.Duration == 0 {
		in.Duration = 5
	}
	if in.Size == "" {
		in.Size = "16:9"
	}
	body := map[string]any{"prompt": in.Prompt, "duration": in.Duration, "size": in.Size}
	if acc := resolveAccount(cfg); acc != "" {
		body["account_id"] = acc
	}
	if len(in.Refs) > 0 {
		imgs := make([]map[string]string, 0, len(in.Refs))
		for _, ref := range in.Refs {
			imgs = append(imgs, map[string]string{"url": ref})
		}
		body["images"] = imgs
	}
	code, out, err := upstreamJSON(http.MethodPost, "/videos", cfg, body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	if code >= 400 {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("服务器返回 %d：%v", code, firstErr(out)))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTask：轮询代理。GET /api/task?kind=image|video&id=task_xxx
func handleTask(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "缺少任务 id")
		return
	}
	path := "/videos/" + id
	if kind == "image" {
		path = "/images/tasks/" + id
	}
	code, out, err := upstreamJSON(http.MethodGet, path, cfg, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	if code >= 400 {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("服务器返回 %d：%v", code, firstErr(out)))
		return
	}
	// 上游有时返回相对路径的媒体地址（如 /v1/media/xxx.webp），统一补成绝对地址，方便前端直接用
	absolutizeResultURLs(out, strings.TrimSuffix(normalizeBase(cfg.Base), "/v1"))
	writeJSON(w, http.StatusOK, out)
}

// handleMedia：媒体代理，让浏览器不用带密钥就能预览 / 下载结果文件。
// GET /api/media?url=<完整的上游媒体地址>&download=1
func handleMedia(w http.ResponseWriter, r *http.Request) {
	cfg, ok := needConfig(w)
	if !ok {
		return
	}
	raw := r.URL.Query().Get("url")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "缺少 url")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeErr(w, http.StatusBadRequest, "url 不合法")
		return
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := mediaClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "下载失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("服务器返回 %d", resp.StatusCode))
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if r.URL.Query().Get("download") == "1" {
		name := filepath.Base(u.Path)
		if name == "" || name == "/" {
			name = "muse2api-media"
		}
		w.Header().Set("Content-Disposition", "attachment; filename="+name)
	}
	_, _ = io.Copy(w, resp.Body)
}

func firstErr(out map[string]any) any {
	if v, ok := out["error"]; ok {
		return v
	}
	if v, ok := out["detail"]; ok {
		return v
	}
	if v, ok := out["message"]; ok {
		return v
	}
	return out
}

// buildMux：路由表 + 禁止缓存（换新版后刷新页面即是新版，不会停留在旧页面）。
func buildMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(renderedHTML)
	})
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/accounts", handleAccounts)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/image", handleImage)
	mux.HandleFunc("/api/image-edit", handleImageEdit)
	mux.HandleFunc("/api/video", handleVideo)
	mux.HandleFunc("/api/chat", handleChat)
	mux.HandleFunc("/api/task", handleTask)
	mux.HandleFunc("/api/media", handleMedia)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		mux.ServeHTTP(w, r)
	})
}

// Listen：把版本号填进页面，在 127.0.0.1:18777（被占用则往后找空闲端口）起监听，
// 只返回监听器，不阻塞。调用方拿到 port 后自行决定是在当前 goroutine 跑 Serve，
// 还是另开 goroutine（Android 版在后台跑，主线程回 UI）。
// dir 非空时 config.json 存到该目录；传空表示桌面版（exe 所在目录）。
func Listen(dir string) (int, net.Listener, error) {
	if dir != "" {
		dataDir = dir
	}
	// 把版本号填进页面
	renderedHTML = bytes.Replace(uiHTML, []byte("{{APP_VERSION}}"), []byte(Version), -1)

	var ln net.Listener
	var err error
	port := 18777
	for i := 0; i < 10; i++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+i))
		if err == nil {
			if i > 0 {
				log.Printf("注意：端口 18777 已被占用（可能是旧版本还在运行），本次使用端口 %d。请先关闭旧版本的黑色窗口，再运行新版。", port+i)
			}
			port = port + i
			break
		}
	}
	if ln == nil {
		return 0, nil, fmt.Errorf("无法启动本地服务：%v", err)
	}
	log.Printf("muse2api Studio %s 已启动：http://127.0.0.1:%d/", Version, port)
	return port, ln, nil
}

// Serve：在已绑好的监听器上跑服务，阻塞直到服务退出，返回退出原因。
func Serve(ln net.Listener) error {
	return http.Serve(ln, buildMux())
}
