# Changelog

All notable changes to this project will be documented in this file.

## [1.0.00006] - 2025-12-06

### Changed
- Package target now automatically generates trading days calendar during packaging
- Calendar is stored in `calendar/trading-days.json` within the tarball

### Added
- New `trading-days` CLI tool to generate trading days calendar for current and next year
- Support for retrieving past N trading days from generated calendar JSON file
- Uses scmhub/calendar library for NYSE trading day calculations

### Added
- New `trading-days` CLI tool to generate trading days calendar for current and next year
- Support for retrieving past N trading days from generated calendar JSON file
- Uses scmhub/calendar library for NYSE trading day calculations

## [1.0.00005] - 2025-12-06

### Changed
- Server now requires ticker parameter for WebSocket and HTTP endpoints
- Server reads and responds only with data for the specified ticker's log file

## [1.0.00004] - 2025-12-06

### Changed
- Added CHANGELOG.md to packaged tarball

## [1.0.00002] - 2025-12-06

### Changed
- Updated logger to support multi-symbol logging with per-symbol log files (format: SYMBOL_YYYY-MM-DD.jsonl)
- Made --ticker parameter optional in logger (logs all symbols if not provided)
- Updated server to read and combine aggregates from all per-symbol log files for a given date

