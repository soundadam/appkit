use crate::install::Install;
use crate::release::{agent, Asset, Release};
use crate::{App, Error, Result, Version};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::fs::{self, File};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{SystemTime, UNIX_EPOCH};

/// 有比正在运行的更新的正式版就返回它。
pub fn check(app: &App) -> Result<Option<Release>> {
    let latest = Release::latest(app)?;
    Ok((latest.version > app.version()).then_some(latest))
}

/// 自动检查的开关与节奏，存在 `<系统配置目录>/<name>/update.json`。
/// 存不下（目录不可写）就只在本次运行内生效，不影响程序本身。
pub struct AutoCheck {
    path: Option<PathBuf>,
    state: State,
}

/// 两次自动检查的最短间隔。GitHub 未登录 API 每个 IP 每小时 60 次，一天一次远低于它。
const INTERVAL_SECS: u64 = 24 * 60 * 60;

#[derive(Serialize, Deserialize)]
#[serde(default)]
struct State {
    enabled: bool,
    /// 上次检查的 Unix 时间（秒）。
    last_check: u64,
    /// 上次查到、还没装上的版本；在它装上之前每次启动都查，好把提示一直挂着。
    pending: Option<Version>,
}

impl Default for State {
    fn default() -> Self {
        Self {
            enabled: true,
            last_check: 0,
            pending: None,
        }
    }
}

fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_secs())
}

impl AutoCheck {
    pub fn load(app: &App) -> Self {
        let path = dirs::config_dir().map(|d| d.join(app.name).join("update.json"));
        let state = path
            .as_ref()
            .and_then(|p| fs::read(p).ok())
            .and_then(|b| serde_json::from_slice(&b).ok())
            .unwrap_or_default();
        Self { path, state }
    }

    pub fn enabled(&self) -> bool {
        self.state.enabled
    }

    pub fn set_enabled(&mut self, on: bool) {
        self.state.enabled = on;
        self.save();
    }

    /// 现在该不该自动去查。
    pub fn due(&self, app: &App) -> bool {
        self.state.enabled
            && (now().saturating_sub(self.state.last_check) >= INTERVAL_SECS
                || self.state.pending.as_ref() > Some(&app.version()))
    }

    /// 记下一次检查（自动或手动）的结果。
    pub fn record(&mut self, found: Option<&Version>) {
        self.state.last_check = now();
        self.state.pending = found.cloned();
        self.save();
    }

    fn save(&self) {
        let Some(path) = &self.path else { return };
        if let Some(dir) = path.parent() {
            let _ = fs::create_dir_all(dir);
        }
        if let Ok(json) = serde_json::to_vec_pretty(&self.state) {
            let _ = fs::write(path, json);
        }
    }
}

/// 把 `install` 原地换成 `release`。`progress(已下载, 总字节)`。
/// 只处理 [`Install::self_updates`] 的两种；新版在下次启动时生效，见 [`relaunch`]。
pub fn apply(
    app: &App,
    install: &Install,
    release: &Release,
    progress: impl FnMut(u64, u64),
) -> Result<()> {
    let (current, want) = match install {
        Install::Bundle(b) => (b, format!("{}.app", app.display_name)),
        Install::Executable(e) => (e, format!("{}{}", app.name, std::env::consts::EXE_SUFFIX)),
        Install::Homebrew { .. } => {
            return Err(Error::new("updating", "installed with Homebrew; use brew"))
        }
        Install::Dev => return Err(Error::new("updating", "development build")),
    };
    let dir = current.parent().unwrap_or(Path::new("."));
    let asset = release.asset_for_this_platform(app)?;

    // 暂存在同一目录：最后一步是同一文件系统内的 rename，也顺便提前查出没有写权限
    let staging = tempfile::Builder::new()
        .prefix(&format!(".{}-update-", app.name))
        .tempdir_in(dir)
        .map_err(|e| Error::new(format!("writing to {}", dir.display()), e))?;
    let archive = staging.path().join(&asset.name);
    download(asset, &archive, progress)?;
    let unpacked = staging.path().join("unpacked");
    extract(&archive, &unpacked)?;
    let new = find(&unpacked, &want)
        .ok_or_else(|| Error::new("updating", format!("{want} is not in {}", asset.name)))?;

    match install {
        Install::Bundle(bundle) => swap(bundle, &new, &staging.path().join("previous")),
        _ => self_replace::self_replace(&new)
            .map_err(|e| Error::new(format!("replacing {}", current.display()), e)),
    }
}

