//go:build !windows

package appkit

import "os"

// replace 用 rename 把 newExe 换到 exe 的位置；运行中的进程继续用旧的 inode，不受影响。
func replace(exe, newExe string) error {
	if err := os.Chmod(newExe, 0o755); err != nil {
		return err
	}
	return os.Rename(newExe, exe)
}

func removeLeftover(string) {}
