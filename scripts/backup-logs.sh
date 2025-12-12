#!/bin/bash

# Log Backup Script
# Backs up old log files to S3, keeping only the past N trading days on disk

set -euo pipefail

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Default values
LOG_DIR="${LOG_DIR:-./logs}"
NO_DELETE=false
# Script directory is the project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse command-line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --no-delete)
                NO_DELETE=true
                shift
                ;;
            --log-dir)
                LOG_DIR="$2"
                shift 2
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --no-delete    Copy files to S3 instead of moving (keeps files on disk)"
                echo "  --log-dir DIR  Log directory path (default: ./logs)"
                echo "  -h, --help     Show this help message"
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
}

# Error logging function
error() {
    echo -e "${RED}ERROR:${NC} $1" >&2
}

warning() {
    echo -e "${YELLOW}WARNING:${NC} $1" >&2
}

info() {
    echo -e "${GREEN}INFO:${NC} $1"
}

# Load environment variables from .env file
load_env() {
    local env_file="${SCRIPT_DIR}/.env"
    
    if [[ ! -f "$env_file" ]]; then
        error ".env file not found at $env_file"
        exit 1
    fi
    
    # Source .env file, ignoring comments and empty lines
    # Temporarily disable unset variable check while loading
    set +u
    set -a
    # Use a safer method to source the file
    while IFS= read -r line || [[ -n "$line" ]]; do
        # Skip comments and empty lines
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ -z "${line// }" ]] && continue
        # Export the variable
        export "$line"
    done < "$env_file"
    set +a
    set -u
}

# Validate environment variables
validate_env() {
    if [[ -z "${LOG_RETENTION_DAYS:-}" ]]; then
        error "LOG_RETENTION_DAYS not set in .env file"
        exit 1
    fi
    
    if ! [[ "$LOG_RETENTION_DAYS" =~ ^[0-9]+$ ]] || [[ "$LOG_RETENTION_DAYS" -le 0 ]]; then
        error "LOG_RETENTION_DAYS must be a positive integer, got: $LOG_RETENTION_DAYS"
        exit 1
    fi
    
    if [[ -z "${AWS_S3_PATH:-}" ]]; then
        error "AWS_S3_PATH not set in .env file"
        exit 1
    fi
    
    # Validate AWS_S3_PATH format: bucket/prefix
    if [[ ! "$AWS_S3_PATH" =~ ^[^/]+/.+$ ]]; then
        error "AWS_S3_PATH must be in format 'bucket/prefix', got: $AWS_S3_PATH"
        exit 1
    fi
    
    # Load prefix environment variables with defaults
    AWS_S3_TRANSACTIONS_PREFIX="${AWS_S3_TRANSACTIONS_PREFIX:-transactions}"
    AWS_S3_ANALYZED_PREFIX="${AWS_S3_ANALYZED_PREFIX:-analyzed}"
    AWS_S3_BACKUPS_PREFIX="${AWS_S3_BACKUPS_PREFIX:-backups}"
    
    # Validate prefixes don't have leading/trailing slashes
    if [[ "$AWS_S3_TRANSACTIONS_PREFIX" =~ ^/|/$ ]]; then
        error "AWS_S3_TRANSACTIONS_PREFIX should not have leading or trailing slashes"
        exit 1
    fi
    
    if [[ "$AWS_S3_ANALYZED_PREFIX" =~ ^/|/$ ]]; then
        error "AWS_S3_ANALYZED_PREFIX should not have leading or trailing slashes"
        exit 1
    fi
    
    if [[ "$AWS_S3_BACKUPS_PREFIX" =~ ^/|/$ ]]; then
        error "AWS_S3_BACKUPS_PREFIX should not have leading or trailing slashes"
        exit 1
    fi
    
    # Load analysis period with default
    ANALYSIS_PERIOD="${ANALYSIS_PERIOD:-1}"
    
    # Validate analysis period
    if ! [[ "$ANALYSIS_PERIOD" =~ ^[0-9]+$ ]] || [[ "$ANALYSIS_PERIOD" -le 0 ]]; then
        error "ANALYSIS_PERIOD must be a positive integer, got: $ANALYSIS_PERIOD"
        exit 1
    fi
    
    # Load analyzed temp directory with default
    ANALYZED_TMP_DIR="${ANALYZED_TMP_DIR:-tmp_analyzed}"
    
    # Validate it's a relative path (no leading slash, no ..)
    if [[ "$ANALYZED_TMP_DIR" =~ ^/|\.\./|\.\.$ ]]; then
        error "ANALYZED_TMP_DIR must be a relative path (no leading slash or ..), got: $ANALYZED_TMP_DIR"
        exit 1
    fi
}

