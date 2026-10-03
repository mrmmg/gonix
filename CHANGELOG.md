# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.1] - 2026-10-03

### Added

- Add logo ([5a0c8d5](https://github.com/mrmmg/gonix/commit/5a0c8d5d54b74581087f17f96a947a322904cb90))
- Add nginx custom error pages configuration ([17d7341](https://github.com/mrmmg/gonix/commit/17d7341c9ca7cf78f86bb35a218cc017ea3b67b8))

### Fixed

- Remove gonix binary from git cache ([177b557](https://github.com/mrmmg/gonix/commit/177b5572f19520fd4d5605b116d8c4bc0eafbff4))
- File access and permission that cause 500 error ([0aeccc3](https://github.com/mrmmg/gonix/commit/0aeccc353687614fbb983efde8faa850a973d8e3))

## [1.2.0] - 2026-09-16

### Added

- Add mp4 preview ([538e327](https://github.com/mrmmg/gonix/commit/538e327bdf6bf4b965e00fefdf04b1c1dff0fbaf))
- Add TOC to readme files ([87eb977](https://github.com/mrmmg/gonix/commit/87eb977690b740fc0121566f1ab5fb9ec3fba612))
- Remove art directory on build ([9339a11](https://github.com/mrmmg/gonix/commit/9339a11a8eff64caaf91fe96fd232a5aa86b4ffa))
- Certbot ssl issue with cloudflare api ([ece3953](https://github.com/mrmmg/gonix/commit/ece3953ad940b9604a28edaa611663ca9690df02))

### Fixed

- Preview wrong html tag ([5078d0f](https://github.com/mrmmg/gonix/commit/5078d0f4881996240dde246817762d059267a22b))
- Video preview not showing up ([d4b9d8c](https://github.com/mrmmg/gonix/commit/d4b9d8c98366e02a6b587a3b4e74c0783aa34b88))
- Use a trick to preview mp4 file in the md file ([0b8990b](https://github.com/mrmmg/gonix/commit/0b8990ba24cc17763e1336357f2b6e6cb55088e5))

### Documentation

- Add Persian (Farsi) README, cross-linked with the English one ([42bad68](https://github.com/mrmmg/gonix/commit/42bad6889157100c235483ac94d3b7ddd3d5fda7))

## [1.0.0] - 2026-09-15

### Added

- Initial implementation of nginx-manager TUI ([f0ea171](https://github.com/mrmmg/gonix/commit/f0ea1716ca1da804cfb56f07e21bc013f2560ae5))
- Add automated multi-arch releases and one-command installer ([551d016](https://github.com/mrmmg/gonix/commit/551d016a64d1ed23811beb095a2bcda9d5b70c1d))

### Changed

- Rename project to GoNix ([834bca2](https://github.com/mrmmg/gonix/commit/834bca2a094b912eadfef96803a253074f0dd6ec))
- Drop scripts/install.sh; make `make install` self-contained ([85a5e75](https://github.com/mrmmg/gonix/commit/85a5e75f8da1469f78f9dd20dd6b9457febb9419))
- Version local builds from the git commit hash, drop VERSION file ([c7f7941](https://github.com/mrmmg/gonix/commit/c7f79410d27d58953a0c755b976a9afd97826f23))

### Fixed

- **tui:** Repair Esc navigation, including Backup/Restore ([11b6851](https://github.com/mrmmg/gonix/commit/11b6851643dcf30f1b1c04c66df29b83f71289d3))

### Documentation

- Add go run instructions for local dev and more test examples ([508133a](https://github.com/mrmmg/gonix/commit/508133ac6fd490f09c6b8c0e38899861dde02455))
- Mention both installers in default config header comment ([248bc15](https://github.com/mrmmg/gonix/commit/248bc158dc2379d2dacc8699782e1e67a6cc2f77))

[1.3.1]: https://github.com/mrmmg/gonix/compare/v1.2.0...v1.3.1
[1.2.0]: https://github.com/mrmmg/gonix/compare/v1.0.0...v1.2.0
[1.0.0]: https://github.com/mrmmg/gonix/releases/tag/v1.0.0

