//! egui 控件：顶栏的更新提示，设置里的 Updates 一节。检查与下载都在后台线程，
//! 完成后 `request_repaint`。

use crate::{update, App, AutoCheck, Install, Release, Version};
use egui::{Color32, Context, RichText, Ui};
use std::sync::{Arc, Mutex, MutexGuard};

const ACCENT: Color32 = Color32::from_rgb(90, 170, 255);

#[derive(Clone)]
enum Status {
    Idle,
    Checking,
    UpToDate,
    Available(Release),
    /// 下载并替换中；交给 brew 时没有进度。
    Updating {
        version: Version,
        progress: Option<f32>,
    },
    /// 新版已就位，重启生效。
    Ready(Version),
    Failed(String),
}

pub struct Updater {
    app: App,
    install: Install,
    auto: Arc<Mutex<AutoCheck>>,
    status: Arc<Mutex<Status>>,
    ctx: Context,
}

fn lock<T>(m: &Mutex<T>) -> MutexGuard<'_, T> {
    m.lock().unwrap_or_else(|e| e.into_inner())
}

impl Updater {
    /// 在应用构造时创建；到了该自动检查的时候会立刻在后台查一次。
    pub fn new(app: App, ctx: &Context) -> Self {
        let me = Self {
            install: Install::detect(&app),
            auto: Arc::new(Mutex::new(AutoCheck::load(&app))),
            status: Arc::new(Mutex::new(Status::Idle)),
            ctx: ctx.clone(),
            app,
        };
        if lock(&me.auto).due(&app) {
            me.check();
        }
        me
    }

    fn status(&self) -> Status {
        lock(&self.status).clone()
    }

    fn busy(&self) -> bool {
        matches!(
            self.status(),
            Status::Checking | Status::Updating { .. } | Status::Ready(_)
        )
    }

    fn spawn(&self, job: impl FnOnce(&App, &Install) -> Status + Send + 'static) {
        let (app, install, status, ctx) = (
            self.app,
            self.install.clone(),
            self.status.clone(),
            self.ctx.clone(),
        );
        std::thread::spawn(move || {
            let s = job(&app, &install);
            *lock(&status) = s;
            ctx.request_repaint();
        });
    }

    fn check(&self) {
        *lock(&self.status) = Status::Checking;
        let auto = self.auto.clone();
        self.spawn(move |app, _| match update::check(app) {
            Ok(found) => {
                lock(&auto).record(found.as_ref().map(|r| &r.version));
                found.map_or(Status::UpToDate, Status::Available)
            }
            Err(e) => Status::Failed(e.to_string()),
        });
    }

    fn start_update(&self, release: Release) {
        let version = release.version.clone();
        let brew = matches!(self.install, Install::Homebrew { .. });
        *lock(&self.status) = Status::Updating {
            version: version.clone(),
            progress: (!brew).then_some(0.0),
        };
        let (status, ctx) = (self.status.clone(), self.ctx.clone());
        self.spawn(move |app, install| {
            let result = match install {
                Install::Homebrew { .. } => update::brew_upgrade(app, install),
                _ => update::apply(app, install, &release, |done, total| {
                    if total > 0 {
                        *lock(&status) = Status::Updating {
                            version: version.clone(),
                            progress: Some(done as f32 / total as f32),
                        };
                        ctx.request_repaint();
                    }
                }),
            };
            match result {
                Ok(()) => Status::Ready(version),
                Err(e) => Status::Failed(e.to_string()),
            }
        });
    }

    fn update_hint(&self) -> String {
        match &self.install {
            Install::Homebrew { formula, .. } => {
                let kind = if *formula { "--formula" } else { "--cask" };
                format!("Runs brew upgrade {kind} {}", self.app.cask())
            }
            Install::Dev => "Development build: updates are disabled".into(),
            _ => "Downloads the release from GitHub, checks its SHA-256 and replaces this copy"
                .into(),
        }
    }

    fn update_button(&self, ui: &mut Ui, release: &Release) {
        let text = RichText::new(format!("⬆ Update to {}", release.version)).color(ACCENT);
        let enabled = self.install != Install::Dev;
        if ui
            .add_enabled(enabled, egui::Button::new(text))
            .on_hover_text(self.update_hint())
            .on_disabled_hover_text(self.update_hint())
            .clicked()
        {
            self.start_update(release.clone());
        }
    }

    /// 顶栏右侧：有新版时一个按钮，更新中显示进度，装好后提示重启。平时什么都不画。
    pub fn top_bar(&mut self, ui: &mut Ui) {
        match self.status() {
            Status::Available(r) => self.update_button(ui, &r),
            Status::Updating { version, progress } => match progress {
                Some(p) => {
                    ui.add(
                        egui::ProgressBar::new(p)
                            .desired_width(140.0)
                            .text(format!("Updating to {version}")),
                    );
                }
                None => {
                    ui.label(format!("Updating to {version}…"));
                    ui.spinner();
                }
            },
            Status::Ready(version) => {
                let text = RichText::new(format!("⟳ Restart to use {version}")).color(ACCENT);
                if ui.button(text).clicked() {
                    let e = update::relaunch(&self.install);
                    *lock(&self.status) = Status::Failed(e.to_string());
                }
            }
            _ => {}
        }
    }

    /// 设置面板里的 Updates 一节：自动检查开关、立即检查、当前状态。
    pub fn settings(&mut self, ui: &mut Ui) {
        ui.heading("Updates");
        let mut on = lock(&self.auto).enabled();
        if ui
            .checkbox(&mut on, "Check automatically")
            .on_hover_text(format!(
                "Once a day, asks GitHub for the latest release of {}. Nothing else is sent.",
                self.app.repo
            ))
            .changed()
        {
            lock(&self.auto).set_enabled(on);
        }
        ui.horizontal(|ui| {
            if ui
                .add_enabled(!self.busy(), egui::Button::new("Check now"))
                .clicked()
            {
                self.check();
            }
            match self.status() {
                Status::Idle => {
                    ui.label(RichText::new(format!("v{}", self.app.version)).weak());
                }
                Status::Checking => {
                    ui.spinner();
                }
                Status::UpToDate => {
                    ui.label(RichText::new("Up to date").weak());
                }
                Status::Available(r) => {
                    ui.hyperlink_to(format!("{} is out", r.version), &r.page);
                }
                Status::Updating { .. } | Status::Ready(_) | Status::Failed(_) => {}
            }
        });
        match self.status() {
            Status::Available(r) => self.update_button(ui, &r),
            Status::Failed(e) => {
                ui.label(RichText::new(e).small().color(Color32::LIGHT_RED));
            }
            _ => {}
        }
    }
}
