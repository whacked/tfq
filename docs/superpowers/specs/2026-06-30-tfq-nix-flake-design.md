# tfq — Nix flake packaging + unified dev shell

> Status: approved design (via Q&A), pre-implementation.
> Motivation: make tfq installable/runnable through Nix flakes
> (`nix run github:whacked/tfq -- …`, `nix build`, `nix profile install`) and
> provide a `nix develop` dev shell, while keeping the existing interactive
> `nix-shell` workflow working. The two shells are unified so the environment is
> defined once.

## 1. Decision

**flake.nix is the single source of truth** for the package, the runnable app,
and the dev shell. `shell.nix` is rewritten as a thin **flake-compat shim** that
loads the flake's `devShells.default`, so `nix-shell` and `nix develop` produce
the identical environment with no duplicated definition.

(Considered and rejected: keeping `shell.nix` primary with the flake importing
it; and keeping the two definitions independent. Rejected for drift / for not
giving the flake a single owning definition.)

## 2. Files

| File | Change |
|---|---|
| `flake.nix` | **new** — inputs, `packages.default`, `apps.default`, `devShells.default` |
| `shell.nix` | **rewritten** — flake-compat shim → `flake`'s `shellNix` |
| `flake.lock` | **new (generated)** — pins `nixpkgs` + `flake-compat` |
| `Makefile` | unchanged — local `make build` keeps its git-derived version |

## 3. flake.nix

### 3.1 inputs
- `nixpkgs` → `nixpkgs-unstable` (required: go.mod declares `go 1.25.5`; the
  default `go` in unstable must be ≥ 1.25.5, else pin `go_1_25`).
- `flake-compat` → backs the `shell.nix` shim.

### 3.2 outputs (per-system)
Evaluated over `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`,
`aarch64-darwin` via a small inline `forAllSystems` helper (no flake-utils
input — keep inputs to nixpkgs + flake-compat).

- **`packages.default`** = `buildGoModule`:
  - `pname = "tfq"`, `subPackages = [ "cmd/tfq" ]`
  - `src = ./.` (or `self`)
  - `vendorHash` = computed locally and pinned (nix is available on this box)
  - `ldflags = [ "-X main.version=${version}" ]`
  - `meta` (description, homepage, license, mainProgram = "tfq")
- **`apps.default`** = `{ type = "app"; program = "${pkg}/bin/tfq"; }` — enables
  `nix run`.
- **`devShells.default`** = `mkShell` carrying the **current shell.nix
  contents**: `nodejs`, `go`, `pkg-config`, plus the pinned `nix_shortcuts`
  fetch and the `echo-shortcuts` shellHook, so behavior is preserved in both
  `nix develop` and `nix-shell`.

### 3.3 version string
The Makefile derives `yyyymmdd.<nth-commit-of-day>.<shorthash>` from git, but a
pure flake build has no `.git`. The flake instead derives the version from flake
metadata:

```
version = "${date}.${rev}"
  date = substring 0 8 (self.lastModifiedDate or "00000000")   # yyyymmdd
  rev  = self.shortRev or self.dirtyShortRev or "dev"
```

This is meaningful for `nix run`/`nix build` and degrades to `…-dev` on a dirty
tree. The `nth-commit-of-the-day` count is dropped (not reconstructable in the
sandbox); the Makefile path keeps the full format for local builds.

## 4. shell.nix (shim)

```nix
(import (
  let lock = (builtins.fromJSON (builtins.readFile ./flake.lock)).nodes.flake-compat.locked;
  in fetchTarball {
    url = "https://github.com/edolstra/flake-compat/archive/${lock.rev}.tar.gz";
    sha256 = lock.narHash;   # fetchTarball sha256 == NAR hash of unpacked tree == flake.lock narHash
  }
) { src = ./.; }).shellNix
```

## 5. Entry-point behavior after this change

| Command | Result |
|---|---|
| `nix run github:whacked/tfq -- <args>` | build & run tfq |
| `nix run .# -- <args>` | same, from local checkout |
| `nix build .#` / `nix profile install .#` | build/install the `tfq` binary |
| `nix develop` | dev shell (go, node, pkg-config, shortcuts) |
| `nix-shell` | same dev shell via the shim |

## 6. Gotchas

- **git visibility**: flakes only see git-tracked files. `flake.nix`,
  `shell.nix`, `flake.lock` must be `git add`ed (staged is enough) before
  `nix develop`/`nix build` evaluate. Staging is part of the work.
- **repo name**: remote is `whacked/tfq`, so the public command is
  `nix run github:whacked/tfq` (the user's original `…/text-file-query` was the
  long name). flake.nix is name-agnostic; this only affects README prose.
- **go version**: if unstable's default `go` lags behind 1.25.5, pin
  `go = pkgs.go_1_25;` in `buildGoModule`.

## 7. Verification (definition of done)

1. `nix build .#` succeeds.
2. `nix run .# -- --version` prints the injected `yyyymmdd.<rev>` version.
3. `nix develop -c tfq --help` (or `-c go version`) works.
4. `nix-shell --run 'echo ok'` enters via the shim.
5. `nix flake check` passes.
