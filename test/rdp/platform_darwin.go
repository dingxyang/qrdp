//go:build darwin

package rdp_test

import (
	"os"
	"path/filepath"
)

// binaryName 返回 macOS 下的可执行文件名（无扩展名）。
func binaryName() string { return "go-freerdp-webconnect" }

// platformEnv 返回 macOS 运行时需要追加的环境变量。
// FreeRDP 动态库安装在 Homebrew 的 /usr/local/opt/freerdp/lib 目录。
func platformEnv() []string {
	dyldPath := "/usr/local/opt/freerdp/lib"
	if prev := os.Getenv("DYLD_LIBRARY_PATH"); prev != "" {
		dyldPath = dyldPath + ":" + prev
	}
	return []string{"DYLD_LIBRARY_PATH=" + dyldPath}
}

// installDir 返回 macOS 下 FreeRDP 库的安装目录（供检查依赖使用）。
func installDir() string {
	return filepath.Join(projectRoot(), "install")
}
