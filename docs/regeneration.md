# Regeneration pipeline

`twoctl` is rebuilt from the OpenAPI specs in [`two-inc/docs`](https://github.com/two-inc/docs/tree/main/openapi). The flow:

```
docs/openapi/*.yaml change
        |
        v  (push to main, paths: openapi/**)
[notify-twoctl.yml in docs] --- repository_dispatch ----> [regenerate.yml in twoctl]
                                                                  |
                                                                  v
                                                          1. fetch latest specs
                                                          2. ./scripts/codegen.sh
                                                          3. go build/vet
                                                          4. open PR (peter-evans)
```

A nightly cron in `regenerate.yml` is the safety net if a dispatch ever gets dropped.

## Wiring it up

### One-time setup in `two-inc/docs`

1. Create a fine-grained PAT scoped to `two-inc/twoctl-cli` with `actions: write`. Store as `TWOCTL_DISPATCH_TOKEN`.
2. Copy [`notify-twoctl.yml.example`](notify-twoctl.yml.example) into `.github/workflows/notify-twoctl.yml`.

### One-time setup in `two-inc/twoctl-cli`

The `regenerate.yml` workflow uses `GITHUB_TOKEN` for PR creation. No extra secret needed unless you want it to read private docs (override `DOCS_READ_TOKEN`).

### Local regeneration

```sh
./scripts/codegen.sh
```

That preprocesses the OpenAPI 3.1 specs into 3.0-compatible shape (see [`internal/preprocess`](../internal/preprocess/main.go)) and copies the processed specs into `cmd/twoctl/cli/specs/` for embedding. The CLI parses the embedded spec at startup and walks every operation; no typed Go client code is generated, which keeps the dependency surface small and the Go Report Card clean.

## Releases

Push a `vX.Y.Z` tag. `release.yml` runs `goreleaser` which produces darwin/linux/windows binaries and updates the `two-inc/homebrew-tap` Brewfile (requires `HOMEBREW_TAP_GITHUB_TOKEN` repo secret).