fn download(asset: &Asset, to: &Path, mut progress: impl FnMut(u64, u64)) -> Result<()> {
    let what = format!("downloading {}", asset.name);
    let expected = asset
        .digest
        .as_deref()
        .and_then(|d| d.strip_prefix("sha256:"))
        .ok_or_else(|| Error::new(&what, "the release lists no SHA-256 for it"))?;
    let mut body = agent()
        .get(&asset.url)
        .call()
        .map_err(|e| Error::new(&what, e))?
        .into_body()
        .into_reader();
    let mut file = File::create(to).map_err(|e| Error::new(&what, e))?;
    let mut hasher = Sha256::new();
    let mut buf = vec![0; 64 * 1024];
    let mut done = 0;
    loop {
        let n = body.read(&mut buf).map_err(|e| Error::new(&what, e))?;
        if n == 0 {
            break;
        }
        hasher.update(&buf[..n]);
        file.write_all(&buf[..n])
            .map_err(|e| Error::new(&what, e))?;
        done += n as u64;
        progress(done, asset.size);
    }
    verify(&hasher.finalize(), expected).map_err(|e| Error::new(&what, e))
}

fn verify(actual: &[u8], expected_hex: &str) -> std::result::Result<(), String> {
    let actual: String = actual.iter().map(|b| format!("{b:02x}")).collect();
    if actual.eq_ignore_ascii_case(expected_hex) {
        Ok(())
    } else {
        Err(format!(
            "SHA-256 is {actual}, the release says {expected_hex}"
        ))
    }
}

fn extract(archive: &Path, dest: &Path) -> Result<()> {
    let what = format!("unpacking {}", archive.display());
    let name = archive.to_string_lossy();
    if name.ends_with(".tar.gz") {
        let file = File::open(archive).map_err(|e| Error::new(&what, e))?;
        tar::Archive::new(flate2::read::GzDecoder::new(file))
            .unpack(dest)
            .map_err(|e| Error::new(&what, e))
    } else if cfg!(target_os = "macos") {
        // ditto 原样还原 bundle 的权限、符号链接与签名所需的扩展属性
        let ok = Command::new("/usr/bin/ditto")
            .args(["-x", "-k"])
            .arg(archive)
            .arg(dest)
            .status()
            .map_err(|e| Error::new(&what, e))?
            .success();
        ok.then_some(())
            .ok_or_else(|| Error::new(&what, "ditto failed"))
    } else {
        let file = File::open(archive).map_err(|e| Error::new(&what, e))?;
        zip::ZipArchive::new(file)
            .and_then(|mut z| z.extract(dest))
            .map_err(|e| Error::new(&what, e))
    }
}

/// 产物里要找的东西在顶层或顶层目录下一层（`<name>-<ver>-<platform>/<name>`）。
fn find(root: &Path, name: &str) -> Option<PathBuf> {
    let direct = root.join(name);
    if direct.exists() {
        return Some(direct);
    }
    fs::read_dir(root)
        .ok()?
        .flatten()
        .map(|e| e.path().join(name))
        .find(|p| p.exists())
}

/// 把目录 `old` 换成 `new`；`old` 原来的内容挪到 `backup`，失败就挪回去。
fn swap(old: &Path, new: &Path, backup: &Path) -> Result<()> {
    let what = format!("replacing {}", old.display());
    fs::rename(old, backup).map_err(|e| Error::new(&what, e))?;
    fs::rename(new, old).map_err(|e| {
        let _ = fs::rename(backup, old);
        Error::new(&what, e)
    })
}

/// Homebrew 装的：先 `brew update` 拿到 tap 里的新 Cask，再 `brew upgrade --cask`。
pub fn brew_upgrade(app: &App, brew: &Path) -> Result<()> {
    let run = |args: &[&str]| -> Result<()> {
        let what = format!("brew {}", args.join(" "));
        let out = Command::new(brew)
            .args(args)
            .env("HOMEBREW_NO_AUTO_UPDATE", "1")
            .output()
            .map_err(|e| Error::new(&what, e))?;
        if out.status.success() {
            return Ok(());
        }
        let stderr = String::from_utf8_lossy(&out.stderr);
        let tail = stderr.lines().rev().find(|l| !l.trim().is_empty());
        Err(Error::new(&what, tail.unwrap_or("failed")))
    };
    run(&["update", "--quiet"])?;
    run(&["upgrade", "--cask", &app.cask()])
}

