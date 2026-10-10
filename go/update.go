package appkit

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Check 在有比正在运行的更新的正式版时返回它，否则返回 nil。
func Check(ctx context.Context, app App) (*Release, error) {
	if app.Dev() {
		return nil, errors.New("development build: updates are disabled")
	}
	latest, err := Latest(ctx, app)
	if err != nil {
		return nil, err
	}
	if !newer(latest.Version, app.Version) {
		return nil, nil
	}
	return &latest, nil
}

// Apply 把 inst 原地换成 rel，新版在下次启动时生效。只处理 Executable；
// progress(已下载, 总字节) 可以为 nil。
func Apply(ctx context.Context, app App, inst Install, rel Release, progress func(done, total int64)) error {
	switch inst.Kind {
	case Homebrew:
		return errors.New("updating: installed with Homebrew; use brew")
	case Dev:
		return errors.New("updating: development build")
	}
	asset, err := rel.AssetForThisPlatform(app)
	if err != nil {
		return err
	}
	dir := filepath.Dir(inst.Path)
	removeLeftover(inst.Path)

	// 暂存在同一目录：最后一步是同一文件系统内的 rename，也顺便提前查出没有写权限
	staging, err := os.MkdirTemp(dir, "."+app.Name+"-update-")
	if err != nil {
		return fmt.Errorf("writing to %s: %w", dir, err)
	}
	defer os.RemoveAll(staging)

	archive := filepath.Join(staging, asset.Name)
	if err := download(ctx, asset, archive, progress); err != nil {
		return err
	}
	unpacked := filepath.Join(staging, "unpacked")
	if err := extract(archive, unpacked); err != nil {
		return fmt.Errorf("unpacking %s: %w", asset.Name, err)
	}
	want := app.Name
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	newExe, ok := find(unpacked, want)
	if !ok {
		return fmt.Errorf("updating: %s is not in %s", want, asset.Name)
	}
	if err := replace(inst.Path, newExe); err != nil {
		return fmt.Errorf("replacing %s: %w", inst.Path, err)
	}
	return nil
}

func download(ctx context.Context, asset Asset, to string, progress func(done, total int64)) error {
	what := "downloading " + asset.Name
	expected, ok := strings.CutPrefix(asset.Digest, "sha256:")
	if !ok {
		return fmt.Errorf("%s: the release lists no SHA-256 for it", what)
	}
	resp, err := get(ctx, asset.URL)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	defer resp.Body.Close()
	f, err := os.Create(to)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	defer f.Close()
	hash := sha256.New()
	var r io.Reader = resp.Body
	if progress != nil {
		r = &counter{r: r, total: asset.Size, progress: progress}
	}
	if _, err := io.Copy(io.MultiWriter(f, hash), r); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return verify(hash.Sum(nil), expected, what)
}

type counter struct {
	r        io.Reader
	done     int64
	total    int64
	progress func(done, total int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	c.progress(c.done, c.total)
	return n, err
}

func verify(actual []byte, expectedHex, what string) error {
	if got := hex.EncodeToString(actual); !strings.EqualFold(got, expectedHex) {
		return fmt.Errorf("%s: SHA-256 is %s, the release says %s", what, got, expectedHex)
	}
	return nil
}

func extract(archive, dest string) error {
	if strings.HasSuffix(archive, ".tar.gz") {
		return untar(archive, dest)
	}
	return unzip(archive, dest)
}

// inside 把产物里的路径落到 dest 下；带 .. 跑出去的拒绝。
func inside(dest, name string) (string, error) {
	p := filepath.Join(dest, name)
	if p != dest && !strings.HasPrefix(p, dest+string(filepath.Separator)) {
		return "", fmt.Errorf("%s points outside the archive", name)
	}
	return p, nil
}

func untar(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := inside(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(p, 0o755)
		case tar.TypeReg:
			err = writeFile(p, tr, h.FileInfo().Mode().Perm())
		}
		if err != nil {
			return err
		}
	}
}

func unzip(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, zf := range z.File {
		p, err := inside(dest, zf.Name)
		if err != nil {
			return err
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		r, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeFile(p, r, zf.Mode().Perm()|0o755)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// find 找产物里的可执行文件：在顶层，或顶层目录下一层（"<name>-<ver>-<platform>/<name>"）。
func find(root, name string) (string, bool) {
	if p := filepath.Join(root, name); exists(p) {
		return p, true
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if p := filepath.Join(root, e.Name(), name); exists(p) {
			return p, true
		}
	}
	return "", false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// BrewUpgrade 先 brew update 拿到 tap 里的新 Cask，再 brew upgrade --cask。
func BrewUpgrade(ctx context.Context, app App, brew string) error {
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, brew, args...)
		cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("brew %s: %s", strings.Join(args, " "), lines[len(lines)-1])
	}
	if err := run("update", "--quiet"); err != nil {
		return err
	}
	return run("upgrade", "--cask", app.Cask())
}
