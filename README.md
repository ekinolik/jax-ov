# jax-ov

Options Volume Monitor Service - A real-time monitoring tool for stock option volumes using massive.com WebSocket API.

## Overview

This service continuously monitors stock option volumes by connecting to massive.com's WebSocket API to stream real-time options aggregate data per second. It allows you to track large volume jumps throughout the day for all options of an underlying asset, helping identify significant market maker or hedge fund activity.

## Features

- Real-time streaming of options aggregate data (OHLC + Volume) via WebSocket
- Subscribe to all options for an underlying stock or specific option contracts
- Historical feed reconstruction from REST API
- Configurable via command-line parameters
- Automatic reconnection handling
- Graceful shutdown support

## Prerequisites

- Go 1.19 or later
- A massive.com API key (get one at https://massive.com)

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd jax-ov
```

2. Install dependencies:
```bash
go mod download
```

3. Create a `.env` file in the project root:
```bash
cp .env.example .env
```

4. Edit `.env` and add your massive.com API key:
```
MASSIVE_API_KEY=your_api_key_here
```

## Building

### Using Make (Recommended)

Build all commands for your current OS:
```bash
make all
```

Build all commands for Linux:
```bash
make linux-all
```

Build a specific command for Linux:
```bash
make linux-monitor
```

Build a specific command for your current OS:
```bash
make monitor
```

See all available targets:
```bash
make help
```

Linux binaries will be placed in `bin/linux/jax-ov/` directory.

### Using Go directly

Build a specific command:
```bash
go build -o monitor ./cmd/monitor
```

Build for Linux:
```bash
GOOS=linux GOARCH=amd64 go build -o monitor ./cmd/monitor
```

## Usage

### Commands

The project includes nine main commands:

1. **monitor** - Real-time WebSocket streaming
2. **reconstruct** - Historical feed reconstruction
3. **analyze** - Premium analysis of reconstructed feeds (JSON format)
4. **log-analyze** - Premium analysis of logger log files (JSONL format)
5. **extract** - Extract transactions for a specific time period (JSON format)
6. **log-extract** - Extract transactions for a specific time period (JSONL format)
7. **top-contracts** - Find contracts with largest premiums
8. **logger** - WebSocket logger service (logs to daily files)
9. **server** - Analysis WebSocket server (serves analyzed data to clients)

### Monitor Command (Real-time WebSocket)

#### Build the monitor command

```bash
go build -o monitor ./cmd/monitor
```

#### Subscribe to all options for an underlying stock

```bash
./monitor --ticker AAPL --mode all
```

This will subscribe to all option contracts for Apple (AAPL) and stream per-second aggregate data.

#### Subscribe to a specific option contract

```bash
./monitor --ticker AAPL --mode contract --contract O:AAPL230616C00150000
```

Replace `O:AAPL230616C00150000` with the actual option contract symbol you want to monitor.

#### Monitor Command-line Flags

- `--ticker` or `-t`: Underlying stock ticker (required, e.g., "AAPL")
- `--mode` or `-m`: Subscription mode - "all" or "contract" (default: "all")
- `--contract` or `-c`: Specific option contract symbol (required if mode is "contract")

### Reconstruct Command (Historical Feed)

#### Build the reconstruct command

```bash
go build -o reconstruct ./cmd/reconstruct
```

#### Reconstruct historical feed for a specific date

```bash
./reconstruct --ticker AAPL --date 2025-11-30
```

This will:
1. Fetch all option contracts (calls and puts) for AAPL
2. Retrieve per-second aggregates for each contract on the specified date
3. Combine all aggregates into a single JSON file sorted by timestamp
4. Save to `AAPL_options_2025-11-30.json` (or custom filename)

#### Reconstruct Command-line Flags

- `--ticker` or `-t`: Underlying stock ticker (required, e.g., "AAPL")
- `--date` or `-d`: Date in YYYY-MM-DD format (required, e.g., "2025-11-30")
- `--output` or `-o`: Output JSON file path (default: "{ticker}_options_{date}.json")
- `--workers`: Number of concurrent workers for fetching aggregates (default: 10)

#### Example with custom output

```bash
./reconstruct --ticker AAPL --date 2025-11-30 --output aapl_feed.json --workers 20
```

### Analyze Command (Premium Analysis)

#### Build the analyze command

```bash
go build -o analyze ./cmd/analyze
```

#### Analyze premium data from reconstructed feed

```bash
./analyze --input AAPL_options_2025-11-30.json
```

This will:
1. Read the reconstructed JSON file
2. Calculate total premium (volume × VWAP) for each aggregate
3. Group by time periods (default: 5 minutes)
4. Separate premiums by call/put options
5. Display results in a formatted table

#### Analyze with custom time period

```bash
./analyze --input AAPL_options_2025-11-30.json --period 1
```

This analyzes with 1-minute periods instead of the default 5 minutes.

#### Analyze and save to JSON

```bash
./analyze --input AAPL_options_2025-11-30.json --period 15 --output premium_analysis.json
```

This analyzes with 15-minute periods and saves detailed results to a JSON file.

#### Analyze Command-line Flags

- `--input` or `-i`: Input JSON file path (required, from reconstruct command)
- `--period` or `-p`: Time period in minutes (default: 5)
- `--output` or `-o`: Optional output JSON file path

### Log-Analyze Command (JSONL Log File Analysis)

#### Build the log-analyze command

```bash
go build -o log-analyze ./cmd/log-analyze
```

#### Analyze premium data from logger log files

```bash
./log-analyze --input logs/2025-11-28.jsonl
```

This will:
1. Read the JSONL log file (created by the logger service)
2. Calculate total premium (volume × VWAP × 100) for each aggregate
3. Group by time periods (default: 5 minutes)
4. Separate premiums by call/put options
5. Display results in a formatted table

#### Analyze with custom time period

```bash
./log-analyze --input logs/2025-11-28.jsonl --period 1
```

This analyzes with 1-minute periods instead of the default 5 minutes.

#### Analyze and save to JSON

```bash
./log-analyze --input logs/2025-11-28.jsonl --period 15 --output premium_analysis.json
```

#### Log-Analyze Command-line Flags

- `--input` or `-i`: Input JSONL log file path (required, from logger service)
- `--period` or `-p`: Time period in minutes (default: 5)
- `--output` or `-o`: Optional output JSON file path

**Note**: This command works the same as the `analyze` command but reads JSONL format (one JSON object per line) instead of a JSON array. Use this for analyzing log files created by the logger service.

### Extract Command (Time Period Filtering)

#### Build the extract command

```bash
go build -o extract ./cmd/extract
```

#### Extract transactions for a specific time period

```bash
./extract --input AAPL_options_2025-11-30.json --time 9:46 --period 5
```

This will extract all transactions between 9:46 AM and 9:51 AM PT (5-minute period) and output them as JSON to the console.

#### Extract with specific date

```bash
./extract --input AAPL_options_2025-11-30.json --time 14:30 --period 10 --date 2025-11-30
```

This extracts transactions from 2:30 PM to 2:40 PM PT on November 30, 2025.

#### Extract Command-line Flags

- `--input` or `-i`: Input JSON file path (required, from reconstruct command)
- `--time` or `-t`: Start time in HH:MM format (required, e.g., "9:46")
- `--period` or `-p`: Time period in minutes (default: 1)
- `--date` or `-d`: Date in YYYY-MM-DD format (optional, defaults to today)

**Note**: Times are interpreted in Pacific Time (PT).

### Log-Extract Command (Time Period Filtering for JSONL Logs)

#### Build the log-extract command

```bash
go build -o log-extract ./cmd/log-extract
```

#### Extract transactions for a specific time period from log files

```bash
./log-extract --input logs/2025-11-28.jsonl --time 9:46 --period 5
```

This will extract all transactions between 9:46 AM and 9:51 AM PT (5-minute period) from the JSONL log file and output them as JSON to the console.

#### Extract with specific date

```bash
./log-extract --input logs/2025-11-28.jsonl --time 14:30 --period 10 --date 2025-11-28
```

This extracts transactions from 2:30 PM to 2:40 PM PT on November 28, 2025.

#### Log-Extract Command-line Flags

- `--input` or `-i`: Input JSONL log file path (required, from logger service)
- `--time` or `-t`: Start time in HH:MM format (required, e.g., "9:46")
- `--period` or `-p`: Time period in minutes (default: 1)
- `--date` or `-d`: Date in YYYY-MM-DD format (optional, defaults to today)

**Note**: Times are interpreted in Pacific Time (PT). This command works the same as the `extract` command but reads JSONL format (one JSON object per line) instead of a JSON array. Use this for extracting time periods from log files created by the logger service.

### Top-Contracts Command (Largest Premium Contracts)

#### Build the top-contracts command

```bash
go build -o top-contracts ./cmd/top-contracts
```

#### Find top contracts by total premium

```bash
./top-contracts --input logs/2025-11-28.jsonl
```

This will find the top 5 contracts (default) with the largest total premium, calculated as the sum of all premiums (volume × VWAP × 100) for each contract.

#### Find top N contracts

```bash
./top-contracts --input logs/2025-11-28.jsonl --top 10
```

This finds the top 10 contracts instead of the default 5.

#### Save results to JSON

```bash
./top-contracts --input logs/2025-11-28.jsonl --top 10 --output top_contracts.json
```

This saves the top 10 contracts to a JSON file.

#### Top-Contracts Command-line Flags

- `--input` or `-i`: Input JSON or JSONL file path (required)
- `--top` or `-t`: Number of top contracts to display (default: 5)
- `--output` or `-o`: Optional output JSON file path

**Note**: This command works with both JSON (from `reconstruct`) and JSONL (from `logger`) formats. It automatically detects the format. The premium is calculated as the aggregate of all transactions per contract (sum of volume × VWAP × 100 for each contract).

### Output Format

#### Monitor Command Output

The monitor command outputs real-time aggregate data in the following format:

```
[15:04:05] Symbol: O:AAPL230616C00150000 | Volume: 150 | OHLC: O=150.50 H=151.20 L=150.30 C=150.80 | VWAP: 150.65
```

Where:
- `Symbol`: The option contract symbol
- `Volume`: Tick volume for this aggregate window
- `OHLC`: Open, High, Low, Close prices
- `VWAP`: Volume-weighted average price

#### Reconstruct Command Output

The reconstruct command outputs a JSON file containing an array of aggregate objects, each matching the WebSocket format:

```json
[
  {
    "ev": "A",
    "sym": "O:AAPL230616C00150000",
    "v": 150,
    "av": 1500,
    "op": 150.50,
    "vw": 150.65,
    "o": 150.50,
    "h": 151.20,
    "l": 150.30,
    "c": 150.80,
    "a": 150.65,
    "z": 10,
    "s": 1701432245000,
    "e": 1701432246000
  }
]
```

The aggregates are sorted chronologically by start timestamp (`s` field), simulating a real-time feed as if you were subscribed to all option contracts via WebSocket.

#### Analyze Command Output

The analyze command outputs a formatted table showing premium totals by time period:

```
Time Period          | Call Premium    | Put Premium     | Total Premium
---------------------|-----------------|-----------------|----------------
2025-11-30 09:30:00  | $1,234,567.89   | $987,654.32     | $2,222,222.21
2025-11-30 09:35:00  | $1,456,789.01   | $1,123,456.78   | $2,580,245.79
...
```

If `--output` is specified, a detailed JSON file is created:

```json
[
  {
    "period_start": "2025-11-30T09:30:00Z",
    "period_end": "2025-11-30T09:35:00Z",
    "call_premium": 1234567.89,
    "put_premium": 987654.32,
    "total_premium": 2222222.21,
    "call_volume": 15000,
    "put_volume": 12000
  }
]
```

**Premium Calculation**: Premium = Volume × VWAP (Volume Weighted Average Price)
- All strike prices are combined
- Separated by call/put option type
- Aggregated by configurable time periods (1 min, 5 min, 15 min, etc.)

#### Extract Command Output

The extract command outputs a JSON array of all aggregate transactions within the specified time period:

```json
[
  {
    "ev": "A",
    "sym": "O:AAPL230616C00150000",
    "v": 150,
    "av": 1500,
    "op": 150.50,
    "vw": 150.65,
    "o": 150.50,
    "h": 151.20,
    "l": 150.30,
    "c": 150.80,
    "a": 150.65,
    "z": 10,
    "s": 1701432245000,
    "e": 1701432246000
  }
]
```

The output is written to stdout (console) as JSON, making it easy to pipe to other tools or save to a file.

#### Log-Extract Command Output

The log-extract command outputs a JSON array of all aggregate transactions within the specified time period, identical to the extract command output format. The only difference is that it reads from JSONL log files instead of JSON array files.

#### Top-Contracts Command Output

The top-contracts command outputs a formatted table showing the top contracts by total premium with parsed contract details:

```
Rank    Underlying    Expiration    Strike    Type    Total Premium              Total Volume              Transactions
----    ----------    ----------    ------    ----    -------------              ------------              ------------
1       AAPL          2023-06-16    150.00    CALL    $12,345,678.90            1,500,000                150
2       AAPL          2023-06-16    150.00    PUT     $9,876,543.21              1,200,000                120
...
```

The contract symbol is parsed into separate columns showing:
- **Underlying**: The stock ticker (e.g., AAPL)
- **Expiration**: The expiration date in YYYY-MM-DD format
- **Strike**: The strike price with decimal point
- **Type**: CALL or PUT

If `--output` is specified, a detailed JSON file is created:

```json
[
  {
    "symbol": "O:AAPL230616C00150000",
    "total_premium": 12345678.90,
    "total_volume": 1500000,
    "option_type": "call",
    "transaction_count": 150
  }
]
```

**Premium Calculation**: Total premium per contract = sum of all (volume × VWAP × 100) for that contract across all transactions.

### Logger Service (WebSocket Data Logger)

#### Build the logger service

```bash
go build -o logger ./cmd/logger
```

#### Start logging WebSocket data

```bash
./logger --ticker AAPL --mode all --log-dir ./logs
```

This will:
1. Connect to massive.com WebSocket
2. Subscribe to all option contracts for the specified ticker
3. Log each aggregate as a single-line JSON (JSONL format) to daily files
4. Automatically use the correct file based on current date (YYYY-MM-DD.jsonl)

#### Logger Command-line Flags

- `--ticker` or `-t`: Underlying stock ticker (required, e.g., "AAPL")
- `--mode` or `-m`: Subscription mode - "all" or "contract" (default: "all")
- `--contract` or `-c`: Specific option contract symbol (required if mode is "contract")
- `--log-dir`: Log directory path (default: "./logs")

**Log File Format**:
- Location: `{log-dir}/YYYY-MM-DD.jsonl`
- Format: One JSON object per line (JSONL)
- Each line: Complete aggregate object matching WebSocket format
- File automatically rotates daily (new file each day)

### Server Service (Analysis WebSocket Server)

#### Build the server service

```bash
go build -o server ./cmd/server
```

#### Start the analysis server

```bash
./server --log-dir ./logs --period 5 --port 8080
```

This will:
1. Read log files from the specified directory
2. Analyze data using premium aggregation (same as analyze command)
3. Serve analyzed results via WebSocket
4. Send historical data to new clients on connection
5. Broadcast updates every minute with new period summaries

#### Server Command-line Flags

- `--log-dir`: Log directory path (default: "./logs")
- `--period` or `-p`: Analysis period in minutes (default: 5)
- `--port`: WebSocket server port (default: "8080")
- `--host`: Bind address (default: "localhost")

#### WebSocket Protocol

**Endpoint**: `ws://host:port/analyze`

**Message Format**:

All messages are sent as individual JSON objects representing time period summaries. There is no wrapper or type field - clients receive the summary objects directly.

**On Connection** (History):
When a client connects, they receive all historical time period summaries for the current day, sent as separate messages (one per period):

```json
{
  "period_start": "2025-11-28T09:30:00Z",
  "period_end": "2025-11-28T09:35:00Z",
  "call_premium": 1234567.89,
  "put_premium": 987654.32,
  "total_premium": 2222222.21,
  "call_put_ratio": 1.25,
  "call_volume": 15000,
  "put_volume": 12000
}
```

**Periodic Updates** (every minute):
After the initial history, clients receive new time period summaries as they become available:

```json
{
  "period_start": "2025-11-28T14:30:00Z",
  "period_end": "2025-11-28T14:35:00Z",
  "call_premium": 1234567.89,
  "put_premium": 987654.32,
  "total_premium": 2222222.21,
  "call_put_ratio": 1.25,
  "call_volume": 15000,
  "put_volume": 12000
}
```

**Note**: History and update messages are identical in format - clients cannot distinguish between them. All messages are sent as individual JSON objects (JSONL-like format over WebSocket).

#### Running Both Services

```bash
# Terminal 1: Start logger service
./logger --ticker AAPL --mode all --log-dir ./logs

# Terminal 2: Start analysis server
./server --log-dir ./logs --period 5 --port 8080

# Clients can now connect to ws://localhost:8080/analyze
```

## Project Structure

```
jax-ov/
├── cmd/
│   ├── monitor/
│   │   └── main.go          # WebSocket monitor CLI
│   ├── reconstruct/
│   │   └── main.go          # Historical feed reconstruction CLI
│   ├── analyze/
│   │   └── main.go          # Premium analysis CLI
│   ├── extract/
│   │   └── main.go          # Time period extraction CLI
│   ├── log-analyze/
│   │   └── main.go          # JSONL log file analysis CLI
│   ├── log-extract/
│   │   └── main.go          # JSONL log file extraction CLI
│   ├── top-contracts/
│   │   └── main.go          # Top contracts by premium CLI
│   ├── logger/
│   │   └── main.go          # WebSocket logger service
│   └── server/
│       └── main.go          # Analysis WebSocket server
├── internal/
│   ├── config/
│   │   └── config.go        # Configuration loading from .env
│   ├── websocket/
│   │   └── client.go        # WebSocket client wrapper
│   ├── rest/
│   │   └── client.go        # REST API client wrapper
│   ├── analysis/
│   │   └── analyzer.go      # Premium analysis logic
│   ├── logger/
│   │   └── filelogger.go    # Daily file logger
│   └── server/
│       ├── server.go        # WebSocket server
│       └── analyzer.go      # Log file analyzer
├── logs/                    # Log file directory (gitignored)
│   └── YYYY-MM-DD.jsonl     # Daily log files
├── go.mod                   # Go module file
├── go.sum                   # Go dependencies
├── .env.example             # Example environment file
└── README.md                # This file
```

## Development

### Running during development

```bash
# Run monitor command
go run ./cmd/monitor --ticker AAPL --mode all

# Run reconstruct command
go run ./cmd/reconstruct --ticker AAPL --date 2025-11-30

# Run analyze command
go run ./cmd/analyze --input AAPL_options_2025-11-30.json --period 5

# Run extract command
go run ./cmd/extract --input AAPL_options_2025-11-30.json --time 9:46 --period 5

# Run log-analyze command
go run ./cmd/log-analyze --input logs/2025-11-28.jsonl --period 5

# Run log-extract command
go run ./cmd/log-extract --input logs/2025-11-28.jsonl --time 9:46 --period 5

# Run top-contracts command
go run ./cmd/top-contracts --input logs/2025-11-28.jsonl --top 10

# Run logger service
go run ./cmd/logger --ticker AAPL --mode all --log-dir ./logs

# Run server service
go run ./cmd/server --log-dir ./logs --period 5 --port 8080
```

## Future Enhancements

- Volume jump detection and alerting
- Historical data storage
- Support for multiple underlying stocks simultaneously
- Configurable volume threshold alerts
- Data export capabilities

## License

[Add your license here]

## References

- [Massive.com Options Aggregates Per Second Documentation](https://massive.com/docs/websocket/options/aggregates-per-second)
- [Massive.com Go Client Library](https://github.com/massive-com/client-go)