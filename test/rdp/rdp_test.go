// Package rdp_test 对 go-freerdp-webconnect 的 WebSocket/RDP 功能进行端到端测试。
//
// 支持平台：macOS / Linux / Windows
//
// 测试用例：
//   - TestRDPConnect    — 通过 WebSocket 建立 RDP 连接，验证后端输出 "Connected."
//   - TestRDPDisconnect — 连接建立后主动断开，验证后端输出断开日志且无 ERROR
//
// 运行方式（在项目根目录）：
//
//	go test ./test/rdp/ -v -timeout 60s
//
// 自定义密码（覆盖默认值 "7f2668"）：
//
//	RDP_PASS=yourpass go test ./test/rdp/ -v -timeout 60s
//
// Windows 下自定义 MSYS64 路径：
//
//	MSYS64_PATH=C:\tools\msys64 go test ./test/rdp/ -v -timeout 60s
package rdp_test

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── 配置常量 ──────────────────────────────────────────────────────────────────

const (
	// 测试用监听端口（与生产端口 54455 隔离，避免冲突）
	testListenPort = "54466"
	// 等待 RDP 连接建立的最长时间
	connectTimeout = 15 * time.Second
	// 主动断开后等待后端日志刷新的时间
	disconnectWait = 3 * time.Second
)

// rdpPass 从环境变量 RDP_PASS 读取密码，未设置时使用默认值。
func rdpPass() string {
	if p := os.Getenv("RDP_PASS"); p != "" {
		return p
	}
	return "7f2668"
}

// projectRoot 向上查找包含 go.mod 的目录作为项目根目录。
func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// binaryPath 返回可执行文件的完整路径。
// binaryName() 由各平台文件实现（darwin/linux 无扩展名，windows 加 .exe）。
func binaryPath() string {
	return filepath.Join(projectRoot(), binaryName())
}

// ── 辅助函数 ──────────────────────────────────────────────────────────────────

// serverLog 持有从服务端进程采集到的日志行，线程安全。
type serverLog struct {
	mu    sync.Mutex
	lines []string
}

func (s *serverLog) add(line string) {
	s.mu.Lock()
	s.lines = append(s.lines, line)
	s.mu.Unlock()
}

func (s *serverLog) snapshot() []string {
	s.mu.Lock()
	cp := make([]string, len(s.lines))
	copy(cp, s.lines)
	s.mu.Unlock()
	return cp
}

func (s *serverLog) contains(sub string) bool {
	for _, l := range s.snapshot() {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func (s *serverLog) hasError() (bool, string) {
	for _, l := range s.snapshot() {
		if strings.Contains(l, "[ERROR]") {
			return true, l
		}
	}
	return false, ""
}

// startServer 启动 go-freerdp-webconnect 进程，返回 *exec.Cmd 和实时收集日志的 *serverLog。
func startServer(t *testing.T) (*exec.Cmd, *serverLog) {
	t.Helper()
	bin := binaryPath()
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("可执行文件不存在: %s\n请先在对应平台执行构建脚本", bin)
	}

	cmd := exec.Command(bin,
		"--pass", rdpPass(),
		"--listen", testListenPort,
	)
	// 追加平台专用环境变量（库路径等），保留父进程已有的环境
	cmd.Env = append(os.Environ(), platformEnv()...)

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		t.Fatalf("启动服务失败: %v", err)
	}
	pw.Close() // 父进程不写，关闭写端

	log := &serverLog{}
	scanner := bufio.NewScanner(pr)
	go func() {
		for scanner.Scan() {
			log.add(scanner.Text())
		}
		pr.Close()
	}()

	return cmd, log
}

// stopServer 终止服务进程并等待退出。
func stopServer(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill() //nolint:errcheck
	}
	cmd.Wait() //nolint:errcheck
}

