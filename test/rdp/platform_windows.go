//go:build windows

package rdp_test

import (
	"os"
	"path/filepath"
)

// binaryName 返回 Windows 下的可执行文件名（含 .exe 扩展名）。
func binaryName() string { return "go-freerdp-webconnect.exe" }

// platformEnv 返回 Windows 运行时需要追加的环境变量。
// FreeRDP DLL 和 MinGW 运行时位于 install/bin 目录；
// MSYS64 路径通过环境变量 MSYS64_PATH 指定，默认 C:\DevDisk\DevTools\msys64。
func platformEnv() []string {
	root := projectRoot()
	installBin := filepath.Join(root, "install", "bin")

	msys64 := os.Getenv("MSYS64_PATH")
	if msys64 == "" {
		msys64 = `C:\DevDisk\DevTools\msys64`
	}
	mingwBin := filepath.Join(msys64, "mingw64", "bin")

	newPath := installBin + ";" + mingwBin
	if prev := os.Getenv("PATH"); prev != "" {
		newPath = newPath + ";" + prev
	}
	return []string{"PATH=" + newPath}
}

// installDir 返回 Windows 下 FreeRDP 库的安装目录（供检查依赖使用）。
func installDir() string {
	return filepath.Join(projectRoot(), "install")
}
