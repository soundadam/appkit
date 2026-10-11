package appkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// Release 是 GitHub 上的一个正式版（releases/latest：不含草稿与预发布）。
type Release struct {
	// Version 不带 v 前缀。
	Version string
	// Page 是 Release 页面，给人看更新说明。
	Page   string
	Assets []Asset
}

// Asset 是 Release 里的一个产物。
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
	// Digest 是 GitHub 算好的 "sha256:<hex>"。
	Digest string `json:"digest"`
}

// client 的代理取自环境变量（https_proxy 等）。不设整体超时：下载大文件要多久就多久。
var client = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 20 * time.Second,
}}

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "appkit-go")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// Latest 取 GitHub 上最新的正式版。
func Latest(ctx context.Context, app App) (Release, error) {
	resp, err := get(ctx, "https://api.github.com/repos/"+app.Repo+"/releases/latest")
	if err != nil {
		return Release{}, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()
	var api struct {
		TagName string  `json:"tag_name"`
		HTMLURL string  `json:"html_url"`
		Assets  []Asset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&api); err != nil {
		return Release{}, fmt.Errorf("checking for updates: %w", err)
	}
	return Release{Version: strings.TrimPrefix(api.TagName, "v"), Page: api.HTMLURL, Assets: api.Assets}, nil
}

// AssetForThisPlatform 是本机平台该下载的那个产物；bundle 表示要的是 macOS 的 .app。
func (r Release) AssetForThisPlatform(app App, bundle bool) (Asset, error) {
	name, ok := assetName(app.Name, r.Version, runtime.GOOS, runtime.GOARCH, bundle)
	if !ok {
		return Asset{}, fmt.Errorf("updating: no release builds for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("updating: %s is not in the release", name)
}

// assetName 是产物文件名 "<name>-<version>-<platform>.<ext>"，见 README 的「发布产物」。
// 平台与架构用 Rust 的叫法（macos、x86_64、aarch64），两种语言的产物名才一致。
func assetName(name, version, goos, goarch string, bundle bool) (string, bool) {
	arch, ok := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	if !ok {
		return "", false
	}
	var platform string
	switch goos {
	case "darwin":
		platform = "macos-" + arch + ".tar.gz"
		if bundle {
			platform = "macos-universal.zip"
		}
	case "linux":
		platform = "linux-" + arch + ".tar.gz"
	case "windows":
		platform = "windows-" + arch + ".zip"
	default:
		return "", false
	}
	return name + "-" + version + "-" + platform, true
}
