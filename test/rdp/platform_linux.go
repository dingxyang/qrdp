//go:build linux

package rdp_test

import (
	"os"
	"path/filepath"
)

// binaryName 返回 Linux 下的可执行文件名（无扩展名）。
func binaryName() string { return "go-freerdp-webconnect" }

// platformEnv 返回 Linux 运行时需要追加的环境变量。
// FreeRDP 动态库编译安装在项目 install 目录中。
func platformEnv() []string {
	root := projectRoot()
	installLib := filepath.Join(root, "install", "lib")
	installLib2 := filepath.Join(root, "install", "lib", "x86_64-linux-gnu")
	ldPath := installLib + ":" + installLib2
	if prev := os.Getenv("LD_LIBRARY_PATH"); prev != "" {
		ldPath = ldPath + ":" + prev
	}
	return []string{"LD_LIBRARY_PATH=" + ldPath}
}

// installDir 返回 Linux 下 FreeRDP 库的安装目录（供检查依赖使用）。
func installDir() string {
	return filepath.Join(projectRoot(), "install")
}