# Find trading-days binary
find_trading_days_binary() {
    # First, try to find it in the same directory as this script
    local local_binary="${SCRIPT_DIR}/../trading-days"
    if [[ -f "$local_binary" ]] && [[ -x "$local_binary" ]]; then
        echo "$local_binary"
        return 0
    fi
    
    # Try in script directory (project root)
    local root_binary="${SCRIPT_DIR}/trading-days"
    if [[ -f "$root_binary" ]] && [[ -x "$root_binary" ]]; then
        echo "$root_binary"
        return 0
    fi
    
    # Try in PATH
    if command -v trading-days &> /dev/null; then
        echo "trading-days"
        return 0
    fi
    
    error "trading-days binary not found. Please ensure it's in PATH or in the project directory."
    exit 1
}

# Find log-analyze binary
find_log_analyze_binary() {
    # Try in script directory (project root) first
    local root_binary="${SCRIPT_DIR}/log-analyze"
    if [[ -f "$root_binary" ]] && [[ -x "$root_binary" ]]; then
        echo "$root_binary"
        return 0
    fi
    
    # Try in PATH
    if command -v log-analyze &> /dev/null; then
        echo "log-analyze"
        return 0
    fi
    
    error "log-analyze binary not found. Please ensure it's in PATH or in the project directory."
    exit 1
}

# Find calendar/trading-days.json file
find_trading_days_file() {
    # Find calendar file relative to this script's location
    local script_dir
    script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
    
    # Look for calendar/trading-days.json in the same directory as this script
    local calendar_file="${script_dir}/calendar/trading-days.json"
    if [[ -f "$calendar_file" ]]; then
        echo "$calendar_file"
        return 0
    fi
    
    # If script is in scripts/ subdirectory, look in parent directory
    if [[ "$script_dir" == */scripts ]]; then
        calendar_file="${script_dir%/scripts}/calendar/trading-days.json"
        if [[ -f "$calendar_file" ]]; then
            echo "$calendar_file"
            return 0
        fi
    fi
    
    error "calendar/trading-days.json not found. Expected at: ${script_dir}/calendar/trading-days.json"
    exit 1
}

