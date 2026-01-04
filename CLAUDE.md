# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build
go build -o ameriagrab .
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
```

## Environment Variables

- `AMERIA_USERNAME` - Ameriabank username (required)
- `AMERIA_PASSWORD` - Ameriabank password (required)
- `AMERIA_SESSION_DIR` - Directory for session persistence (optional; if not set, no session is stored/loaded)
- `AMERIA_DEBUG_DIR` - Directory for debug HTML files on errors (optional; if not set, no debug files are saved)

## Project Overview

This is a CLI tool that authenticates with Ameriabank's online banking (myameria.am), handles phone push notification 2FA, and retrieves card/account transactions as JSON.

## Architecture

Single-file Go application (`main.go`) with these key components:

- **AmeriabankClient**: Main struct managing HTTP client, cookie jar, and session state
- **Authentication flow**:
  1. OpenID Connect auth via Keycloak at `account.myameria.am`
  2. Push notification 2FA polling (`/push-status?sessionId=...`)
  3. Token exchange for Bearer token
- **Session persistence**: Saves tokens, cookies, and client ID to `/tmp/ameriagrab-{username}/session.json` (0600 permissions)
- **API calls**: Uses `ob.myameria.am` with special headers (`X-Banqr-CDDC`, `X-Banqr-2FA`, `Client-Id`)

## Critical Implementation Details

- **Client-Id header**: Must be fetched from `/api/users/{userId}/clients` API, NOT randomly generated. This is required for API calls to succeed (403 otherwise).
- **Browser fingerprinting**: `buildCDDCHeader()` and `buildTwoFAHeader()` create base64-encoded JSON headers mimicking Firefox browser fingerprint.
- **Redirect handling**: Custom `CheckRedirect` function prevents automatic redirect following for the final OAuth fragment response.

## Session Flow

1. `GetOrRefreshToken()` tries loading saved session first
2. `validateSession()` checks if token is still valid (10-second timeout to detect killed server sessions)
3. If expired/invalid/timeout, clears clientID and performs full `Login()` with push notification wait
4. `InitializeSession()` fetches real client ID from API
5. Session saved with `saveSession()` / `updateSessionClientID()`
