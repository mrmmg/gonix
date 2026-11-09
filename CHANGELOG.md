# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.3.2] - 2026-11-09

### Fixed

- **ci:** Publish the per-file .sha256 checksums with each release ([079c684](https://github.com/mrmmg/gonix/commit/079c684823d97e6ad3fd778cbef5699f3816f22d))

## [2.3.1] - 2026-11-09

### Fixed

- **tui:** Do not crash when Esc cancels the Add New Host wizard ([76179fa](https://github.com/mrmmg/gonix/commit/76179faeba57c5b7915d7a015008f1fd9030920d))

## [2.3.0] - 2026-11-09

### Added

- Certificate sources, a friendlier TUI and self-update ([3f127e8](https://github.com/mrmmg/gonix/commit/3f127e8cdf2e81f96b31dd63a974e5212105ef30))

### Documentation

- Add screenshots of the Grafana dashboard ([1036012](https://github.com/mrmmg/gonix/commit/103601258d50ecc055d14225f7abfa59176c691d))
- Document certificate sources, the TUI keys and updates ([9c5128b](https://github.com/mrmmg/gonix/commit/9c5128b4a826dab4d76be6858bf6690011b150f9))

### Maintenance

- Update CHANGELOG.md on main after each release ([3bf2034](https://github.com/mrmmg/gonix/commit/3bf2034f8b24ba21a9c3034a97eb87727fd18e45))

## [2.2.0] - 2026-11-06

### Added

- Add a Prometheus exporter and a Grafana dashboard ([e474e7e](https://github.com/mrmmg/gonix/commit/e474e7ef2b6c96126f11b49d1e701b2879c1c7a0))

## [2.1.0] - 2026-11-06

### Added

- **nginx:** Add a lossless Nginx configuration parser ([c6731d4](https://github.com/mrmmg/gonix/commit/c6731d4b6a571cd798bcaaf8220ed908ce735ff4))
- Adopt hand-written hosts, keeping certbot's certificates ([d266e66](https://github.com/mrmmg/gonix/commit/d266e66dce39d6c680918cd36602c0999c8fe5ab))
- **config:** Make the HTTP and HTTPS ports of new hosts configurable ([ae09714](https://github.com/mrmmg/gonix/commit/ae097145cd1069412ff2ede1a9e8307e78ae2b5b))
- **tui:** Add the Lab, a separate Nginx for trying GoNix ([10ba074](https://github.com/mrmmg/gonix/commit/10ba07495eef00b503bb07524a26ac60eb666e8c))

### Fixed

- **tui:** Use the whole terminal height and show where long text is ([5638a07](https://github.com/mrmmg/gonix/commit/5638a07efe2dfb7ca66485e0848062ed23bcffc1))

### Maintenance

- Ignore the .roadmap-ideas folder ([6b2a738](https://github.com/mrmmg/gonix/commit/6b2a738af2b877ff65cf0608dba7e443ba52f395))
- Remove idea.md ([940ee9a](https://github.com/mrmmg/gonix/commit/940ee9a9b4624b3ba403d442e1d3754ed4c1b7ca))

## [2.0.0] - 2026-11-03

### Added

- Add redirect host mode and editable domain aliases ([efcdfe8](https://github.com/mrmmg/gonix/commit/efcdfe8a797ca41e0d0c162ffc4f33ccbdba3192))
- Add IP access lists, satisfy any/all and real client IP ([916cf15](https://github.com/mrmmg/gonix/commit/916cf15e915e5d8b017ec8359feefd9cf420540c))
- Add host presets, version-aware HTTP/2 and HTTP/3 ([f670e07](https://github.com/mrmmg/gonix/commit/f670e075d9b78524f755e5aed51586b77cd39f7b))
- Add named rate limits for hosts and paths ([78ddac5](https://github.com/mrmmg/gonix/commit/78ddac5363c25d13583b0886c9b3c523b194f92a))
- Add per-host maintenance mode ([ab90e96](https://github.com/mrmmg/gonix/commit/ab90e968498d8631c7a14e984ce03da0df44c87f))
- Add live logs, traffic summary, config check and upstream health ([09c8005](https://github.com/mrmmg/gonix/commit/09c80058fbc9dc1cd926f727a72402efe0123eaf))
- Add upstream groups with load balancing and failover ([f1c32a7](https://github.com/mrmmg/gonix/commit/f1c32a7e8f10c7311ac5a688f2246f529dd5b03d))
- Add proxy cache preset ([715dc80](https://github.com/mrmmg/gonix/commit/715dc805e5689d97820d96c3143def0029c81a30))
- Add TCP/UDP stream proxies ([871f962](https://github.com/mrmmg/gonix/commit/871f962a1517ed9289f8c8b85e953b23e68103fc))
- Add country blocking (GeoIP) ([61cbed4](https://github.com/mrmmg/gonix/commit/61cbed4f3c57e1da7a10b4bd8eeb1704ffafc607))
- Add Git-based configuration history and restore ([ae9c8b0](https://github.com/mrmmg/gonix/commit/ae9c8b0a2f1344261da6c8be93877a91ff284a1f))
- Add non-interactive CLI with JSON, dry-run and an AI agent skill ([74806ee](https://github.com/mrmmg/gonix/commit/74806eecada6004d6e3f302bd29fc69163dc9513))
- Import hosts from Nginx Proxy Manager ([364ba0f](https://github.com/mrmmg/gonix/commit/364ba0fdc469e520b46c467dedea7c70d5a261ea))
- Manage multiple Nginx instances ([dd43abc](https://github.com/mrmmg/gonix/commit/dd43abcd75d07f673680fd27c944ae98c89896e6))
- Add MCP server for AI agents (gonix mcp) ([a96661b](https://github.com/mrmmg/gonix/commit/a96661bb8fb891210242dfd4b9baec4858d3da31))

### Changed

- Move service wiring into internal/app; add export/import core ([0e61fd9](https://github.com/mrmmg/gonix/commit/0e61fd90a22473f7d0e079eba6ffba728ff47f97))

### Fixed

- **tui:** Scroll long menus on small terminals ([c219e35](https://github.com/mrmmg/gonix/commit/c219e3567a1d2b2631aed6e7d1726ee71a23d4b0))
- Commit the embedded agent skill that .gitignore was hiding ([870296c](https://github.com/mrmmg/gonix/commit/870296c45727baa327c17db9db608a1970724a0b))

### Documentation

- Add README badges and refresh documentation ([ec81b34](https://github.com/mrmmg/gonix/commit/ec81b34e0cb1ffdec59b20ac3e6ceade9597b74b))
- Bypass stale Shields cache for downloads badge ([d27d944](https://github.com/mrmmg/gonix/commit/d27d9448e00000751df03dfab3ad4eb635da026e))
- Add star history chart to README ([7798d64](https://github.com/mrmmg/gonix/commit/7798d641126dd3cc5f5ba3b37d0b257512093514))
- Add idea.md with feature roadmap and AI agent support plan ([8de8490](https://github.com/mrmmg/gonix/commit/8de84903de6c21ebe509de7fe5416f7aa1404e1a))
- **idea:** Detail redirect host workaround and planned design ([78136d1](https://github.com/mrmmg/gonix/commit/78136d191bd2c7a913d2a9069eb48a32261edde4))
- Document the new features, CLI, AI agents and instances ([31fb3ec](https://github.com/mrmmg/gonix/commit/31fb3ecb2a629060080a635769fcc8f08373f79c))
- Add bilingual documentation site (Hugo + Hextra) ([d446690](https://github.com/mrmmg/gonix/commit/d446690af533a4df1821c1cee78f7ec8c8899bbc))
- Slim down the READMEs and link to the documentation site ([ed936e2](https://github.com/mrmmg/gonix/commit/ed936e22672db3f0e0521650326fc6fb6eab8b05))

### Maintenance

- Remove task.md ([e45ff73](https://github.com/mrmmg/gonix/commit/e45ff73b67d40b033a31ca8f7e25b4e64acdad38))
- Gofmt upstreams test ([e7a30fd](https://github.com/mrmmg/gonix/commit/e7a30fdd1c876f3a56501d1cf0c4f9b2180e484a))
- Gofmt npm test ([46942c5](https://github.com/mrmmg/gonix/commit/46942c5dd716176b74963895008e2940572a3d25))
- Publish docs to GitHub Pages and keep them out of releases ([7b045ff](https://github.com/mrmmg/gonix/commit/7b045ffc906decd73389c7e69c88a74fd074499a))

## [1.5.0] - 2026-10-03

### Added

- Add template or seperate file mode for nginx errors ([7df6bf4](https://github.com/mrmmg/gonix/commit/7df6bf4ab0c1458f2f22f17e2f5666893cc6932c))

## [1.4.0] - 2026-10-03

### Added

- Install and add cliff to generate CHANGELOG.md ([2a8a0d2](https://github.com/mrmmg/gonix/commit/2a8a0d275b284c3b71b5eebe0b9cdfc9df88ae3b))

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

[2.3.2]: https://github.com/mrmmg/gonix/compare/v2.3.1...v2.3.2
[2.3.1]: https://github.com/mrmmg/gonix/compare/v2.3.0...v2.3.1
[2.3.0]: https://github.com/mrmmg/gonix/compare/v2.2.0...v2.3.0
[2.2.0]: https://github.com/mrmmg/gonix/compare/v2.1.0...v2.2.0
[2.1.0]: https://github.com/mrmmg/gonix/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/mrmmg/gonix/compare/v1.5.0...v2.0.0
[1.5.0]: https://github.com/mrmmg/gonix/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/mrmmg/gonix/compare/v1.3.1...v1.4.0
[1.3.1]: https://github.com/mrmmg/gonix/compare/v1.2.0...v1.3.1
[1.2.0]: https://github.com/mrmmg/gonix/compare/v1.0.0...v1.2.0
[1.0.0]: https://github.com/mrmmg/gonix/releases/tag/v1.0.0

