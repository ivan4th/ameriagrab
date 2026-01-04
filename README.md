# ameriagrab

CLI tool to download and manage transaction data from Ameriabank online banking.

## Features

- List all accounts and cards with balances
- Download transaction history for cards and accounts
- Sync all data to a local SQLite database for offline access
- Create balance snapshots to track changes over time
- Extended transaction info (beneficiary details, SWIFT data)
- Session persistence to avoid repeated 2FA confirmations

## Installation

```bash
go install github.com/ivan4th/ameriagrab@latest
```

Or build from source:

```bash
git clone https://github.com/ivan4th/ameriagrab
cd ameriagrab
go build .
```

## Configuration

Set environment variables:

```bash
export AMERIA_USERNAME="your_username"
export AMERIA_PASSWORD="your_password"
export AMERIA_DB_PATH="/path/to/ameria.db"  # Required for sync/local mode, also stores session
```

## Usage

### List accounts and cards

```bash
# From API
ameriagrab list

# From local database
ameriagrab list --local

# JSON output
ameriagrab list --json
```

### Get transactions

```bash
# Get transactions for a card (by ID from 'list' output)
ameriagrab get 1234567890

# Get account history (works for both cards and accounts)
ameriagrab get 1234567890 --account

# Extended info (beneficiary, card number, SWIFT details)
ameriagrab get 1234567890 --extended

# Pagination
ameriagrab get 1234567890 --size 100 --page 0

# From local database
ameriagrab get 1234567890 --local

# Show oldest first
ameriagrab get 1234567890 --asc

# Wide output (no column truncation)
ameriagrab get 1234567890 --wide
```

### Sync to local database

```bash
# Sync all accounts, cards, and transactions
ameriagrab sync

# Sync with verbose output
ameriagrab sync --verbose

# Sync and create a balance snapshot
ameriagrab sync --snapshot
```

### Balance snapshots

```bash
# List all snapshots
ameriagrab list-snapshots

# JSON output
ameriagrab list-snapshots --json
```

## Authentication

The tool uses Ameriabank's mobile app authentication flow:

1. On first run, you'll be prompted to confirm via push notification on your phone
2. Session is saved to the SQLite database and reused until expiration
3. Tokens are automatically refreshed when possible

## Database

When using `sync`, data is stored in SQLite with the following tables:

- `products` - Cards and accounts with current balances
- `card_transactions` - Card-specific transactions
- `card_linked_account_transactions` - Linked account history for cards
- `account_transactions` - Account transaction history
- `snapshots` / `snapshot_products` - Point-in-time balance captures

## License

MIT
