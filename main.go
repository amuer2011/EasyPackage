package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"easypackage/internal/archive"
)

//go:embed web/*
var assets embed.FS

type job struct {
	ID              int           `json:"id"`
	Action          string        `json:"action,omitempty"`
	Status          string        `json:"status"`
	Phase           string        `json:"phase"`
	Error           string        `json:"error,omitempty"`
	Output          string        `json:"output,omitempty"`
	Current         string        `json:"current,omitempty"`
	Done            int64         `json:"done"`
	Plan            *archive.Plan `json:"plan,omitempty"`
	Preview         bool          `json:"preview"`
	OutputDirectory bool          `json:"outputDirectory"`
}
type app struct {
	token, host string
	mu          sync.Mutex
	state       job
	cancel      context.CancelFunc
	stop        chan struct{}
	once        sync.Once
	places      *placesStore
}
type task struct {
	archive.Options
	Action string `json:"action"`
}

func main() {
	switch runtime.GOOS {
	case "windows":
		runWindowsWebViewUI()
	case "darwin", "linux":
		runNativeDesktopUI()
	default:
		fmt.Fprintln(os.Stderr, "EasyPackage 支持 Windows、macOS 和 Linux。")
	}
}

// startEmbeddedService runs the existing local-only API for the in-window
// browser renderer. Its randomized port and per-launch token are never exposed
// outside the loopback interface.
func startEmbeddedService() (*app, *http.Server, net.Listener, string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, nil, nil, "", err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		listener.Close()
		return nil, nil, nil, "", err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		listener.Close()
		return nil, nil, nil, "", err
	}
	a := &app{token: hex.EncodeToString(secret), host: listener.Addr().String(), stop: make(chan struct{}), state: job{Status: "idle"}}
	a.places = newPlacesStore(filepath.Join(configDir, "EasyPackage", "preferences.json"))
	server := &http.Server{Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print(err)
			a.once.Do(func() { close(a.stop) })
		}
	}()
	return a, server, listener, "http://" + a.host + "/#" + a.token, nil
}

func stopEmbeddedService(a *app, server *http.Server) {
	if a != nil {
		a.mu.Lock()
		if a.cancel != nil {
			a.cancel()
		}
		a.mu.Unlock()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			a.mu.Lock()
			running := a.state.Status == "running"
			a.mu.Unlock()
			if !running {
				break
			}
			time.Sleep(40 * time.Millisecond)
		}
	}
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

