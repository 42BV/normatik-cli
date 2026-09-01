# Normatik CLI

Normatik CLI is a command-line client for the Normatik Public API. It supports
interactive browser login, API-key based automation, JSON output, shell
completion, and discoverable error recovery hints.

This repository is the public source mirror for released CLI versions. The
release process validates the source before every export; development happens
upstream and this mirror is not a bidirectional contribution workflow.

## Install

With Homebrew:

```bash
brew install 42bv/tap/normatik
```

Or install a released version directly with Go:

```bash
go install github.com/42BV/normatik-cli/cmd/normatik@latest
```

## Get started

```bash
normatik login --url https://wiki.example/ --browser
normatik whoami
normatik pages list
```

For automation, provide both values explicitly:

```bash
export NORMATIK_BASE_URL=https://wiki.example/api
export NORMATIK_API_KEY=wiki_...
normatik pages list --output json
```

The CLI stores an interactively entered key in the operating-system keychain;
it never writes API keys to `config.toml`. HTTPS is required except for an
explicit localhost-only development opt-in.

## Build from source

Requirements: Go 1.26.7 or newer.

```bash
go mod download
go build -trimpath -o normatik ./cmd/normatik
./normatik --version
```

The public source is a minimal runtime mirror: it includes the generated API
client and runtime catalogue needed to build and run the CLI. Generation,
drift validation, unit tests and integration tests remain private release
gates and are performed before source is exported.

## Shell completion

```bash
normatik completion bash
normatik completion zsh
normatik completion fish
```

## License

Copyright (c) 2026 42 B.V. Licensed under the [MIT License](LICENSE).
