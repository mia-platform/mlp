# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.1] - 2021-03-30
### Fixed

- [BMP-940](https://makeitapp.atlassian.net/browse/BMP-940): fix annotation length by using an unique name, `mia-platform.eu/dependenciesChecksum`, for all dependencies and its value is a object of key-values of all the dependencies.

## [0.4.0] - 2021-03-17

### Added

- Add deploy type support, `smart deploy` or `deploy all`.

### Fixes

- [BMP-823](https://makeitapp.atlassian.net/browse/BMP-823): fix quote in configmap strings

## [0.3.2] - 2021-01-22

### Fixed

- [MPPS-57](https://makeitapp.atlassian.net/browse/MPPS-57): interpolation of variables inside single quotes

## [0.3.1] - 2020-11-25

### Added

- Add manual resource deletion

## [0.3.0] - 2020-11-02

### Added

- Add label `"app.kubernetes.io/managed-by": "mia-platform"`
- Unset original resource namespace
- Add resource deletion if no longer deployed with `mlp`

## [0.2.0] - 2020-10-20

### Added

- Add Job creation from CronJob

## [0.1.1] - 2020-10-14

### Changed

- Ignore unreadable or missing files passed as inputs to subcommands

## [0.1.0] - 2020-10-13

### Added

- Initial Release 🎉🎉🎉

[Unreleased]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.4.1...HEAD
[0.4.1]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.4.0...v0.4.1
[0.4.0]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.3.2...v0.4.0
[0.3.2]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.3.1...v0.3.2
[0.3.1]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.3.0...v0.3.1
[0.3.0]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.2.0...v0.3.0
[0.2.0]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.1.1...v0.2.0
[0.1.1]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/compare/v0.1.0...v0.1.1
[0.1.0]: https://git.tools.mia-platform.eu/platform/devops/deploy/-/tags/v0.1.0
