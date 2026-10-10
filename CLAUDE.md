# appkit

本地门槛（每个 commit 前）：

```bash
cargo fmt --check && cargo clippy --all-targets --all-features -- -D warnings && cargo test --all-features
```

约定（产物命名、Cask、命令行、更新）写在 README；改了约定，同一个改动里改 README 和 tonescope 这个参考实现。
