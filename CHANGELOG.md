# Changelog

All notable changes to this project will be documented in this file.

## [1.0.00015] - 2025-12-09

### Changed
- Server now checks for new data every 5 seconds (instead of 1 minute) for near real-time updates
- Server sends updates for both completed periods and current in-progress periods
- Current period updates are sent when data changes, providing live updates to WebSocket clients

## [1.0.00013] - 2025-12-07

### Added
- Apple Sign-In authentication for server endpoints
- `/auth/login` POST endpoint that validates Apple identity tokens and returns JWT session tokens
- JWT middleware to protect `/analyze` and `/transactions` endpoints
- JWT session tokens with 7-day expiration (configurable via JWT_EXPIRY_HOURS)
- Environment variables: APPLE_CLIENT_ID, APPLE_TEAM_ID, APPLE_PRIVATE_KEY, JWT_SECRET, JWT_EXPIRY_HOURS
- Apple identity token validation using Apple's public keys from JWKS endpoint
- Session tokens include sub (Apple user ID), session_id, and expiration claims

## [1.0.00008] - 2025-12-07

### Added
- New `scripts/backup-logs.sh` script to backup old log files to S3
- Script keeps only past N trading days on disk (configurable via LOG_RETENTION_DAYS)
- Automatic S3 backup with safety checks to prevent data loss
- Environment variables: LOG_RETENTION_DAYS and AWS_S3_PATH
- Full backup tarball of entire logs directory before individual file backups (disaster recovery)
- `--no-delete` option to copy files to S3 instead of moving (keeps files on disk)
- Enhanced date validation using GNU date command to ensure valid calendar dates
- Backup script included in packaged tarball (in scripts/ directory)

## [1.0.00007] - 2025-12-06

### Fixed
- Trading-days command now correctly excludes holidays (e.g., Christmas, New Year's Day, Independence Day)
- Changed from using `IsBusinessDay` to `IsOpen` at market hours to properly identify trading days

## [1.0.00006] - 2025-12-06

### Changed
- Package target now automatically generates trading days calendar during packaging
- Calendar is stored in `calendar/trading-days.json` within the tarball

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