/// 启动新装好的那一份，然后退出本进程。只在启动失败时返回。
pub fn relaunch(install: &Install) -> Error {
    let target = match install {
        Install::Bundle(p) | Install::Executable(p) => p.clone(),
        Install::Homebrew { launch, .. } => launch.clone(),
        Install::Dev => match std::env::current_exe() {
            Ok(p) => p,
            Err(e) => return Error::new("restarting", e),
        },
    };
    let spawned = if target.extension().is_some_and(|e| e == "app") {
        // -n：旧实例还没退出，也要开一个新的
        Command::new("/usr/bin/open").arg("-n").arg(&target).spawn()
    } else {
        Command::new(&target).spawn()
    };
    match spawned {
        Ok(_) => std::process::exit(0),
        Err(e) => Error::new(format!("starting {}", target.display()), e),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn verify_compares_hex_case_insensitively() {
        let sum = Sha256::digest(b"x");
        let hex: String = sum.iter().map(|b| format!("{b:02X}")).collect();
        assert!(verify(&sum, &hex).is_ok());
        assert!(verify(&Sha256::digest(b"y"), &hex).is_err());
    }

    #[test]
    fn tarball_unpacks_and_finds_the_binary_one_level_down() {
        let tmp = tempfile::tempdir().unwrap();
        let archive = tmp.path().join("demo-1.0.0-linux-x86_64.tar.gz");
        {
            let gz = flate2::write::GzEncoder::new(
                File::create(&archive).unwrap(),
                flate2::Compression::fast(),
            );
            let mut tar = tar::Builder::new(gz);
            let mut header = tar::Header::new_gnu();
            header.set_size(3);
            header.set_mode(0o755);
            header.set_cksum();
            tar.append_data(&mut header, "demo-1.0.0-linux-x86_64/demo", &b"new"[..])
                .unwrap();
            tar.into_inner().unwrap().finish().unwrap();
        }
        let dest = tmp.path().join("out");
        extract(&archive, &dest).unwrap();
        let found = find(&dest, "demo").unwrap();
        assert_eq!(fs::read(found).unwrap(), b"new");
    }

    #[cfg(not(target_os = "macos"))]
    #[test]
    fn zip_unpacks_and_finds_the_exe() {
        let tmp = tempfile::tempdir().unwrap();
        let archive = tmp.path().join("demo-1.0.0-windows-x86_64.zip");
        {
            let mut zip = zip::ZipWriter::new(File::create(&archive).unwrap());
            zip.start_file::<_, ()>(
                "demo-1.0.0-windows-x86_64/demo.exe",
                zip::write::SimpleFileOptions::default(),
            )
            .unwrap();
            zip.write_all(b"new").unwrap();
            zip.finish().unwrap();
        }
        let dest = tmp.path().join("out");
        extract(&archive, &dest).unwrap();
        assert_eq!(fs::read(find(&dest, "demo.exe").unwrap()).unwrap(), b"new");
    }

    #[test]
    fn swap_replaces_a_directory_and_keeps_the_old_one_aside() {
        let tmp = tempfile::tempdir().unwrap();
        let (old, new, backup) = (
            tmp.path().join("Demo.app"),
            tmp.path().join("new/Demo.app"),
            tmp.path().join("previous"),
        );
        fs::create_dir_all(&old).unwrap();
        fs::write(old.join("v"), "1").unwrap();
        fs::create_dir_all(&new).unwrap();
        fs::write(new.join("v"), "2").unwrap();
        swap(&old, &new, &backup).unwrap();
        assert_eq!(fs::read_to_string(old.join("v")).unwrap(), "2");
        assert_eq!(fs::read_to_string(backup.join("v")).unwrap(), "1");
    }

    #[test]
    fn pending_update_keeps_the_check_due() {
        let app = App {
            name: "demo",
            display_name: "Demo",
            version: "1.0.0",
            repo: "owner/demo",
        };
        let mut auto = AutoCheck {
            path: None,
            state: State::default(),
        };
        assert!(auto.due(&app));
        auto.record(None);
        assert!(!auto.due(&app));
        auto.record(Some(&Version::new(1, 1, 0)));
        assert!(auto.due(&app));
        auto.set_enabled(false);
        assert!(!auto.due(&app));
    }
}