# Get trading days to keep
get_trading_days_to_keep() {
    local trading_days_binary="$1"
    local trading_days_file="$2"
    local retention_days="$3"
    
    # Run trading-days command to get past N trading days
    local output
    if ! output=$("$trading_days_binary" --load "$trading_days_file" --past "$retention_days" 2>&1); then
        error "Failed to get trading days: $output"
        exit 1
    fi
    
    # Parse JSON array output
    local dates
    if ! dates=$(echo "$output" | jq -r '.[]' 2>/dev/null); then
        error "Invalid trading days output (not valid JSON array): $output"
        exit 1
    fi
    
    # Count dates and validate
    local date_count
    date_count=$(echo "$dates" | wc -l | tr -d ' ')
    
    if [[ "$date_count" -ne "$retention_days" ]]; then
        error "Invalid trading days output (expected $retention_days dates, got $date_count)"
        exit 1
    fi
    
    # Validate each date is in YYYY-MM-DD format and is a valid date
    while IFS= read -r date; do
        if [[ ! "$date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
            error "Invalid date format in trading days output: $date (expected YYYY-MM-DD)"
            exit 1
        fi
        
        # Validate it's a valid date using date command
        # Try GNU date format first (Linux), then BSD date format (macOS)
        local date_valid=false
        if date -d "$date" &>/dev/null 2>&1; then
            # GNU date (Linux)
            date_valid=true
        elif date -j -f "%Y-%m-%d" "$date" "+%Y-%m-%d" &>/dev/null 2>&1; then
            # BSD date (macOS)
            date_valid=true
        fi
        
        if [[ "$date_valid" == "false" ]]; then
            error "Invalid date in trading days output: $date (not a valid calendar date)"
            exit 1
        fi
    done <<< "$dates"
    
    # Return dates as space-separated string for easy checking
    echo "$dates" | tr '\n' ' '
}

# Extract date from log filename
extract_date_from_filename() {
    local filename="$1"
    # Pattern: SYMBOL_YYYY-MM-DD.jsonl
    # Extract the YYYY-MM-DD part (last underscore before .jsonl)
    if [[ "$filename" =~ _([0-9]{4}-[0-9]{2}-[0-9]{2})\.jsonl$ ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo ""
    fi
}

    # Check if date is in keep list
    is_date_in_keep_list() {
        local date="$1"
        local keep_list="$2"
        
        # Check if date exists in the keep list
        [[ " $keep_list " =~ " $date " ]]
    }

# Backup analyzed data
backup_analyzed_data() {
    local files_to_backup_ref="$1[@]"
    local files_array=("${!files_to_backup_ref}")
    local log_analyze_binary="$2"
    
    if [[ ${#files_array[@]} -eq 0 ]]; then
        info "No files to analyze"
        return 0
    fi
    
    # Create temporary directory for analyzed output (relative to project root)
    local analyzed_dir="${SCRIPT_DIR}/${ANALYZED_TMP_DIR}"
    mkdir -p "$analyzed_dir"
    
    info "Analyzing ${#files_array[@]} transaction log file(s) for backup..."
    
    local analyzed_count=0
    local failed_count=0
    
    # Process each file
    for file in "${files_array[@]}"; do
        local filename=$(basename "$file")
        local output_filename="${filename%.jsonl}.json"
        local output_path="${analyzed_dir}/${output_filename}"
        
        info "Analyzing $filename..."
        
        # Run log-analyze on this file (quiet mode to reduce output)
        if "$log_analyze_binary" --input "$file" --output "$output_path" --period "$ANALYSIS_PERIOD" --quiet 2>&1; then
            analyzed_count=$((analyzed_count + 1))
            info "Successfully analyzed: $filename"
        else
            failed_count=$((failed_count + 1))
            warning "Failed to analyze: $filename"
            # Remove partial output if it exists
            [[ -f "$output_path" ]] && rm -f "$output_path"
        fi
    done
    
    info "Analysis complete. Successfully analyzed: $analyzed_count, Failed: $failed_count"
    
    # If no files were successfully analyzed, skip backup
    if [[ $analyzed_count -eq 0 ]]; then
        warning "No files were successfully analyzed, skipping analyzed data backup"
        rm -rf "$analyzed_dir"
        return 0
    fi
    
    # Backup analyzed files to S3
    local s3_analyzed_dest="s3://${AWS_S3_PATH}/${AWS_S3_ANALYZED_PREFIX}/"
    info "Backing up analyzed data to S3: $s3_analyzed_dest"
    
    set +e
    local backup_success=false
    if aws s3 cp "$analyzed_dir" "$s3_analyzed_dest" --recursive 2>&1; then
        backup_success=true
        info "Successfully backed up analyzed data to S3"
    else
        error "Failed to backup analyzed data to S3"
    fi
    set -e
    
    # Always delete the temporary directory, regardless of backup success
    rm -rf "$analyzed_dir"
    if [[ "$backup_success" == "true" ]]; then
        info "Removed temporary analyzed data directory"
    else
        warning "Removed temporary analyzed data directory (backup failed)"
    fi
    
    # Return error if backup failed
    if [[ "$backup_success" == "false" ]]; then
        return 1
    fi
    
    return 0
}

# Create full backup tarball of logs directory
create_full_backup() {
    local backup_timestamp
    backup_timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_filename="logs-full-backup-${backup_timestamp}.tar.gz"
    local backup_path="${SCRIPT_DIR}/${backup_filename}"
    local s3_backup_path="s3://${AWS_S3_PATH}/${AWS_S3_BACKUPS_PREFIX}/${backup_filename}"
    
    info "Creating full backup tarball of logs directory..."
    
    # Create tarball of entire logs directory in script directory to avoid including it in the backup
    if ! tar -czf "$backup_path" -C "$(dirname "$LOG_DIR")" "$(basename "$LOG_DIR")" 2>/dev/null; then
        error "Failed to create full backup tarball"
        # Clean up any partial file that may have been created
        if [[ -f "$backup_path" ]]; then
            rm -f "$backup_path"
        fi
        exit 1
    fi
    
    info "Full backup created: $backup_filename ($(du -h "$backup_path" | cut -f1))"
    
    # Upload to S3 using cp (not mv)
    info "Uploading full backup to S3: $s3_backup_path"
    local upload_success=false
    if aws s3 cp "$backup_path" "$s3_backup_path" 2>&1; then
        info "Successfully uploaded full backup to S3"
        upload_success=true
    else
        error "Failed to upload full backup to S3"
    fi
    
    # Always delete the local tarball, regardless of upload success or failure
    if [[ -f "$backup_path" ]]; then
        rm -f "$backup_path"
        if [[ "$upload_success" == "true" ]]; then
            info "Removed local backup tarball after successful upload"
        else
            warning "Removed local backup tarball (upload failed)"
        fi
    fi
    
    # Exit if upload failed
    if [[ "$upload_success" == "false" ]]; then
        exit 1
    fi
}

# Main backup function
main() {
    # Parse command-line arguments
    parse_args "$@"
    
    info "Starting log backup process..."
    
    # Load and validate environment
    load_env
    validate_env
    
    info "LOG_RETENTION_DAYS: $LOG_RETENTION_DAYS"
    info "AWS_S3_PATH: $AWS_S3_PATH"
    
    # Find trading-days binary and calendar file
    local trading_days_binary
    trading_days_binary=$(find_trading_days_binary)
    info "Found trading-days binary: $trading_days_binary"
    
    local trading_days_file
    trading_days_file=$(find_trading_days_file)
    info "Found trading days file: $trading_days_file"
    
    # Get trading days to keep
    local keep_dates
    keep_dates=$(get_trading_days_to_keep "$trading_days_binary" "$trading_days_file" "$LOG_RETENTION_DAYS")
    info "Trading days to keep: $keep_dates"
    
    # Check if log directory exists
    if [[ ! -d "$LOG_DIR" ]]; then
        error "Log directory not found: $LOG_DIR"
        exit 1
    fi
    
    # Create full backup tarball first (disaster recovery backup)
    create_full_backup
    
    # Find log-analyze binary for analyzed data backup
    local log_analyze_binary
    log_analyze_binary=$(find_log_analyze_binary)
    info "Found log-analyze binary: $log_analyze_binary"
    
    # Scan log directory for files
    local files_to_backup=()
    local files_to_keep=()
    
    info "Scanning log directory: $LOG_DIR"
    # Use find to handle cases where no files match (pattern: SYMBOL_YYYY-MM-DD.jsonl)
    while IFS= read -r -d '' file; do
        local filename=$(basename "$file")
        local date
        date=$(extract_date_from_filename "$filename")
        
        if [[ -z "$date" ]]; then
            warning "Skipping file with invalid date format: $filename"
            continue
        fi
        
        if is_date_in_keep_list "$date" "$keep_dates"; then
            files_to_keep+=("$file")
        else
            files_to_backup+=("$file")
        fi
    done < <(find "$LOG_DIR" -maxdepth 1 -name "*_*.jsonl" -type f -print0 2>/dev/null || true)
    
    # Safety check: Ensure at least one file will be kept
    if [[ ${#files_to_keep[@]} -eq 0 ]]; then
        error "No files would be kept on disk - aborting backup to prevent data loss"
        exit 1
    fi
    
    info "Files to keep on disk: ${#files_to_keep[@]}"
    info "Files to backup to S3: ${#files_to_backup[@]}"
    
    if [[ ${#files_to_backup[@]} -eq 0 ]]; then
        info "No files to backup. All files are within retention period."
        exit 0
    fi
    
    # Build exclude patterns for dates to keep
    # Pattern: *_YYYY-MM-DD.jsonl for each date in keep_dates
    local exclude_patterns=()
    for date in $keep_dates; do
        exclude_patterns+=("--exclude" "*_${date}.jsonl")
    done
    
    # Backup analyzed data before backing up transaction logs
    if ! backup_analyzed_data files_to_backup "$log_analyze_binary"; then
        warning "Analyzed data backup failed, but continuing with transaction log backup"
    fi
    
    # Backup files to S3 using recursive copy with exclude patterns
    local s3_dest="s3://${AWS_S3_PATH}/${AWS_S3_TRANSACTIONS_PREFIX}/"
    
    if [[ "$NO_DELETE" == "true" ]]; then
        info "Using COPY mode (--no-delete): files will be kept on disk"
    else
        info "Using COPY mode: files will be removed from disk after successful backup"
    fi
    
    info "Backing up files to S3 (excluding ${#files_to_keep[@]} file(s) to keep)..."
    
    # Temporarily disable exit on error for this command
    set +e
    local backup_success=false
    
    # Use recursive copy with exclude patterns
    # --exclude patterns will exclude files matching the keep dates
    if aws s3 cp "$LOG_DIR" "$s3_dest" --recursive "${exclude_patterns[@]}" 2>&1; then
        backup_success=true
        info "Successfully backed up files to S3"
        
        # If not using --no-delete, remove files from disk after successful backup
        if [[ "$NO_DELETE" != "true" ]]; then
            info "Removing backed up files from disk..."
            for file in "${files_to_backup[@]}"; do
                if [[ -f "$file" ]]; then
                    rm -f "$file"
                fi
            done
            info "Removed ${#files_to_backup[@]} file(s) from disk"
        fi
    else
        error "Failed to backup files to S3"
        warning "All files remain on disk"
    fi
    set -e
    
    if [[ "$backup_success" == "false" ]]; then
        exit 1
    fi
    
    info "Backup complete. Successfully backed up ${#files_to_backup[@]} file(s)"
    
    exit 0
}

# Run main function
main "$@"

