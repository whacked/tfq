{
  description = "tfq — text-file query tool: query/validate/write frontmatter'd text files as a typed graph";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

    # Backs the shell.nix shim so `nix-shell` reuses this flake's devShell.
    flake-compat = {
      url = "github:edolstra/flake-compat";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f:
        nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));

      # The Makefile derives yyyymmdd.<nth-commit-of-day>.<shorthash> from git,
      # but a pure flake build has no .git. Derive a meaningful version from
      # flake metadata instead; degrades to "<date>.dev" on a dirty tree.
      version =
        let
          date = builtins.substring 0 8 (self.lastModifiedDate or "00000000");
          rev = self.shortRev or self.dirtyShortRev or "dev";
        in "${date}.${rev}";
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "tfq";
          inherit version;
          src = ./.;
          # Hash of the vendored Go module deps. Recompute after go.mod/go.sum
          # changes with: nix build .# 2>&1 | grep 'got:'
          vendorHash = "sha256-QuYcUvVaXjvZo6ba0lycjLvyi0hSAN8+m7uUdQ8lYs0=";
          subPackages = [ "cmd/tfq" ];
          ldflags = [ "-X main.version=${version}" ];

          nativeBuildInputs = [ pkgs.makeWrapper ];
          # tfq shells out to ripgrep; the test suite exercises search.
          nativeCheckInputs = [ pkgs.ripgrep ];

          # tfq execs `rg` (internal/search). Guarantee it's reachable at
          # runtime, but let a user's own ripgrep on PATH take precedence.
          postInstall = ''
            wrapProgram $out/bin/tfq \
              --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.ripgrep ]}
          '';

          meta = {
            description = "Query/validate/write frontmatter'd text files as a typed graph";
            homepage = "https://github.com/whacked/tfq";
            mainProgram = "tfq";
          };
        };
      });

      apps = forAllSystems (pkgs:
        let system = pkgs.stdenv.hostPlatform.system;
        in {
          default = {
            type = "app";
            program = pkgs.lib.getExe self.packages.${system}.default;
            meta.description = "Run the tfq CLI";
          };
        });

      # Single definition of the dev environment. shell.nix is a flake-compat
      # shim onto this, so `nix develop` and `nix-shell` are identical.
      devShells = forAllSystems (pkgs:
        let
          nix_shortcuts = import (pkgs.fetchurl {
            url = "https://raw.githubusercontent.com/whacked/setup/ce9fe9be8e42db9ce003772099d08395358efe8c/bash/nix_shortcuts.nix.sh";
            hash = "sha256-uK+Fgwr6iWXbfi/itJGELzkWqGZsQ8HFpfc+ztGSF98=";
          }) { inherit pkgs; };
        in
        {
          default = pkgs.mkShell {
            name = "tfq-dev";

            buildInputs = with pkgs; [
              nodejs
              go
              pkg-config
            ]
            ++ nix_shortcuts.buildInputs;

            shellHook = nix_shortcuts.shellHook + ''
              echo-shortcuts ${__curPos.file}
            '';
          };
        });
    };
}
