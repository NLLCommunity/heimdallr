# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Heimdallr is a Discord moderation bot built in Go using the [Disgo](https://github.com/disgoorg/disgo) library (v0.19). It uses GORM with SQLite for persistence and Viper for configuration. Licensed under GPL-3.0.

## Build & Development Commands

```bash
go build -o heimdallr .       # Build binary
go run .                       # Run directly
go test ./...                  # Run all tests
go test -race ./...            # Run tests with race detection
go test -run TestName ./pkg/   # Run a single test
golangci-lint run --timeout=5m # Lint (uses .golangci.yml config)
```

Taskfile (`task` command) is also available: `task build`, `task run`, `task default` (build + run).

For live-reload during development: `air` (config in `air.conf`).

## Architecture

**Entry point:** `main.go` — loads config, initializes the database, registers interaction commands and event listeners on a Disgo client, opens the gateway, and starts scheduled background tasks.

**Key flow:**
1. `config/` package `init()` loads `config.toml` via Viper (also reads `HEIMDALLR_`-prefixed env vars)
2. `model.InitDB()` opens SQLite and runs GORM auto-migrations
3. Interaction commands are registered by calling each package's `Register(r *handler.Mux)` function, which returns `[]discord.ApplicationCommandCreate` and attaches handlers to the mux
4. Event listeners (in `listeners/`) are attached directly to the Disgo client
5. Scheduled tasks run as ticker-based goroutines

### Package Responsibilities

- **`interactions/`** — Each subdirectory is a slash command group (ban, kick, infractions, gatekeep, prune, quote, role_button, modmail, admin, ping). Each exposes a `Register(*handler.Mux) []discord.ApplicationCommandCreate` function.
- **`listeners/`** — Discord event handlers (ban logging, join/leave tracking, gatekeep enforcement, anti-spam). Attached in `main.go` via `bot.WithEventListenerFunc()`.
- **`model/`** — GORM models and database initialization. Key models: `Infraction`, `TempBan`, `GuildSettings`, `ModmailSettings`, `MemberPendingPrune`.
- **`scheduled_tasks/`** — Background jobs (temp ban expiry, stale prune cleanup). Uses `task/` package's custom scheduler.
- **`config/`** — Viper configuration via package-level `init()`. Imported as `_ "...heimdallr/config"` for side effects.
- **`utils/`** — Helpers: duration parsing (`ParseLongDuration`), infraction weight decay (`CalcHalfLife`), Discord formatting, paginated member iteration.
- **`globals/`** — Shared mutable state (e.g., excluded users list).

### Adding a New Command

1. Create a new subdirectory under `interactions/`
2. Implement a `Register(*handler.Mux) []discord.ApplicationCommandCreate` function
3. Add the `Register` function to the `commandInteractions` slice in `main.go`

## Configuration

Config is loaded from `config.toml` (see `config.template.toml` for structure). Search paths: `/etc/heimdallr/`, `~/.heimdallr/`, `~/.config/heimdallr/`, `$XDG_CONFIG_HOME/heimdallr/`, `./`. Environment variables use `HEIMDALLR_` prefix with `_` replacing `.` (e.g., `HEIMDALLR_BOT_TOKEN`).

Dev mode (`dev_mode.enabled = true`) registers commands to a single guild for faster iteration.

## Testing Conventions

- Uses `stretchr/testify` for assertions and mocks
- Table-driven tests are the standard pattern
- Mock implementations for bot/REST clients exist in test files

## Command Removal Utility

`go run . -rm-global-commands` removes all global commands; `go run . -rm-guild-commands <guild_id>` removes guild-specific commands.
