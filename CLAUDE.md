# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build
go build -o ameriagrab .

# Run tests
go test ./...

# Run tests with verbose output
go test ./... -v
```

## Usage

```bash
# Set credentials
export AMERIA_USERNAME=$(op read 'op://Personal/Ameriabank personal/username')
export AMERIA_PASSWORD=$(op read 'op://Personal/Ameriabank personal/password')
export AMERIA_SESSION_DIR=/tmp/ameria-session  # optional, enables session persistence

# List all accounts and cards
./ameriagrab list              # Table output
./ameriagrab list --json       # JSON output

# Get transactions for a card or account
./ameriagrab get <id>                  # Table output
./ameriagrab get <id> --json           # JSON output
./ameriagrab get <id> --size 100       # Limit number of transactions
./ameriagrab get <id> --page 1         # Pagination (0-indexed)
./ameriagrab get <id> --account        # Force account history API for cards
```

## Environment Variables

- `AMERIA_USERNAME` - Ameriabank username (required)
- `AMERIA_PASSWORD` - Ameriabank password (required)
- `AMERIA_SESSION_DIR` - Directory for session persistence (optional; if not set, no session is stored/loaded)
- `AMERIA_DEBUG_DIR` - Directory for debug HTML files on errors (optional; if not set, no debug files are saved)

## Project Overview

This is a CLI tool that authenticates with Ameriabank's online banking (myameria.am), handles phone push notification 2FA, and retrieves card/account transactions as JSON or table format.

## Project Structure

```
ameriagrab/
├── main.go              # Minimal entry point
├── cmd/
│   ├── root.go          # Cobra root command and client setup
│   ├── list.go          # list subcommand
│   └── get.go           # get subcommand
├── client/
│   ├── types.go         # All response/request types
│   ├── headers.go       # HTTP header builders and constants
│   ├── client.go        # Client struct and constructor
│   ├── session.go       # Session persistence (save/load/validate)
│   ├── auth.go          # Login, push confirmation, token exchange
│   ├── api.go           # API methods (GetTransactions, GetAccountsAndCards, etc.)
│   └── client_test.go   # Client package tests
└── output/
    ├── format.go        # Output formatting functions
    └── format_test.go   # Output package tests
```

## Architecture

### Packages

- **client**: Core API client for Ameriabank
  - `Client` struct: HTTP client, credentials, session state
  - Session persistence with JSON files
  - OAuth authentication with push 2FA
  - API methods for accounts, cards, and transactions

- **cmd**: Cobra CLI commands
  - `list`: List all accounts and cards
  - `get`: Get transactions for a specific card or account

- **output**: Formatting utilities
  - Table and JSON output formatting
  - Transaction type abbreviation (`purchase:` → `p:`, `pre-purchase:` → `prep:`)

### Authentication Flow

1. OpenID Connect auth via Keycloak at `account.myameria.am`
2. Push notification 2FA polling (`/push-status?sessionId=...`)
3. Token exchange for Bearer token
4. Fetch real client ID from `/api/users/{userId}/clients`

### Session Flow

1. `GetOrRefreshToken()` tries loading saved session first
2. `ValidateSession()` checks if token is still valid (10-second timeout)
3. If expired/invalid/timeout, performs full `Login()` with push notification wait
4. `InitializeSession()` fetches real client ID from API
5. Session saved with `SaveSession()` / `UpdateSessionClientID()`

## Critical Implementation Details

- **Client-Id header**: Must be fetched from `/api/users/{userId}/clients` API, NOT randomly generated. Required for API calls to succeed (403 otherwise).
- **Browser fingerprinting**: `BuildCDDCHeader()` and `BuildTwoFAHeader()` create base64-encoded JSON headers mimicking Firefox browser fingerprint.
- **Redirect handling**: Custom `CheckRedirect` function prevents automatic redirect following for the final OAuth fragment response.
- **OAuth client secret**: The `ClientSecret` constant is a public OAuth client secret from Ameriabank's web app frontend (standard for public OAuth clients).

## API Endpoints

- `/api/accounts-and-cards` - List all accounts and cards
- `/api/events/settled/{cardId}` - Card transactions (settled)
- `/api/events/past` - Card account history (uses linked account ID)
- `/api/history` - Account transaction history
- `/api/users/info` - User information
- `/api/users/{userId}/clients` - Get client ID

## Testing

Tests use fictional data with no personal information. Key test areas:
- Session save/load/expiration
- API response parsing
- Output formatting
- Transaction type abbreviations
