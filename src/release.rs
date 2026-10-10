use crate::{App, Error, Result, Version};
use serde::Deserialize;
use std::time::Duration;

/// GitHub 上的一个正式版（`releases/latest`：不含草稿与预发布）。
#[derive(Clone, Debug)]
pub struct Release {
    pub version: Version,
    /// Release 页面，给人看更新说明。
    pub page: String,
    pub assets: Vec<Asset>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct Asset {
    pub name: String,
    #[serde(rename = "browser_download_url")]
    pub url: String,
    pub size: u64,
    /// GitHub 算好的 `sha256:<hex>`。
    pub digest: Option<String>,
}

#[derive(Deserialize)]
struct ApiRelease {
    tag_name: String,
    html_url: String,
    assets: Vec<Asset>,
}

pub(crate) fn agent() -> ureq::Agent {
    // 代理取自环境变量（https_proxy 等）
    ureq::Agent::config_builder()
        .timeout_connect(Some(Duration::from_secs(10)))
        .timeout_recv_response(Some(Duration::from_secs(20)))
        .user_agent(concat!("appkit/", env!("CARGO_PKG_VERSION")))
        .build()
        .new_agent()
}

impl Release {
    pub fn latest(app: &App) -> Result<Self> {
        let url = format!("https://api.github.com/repos/{}/releases/latest", app.repo);
        let what = "checking for updates";
        let api: ApiRelease = agent()
            .get(&url)
            .header("Accept", "application/vnd.github+json")
            .call()
            .map_err(|e| Error::new(what, e))?
            .body_mut()
            .read_json()
            .map_err(|e| Error::new(what, e))?;
        let version = Version::parse(api.tag_name.trim_start_matches('v'))
            .map_err(|e| Error::new(format!("release tag {}", api.tag_name), e))?;
        Ok(Self {
            version,
            page: api.html_url,
            assets: api.assets,
        })
    }

    /// 本机平台该下载的那个产物。
    pub fn asset_for_this_platform(&self, app: &App) -> Result<&Asset> {
        let name = asset_name(
            app.name,
            &self.version,
            std::env::consts::OS,
            std::env::consts::ARCH,
        )
        .ok_or_else(|| Error::new("updating", "no release builds for this platform"))?;
        self.assets
            .iter()
            .find(|a| a.name == name)
            .ok_or_else(|| Error::new("updating", format!("{name} is not in the release")))
    }
}

/// 产物文件名：`<name>-<version>-<platform>.<ext>`，与各仓的 `scripts/package.sh` 一致。
pub(crate) fn asset_name(name: &str, version: &Version, os: &str, arch: &str) -> Option<String> {
    let platform = match os {
        "macos" => "macos-universal.zip".to_string(),
        "linux" => format!("linux-{arch}.tar.gz"),
        "windows" => format!("windows-{arch}.zip"),
        _ => return None,
    };
    Some(format!("{name}-{version}-{platform}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn asset_names_follow_the_package_script() {
        let v = Version::new(1, 2, 3);
        assert_eq!(
            asset_name("demo", &v, "linux", "aarch64").as_deref(),
            Some("demo-1.2.3-linux-aarch64.tar.gz")
        );
        // macOS 只发一份通用二进制，与 arch 无关
        assert_eq!(
            asset_name("demo", &v, "macos", "aarch64"),
            asset_name("demo", &v, "macos", "x86_64")
        );
        assert_eq!(asset_name("demo", &v, "freebsd", "x86_64"), None);
    }
}