// waitHTTPReady 轮询 /api/version 直到服务就绪，超时则 Fatal。
func waitHTTPReady(t *testing.T) {
	t.Helper()
	url := fmt.Sprintf("http://localhost:%s/api/version", testListenPort)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("服务在 10 秒内未就绪: %s", url)
}

// wsConnect 完成 WebSocket 握手，返回已连接的 net.Conn。
func wsConnect(t *testing.T, path string) net.Conn {
	t.Helper()
	addr := "localhost:" + testListenPort
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("TCP 连接失败: %v", err)
	}

	keyBytes := make([]byte, 16)
	rand.Read(keyBytes) //nolint:errcheck
	key := base64.StdEncoding.EncodeToString(keyBytes)

	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: localhost:" + testListenPort + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Origin: http://localhost:" + testListenPort + "\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		t.Fatalf("发送升级请求失败: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	conn.SetReadDeadline(time.Time{}) //nolint:errcheck
	if err != nil {
		conn.Close()
		t.Fatalf("读取握手响应失败: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "101 Switching Protocols") {
		conn.Close()
		t.Fatalf("WebSocket 握手失败，响应:\n%s", string(buf[:n]))
	}
	return conn
}

// waitLog 轮询 serverLog 直到出现目标字符串或超时。
func waitLog(t *testing.T, log *serverLog, target string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if log.contains(target) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("超时 %v：后端日志中未出现 %q", timeout, target)
}

// dumpLog 将采集到的日志行输出到 t.Log，便于调试。
func dumpLog(t *testing.T, log *serverLog) {
	t.Helper()
	t.Log("── 后端日志 ──")
	for _, l := range log.snapshot() {
		t.Log(l)
	}
}

// ── 测试用例 ──────────────────────────────────────────────────────────────────

// TestRDPConnect 验证通过 WebSocket 能成功建立 RDP 连接。
// 判定标准：后端输出 "Connected."，且全程无 [ERROR] 日志。
func TestRDPConnect(t *testing.T) {
	cmd, log := startServer(t)
	defer stopServer(cmd)

	waitHTTPReady(t)
	t.Log("服务就绪，发起 WebSocket 连接...")

	conn := wsConnect(t, "/ws?dtsize=800x600")
	defer conn.Close()

	t.Log("WebSocket 握手成功，等待 RDP 连接建立...")
	waitLog(t, log, "Connected.", connectTimeout)

	dumpLog(t, log)

	if ok, line := log.hasError(); ok {
		t.Fatalf("RDP 连接过程中出现 [ERROR] 日志: %s", line)
	}
	t.Log("✅ RDP 连接成功，无 ERROR 日志")
}

// TestRDPDisconnect 验证主动断开 WebSocket 后后端正常处理断开流程。
// 判定标准：后端输出含 "Disconnecting" 的日志，且全程无 [ERROR] 日志。
func TestRDPDisconnect(t *testing.T) {
	cmd, log := startServer(t)
	defer stopServer(cmd)

	waitHTTPReady(t)
	conn := wsConnect(t, "/ws?dtsize=800x600")

	t.Log("等待 RDP 连接建立...")
	waitLog(t, log, "Connected.", connectTimeout)

	t.Log("RDP 已连接，发送 WebSocket 关闭帧（模拟点击断开）...")
	// WebSocket Close 帧：FIN=1, opcode=8, mask=1, payload=状态码 1000（\x03\xe8）
	closeFrame := []byte{0x88, 0x82, 0x00, 0x00, 0x00, 0x00, 0x03, 0xe8}
	conn.Write(closeFrame) //nolint:errcheck
	conn.Close()

	// 等待后端处理断开并刷新日志
	waitLog(t, log, "Disconnecting", disconnectWait+5*time.Second)

	dumpLog(t, log)

	if ok, line := log.hasError(); ok {
		t.Fatalf("断开过程中出现 [ERROR] 日志: %s", line)
	}
	t.Log("✅ RDP 断开成功，无 ERROR 日志")
}
