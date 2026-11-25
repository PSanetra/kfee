# Agent instructions for kfee

## Dependencies

Whenever `go.mod` or `go.sum` change, the Nix package needs a new `vendorHash`, otherwise `nix build` fails with a
hash mismatch. Do this in the same change as the dependency update:

1. Set `vendorHash` in `nix/package.nix` to `lib.fakeHash`.
2. Run `nix build .#kfee --no-link`. Nix reports the correct hash as `got: sha256-...`.
3. Set `vendorHash` to that hash.
4. Run `nix build .#kfee --no-link` again and check that it exits with 0.

Commit the `vendorHash` change together with the `go.mod` and `go.sum` changes.

## Checks before a commit

- `gofmt -l .` prints nothing.
- `CGO_ENABLED=0 go build ./...` succeeds. The machine may have no C compiler, so `CGO_ENABLED=0` is used.
- `CGO_ENABLED=0 go test -count=1 ./...` passes.

## Commits

- Use conventional commit messages (`feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`).
- Keep each commit buildable and its tests passing.
