# AGENTS.md — horizonx

Go API + agent. Contract is short on purpose: only rules that change behaviour.

## Commands

```bash
make check        # fmt-check + vet + lint + test, the done criterion
make test         # go test ./... -count=1
make test-unit    # targeted unit tests
make race         # race detector
make lint         # golangci-lint
make fmt          # gofmt -w ./cmd ./internal
make build        # binaries into bin/
```

## Invariants

1. Never edit generated mocks by hand. Change the interface, run `make mocks`.
2. Never weaken a test to pass `make check`. Deleting an assertion is a failure, not a fix.
3. No new dependency without a decision record in the workspace and approval.
4. Migrations ship with an up and a down path. No manual schema edits.
5. Nothing is pushed. Local commits only.
6. Local dev port is 4958. Deployed ports 4858/4859 belong to the installer, never to dev.

## Branching

`feature/` or `fix/` off `develop`. Never commit to `main` or `develop`.
Commits: `type(scope): summary`.

## Done

`make check` exits 0. Report the command and its exit code.
