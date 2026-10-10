package appkit

import (
	"os"
	"path/filepath"
)

// leftover 是被换下来的旧 exe：运行中的 exe 删不掉，但能改名，等下次更新时再删。
func leftover(exe string) string {
	return filepath.Join(filepath.Dir(exe), "."+filepath.Base(exe)+".old")
}

func removeLeftover(exe string) {
	_ = os.Remove(leftover(exe))
}

func replace(exe, newExe string) error {
	old := leftover(exe)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(newExe, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}