func runBrowserUI() {
	noBrowser := flag.Bool("no-browser", false, "不自动打开浏览器")
	urlFile := flag.String("url-file", "", "将本次启动地址写入指定文件（用于自动化）")
	flag.Parse()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		log.Fatal(err)
	}
	a := &app{token: hex.EncodeToString(secret), host: listener.Addr().String(), stop: make(chan struct{}), state: job{Status: "idle"}}
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	a.places = newPlacesStore(filepath.Join(configDir, "EasyPackage", "preferences.json"))
	server := &http.Server{Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	url := "http://" + a.host + "/#" + a.token
	if *urlFile != "" {
		if err = os.WriteFile(*urlFile, []byte(url), 0600); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("EasyPackage: " + url)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print(err)
			a.once.Do(func() { close(a.stop) })
		}
	}()
	if !*noBrowser {
		if err := openURL(url); err != nil {
			log.Printf("无法启动系统浏览器，请手动访问上方地址：%v", err)
		}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	select {
	case <-signals:
	case <-a.stop:
	}
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	// Give cancellation time to close archive handles and remove staged files.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		running := a.state.Status == "running"
		a.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func openOutputPath(path string, isDir bool) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if isDir {
			cmd = exec.Command("explorer.exe", path)
		} else {
			cmd = exec.Command("explorer.exe", "/select,"+path)
		}
	case "darwin":
		if isDir {
			cmd = exec.Command("open", path)
		} else {
			cmd = exec.Command("open", "-R", path)
		}
	default:
		if !isDir {
			path = filepath.Dir(path)
		}
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func (a *app) handler() http.Handler {
	files, _ := fs.Sub(assets, "web")
	static := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Host != a.host {
			http.Error(w, "Invalid host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+a.host {
			http.Error(w, "Invalid origin", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-EasyPackage-Token")), []byte(a.token)) != 1 {
				http.Error(w, "请从本次启动地址打开应用", http.StatusUnauthorized)
				return
			}
			a.api(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		static.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(value)
}
func bad(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("请求格式不正确")
	}
	return nil
}
func (a *app) api(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/browse" && r.Method == "GET":
		a.browse(w, r)
	case r.URL.Path == "/api/recent" && r.Method == "POST":
		var data struct {
			Path string `json:"path"`
		}
		if err := decode(w, r, &data); err != nil {
			bad(w, err, 400)
			return
		}
		if err := a.places.Remember(data.Path); err != nil {
			bad(w, err, 400)
			return
		}
		respond(w, map[string]any{"recent": a.places.Recent()})
	case r.URL.Path == "/api/state" && r.Method == "GET":
		a.mu.Lock()
		defer a.mu.Unlock()
		respond(w, a.state)
	case r.URL.Path == "/api/reveal" && r.Method == "POST":
		a.mu.Lock()
		output, isDir, status := a.state.Output, a.state.OutputDirectory, a.state.Status
		a.mu.Unlock()
		if status != "done" || output == "" {
			bad(w, errors.New("没有可打开的保存位置"), 409)
			return
		}
		if err := openOutputPath(output, isDir); err != nil {
			bad(w, fmt.Errorf("无法打开保存位置：%w", err), 500)
			return
		}
		respond(w, map[string]bool{"ok": true})
	case r.URL.Path == "/api/start" && r.Method == "POST":
		var t task
		if err := decode(w, r, &t); err != nil {
			bad(w, err, 400)
			return
		}
		if t.Action != "pack" && t.Action != "preview" && t.Action != "restore" {
			bad(w, errors.New("未知操作"), 400)
			return
		}
		if strings.TrimSpace(t.Source) == "" || strings.TrimSpace(t.Output) == "" {
			bad(w, errors.New("请选择输入和输出位置"), 400)
			return
		}
		if t.MaxBytes < 0 {
			bad(w, errors.New("大小阈值不能为负数"), 400)
			return
		}
		if len(t.Password) > 1024 {
			bad(w, errors.New("密码过长，请使用不超过 1024 字节的密码"), 400)
			return
		}
		if t.Action == "restore" {
			if _, err := os.Lstat(t.Output); err == nil {
				bad(w, errors.New("还原文件夹已存在，请换一个新文件夹名称"), 400)
				return
			} else if !errors.Is(err, os.ErrNotExist) {
				bad(w, err, 400)
				return
			}
		} else if t.Action != "preview" {
			if st, err := os.Lstat(t.Output); err == nil {
				if !st.Mode().IsRegular() {
					bad(w, errors.New("输出位置不是普通文件"), 400)
					return
				}
				if !t.Overwrite {
					bad(w, errors.New("文件已存在，是否覆盖？"), 409)
					return
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				bad(w, err, 400)
				return
			}
		}
		a.mu.Lock()
		if a.state.Status == "running" {
			a.mu.Unlock()
			bad(w, errors.New("请等待当前任务完成"), 423)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		a.state = job{ID: a.state.ID + 1, Action: t.Action, Status: "running", Phase: "正在扫描项目", Preview: t.Action == "preview"}
		id := a.state.ID
		a.mu.Unlock()
		go a.run(ctx, t)
		respond(w, map[string]int{"id": id})
	case r.URL.Path == "/api/cancel" && r.Method == "POST":
		a.mu.Lock()
		if a.cancel != nil {
			a.cancel()
		}
		a.mu.Unlock()
		respond(w, map[string]bool{"ok": true})
	case r.URL.Path == "/api/quit" && r.Method == "POST":
		respond(w, map[string]bool{"ok": true})
		a.once.Do(func() { close(a.stop) })
	default:
		http.NotFound(w, r)
	}
}
func (a *app) run(ctx context.Context, t task) {
	var err error
	if t.Action == "restore" {
		a.mu.Lock()
		a.state.Phase = "正在校验并解压项目"
		a.mu.Unlock()
		err = archive.Extract(ctx, t.Source, t.Output, t.Password, func(done int64, name string) { a.mu.Lock(); a.state.Done = done; a.state.Current = name; a.mu.Unlock() })
	} else {
		var plan archive.Plan
		plan, err = archive.Scan(ctx, t.Options)
		if err == nil {
			view := plan
			if len(view.Items) > 2000 {
				view.Items = view.Items[:2000]
			}
			a.mu.Lock()
			a.state.Plan = &view
			a.state.Phase = "正在压缩文件"
			a.mu.Unlock()
			if t.Action == "pack" {
				last := time.Time{}
				err = archive.Pack(ctx, t.Options, plan, func(done int64, name string) {
					if time.Since(last) < 80*time.Millisecond {
						return
					}
					last = time.Now()
					a.mu.Lock()
					a.state.Done = done
					a.state.Current = name
					a.mu.Unlock()
				})
			}
		}
	}
	if err == nil {
		paths := []string{filepath.Dir(t.Output)}
		if t.Cover != "" {
			paths = append(paths, filepath.Dir(t.Cover))
		}
		if t.Action == "restore" {
			paths = append(paths, filepath.Dir(t.Source))
		} else {
			paths = append(paths, t.Source)
		}
		if e := a.places.Remember(paths...); e != nil {
			log.Printf("无法保存最近目录：%v", e)
		}
	}
	t.Password = ""
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		a.state.Status = "cancelled"
		a.state.Phase = "任务已取消"
	case err != nil:
		a.state.Status = "error"
		a.state.Error = err.Error()
		a.state.Phase = "操作未完成"
	default:
		a.state.Status = "done"
		a.state.Phase = "打包完成"
		a.state.Output = t.Output
		if t.Action == "preview" {
			a.state.Phase = "预览完成"
			a.state.Output = ""
		}
		if t.Action == "restore" {
			a.state.Phase = "项目解压完成"
			a.state.OutputDirectory = true
		}
		if a.state.Plan != nil {
			a.state.Done = a.state.Plan.Bytes
		}
	}
}

type fileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
}

func (a *app) browse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	home, _ := os.UserHomeDir()
	if path == "" {
		path = home
	}
	path, err := filepath.Abs(path)
	if err != nil {
		bad(w, err, 400)
		return
	}
	dir, err := os.Open(path)
	if err != nil {
		bad(w, err, 400)
		return
	}
	defer dir.Close()
	// Large folders remain navigable without allocating an unbounded listing.
	entries, err := dir.ReadDir(10001)
	if err != nil && err != io.EOF {
		bad(w, err, 400)
		return
	}
	truncated := len(entries) > 10000
	if truncated {
		entries = entries[:10000]
	}
	rows := []fileEntry{}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		rows = append(rows, fileEntry{entry.Name(), filepath.Join(path, entry.Name()), info.IsDir(), info.Size()})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Directory != rows[j].Directory {
			return rows[i].Directory
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	roots := []string{}
	if runtime.GOOS == "windows" {
		for drive := 'A'; drive <= 'Z'; drive++ {
			p := fmt.Sprintf("%c:\\", drive)
			if st, e := os.Stat(p); e == nil && st.IsDir() {
				roots = append(roots, p)
			}
		}
	} else {
		roots = append(roots, "/")
	}
	respond(w, map[string]any{"path": path, "parent": filepath.Dir(path), "home": home, "roots": roots, "entries": rows, "truncated": truncated, "shortcuts": commonFolders(), "recent": a.places.Recent()})
}
