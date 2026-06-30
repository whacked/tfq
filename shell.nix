# Compatibility shim: `nix-shell` enters the flake's devShells.default, so the
# dev environment is defined once in flake.nix. flake-compat is pinned via
# flake.lock; fetchTarball's sha256 == the NAR hash recorded there.
(import
  (
    let
      lock = (builtins.fromJSON (builtins.readFile ./flake.lock)).nodes.flake-compat.locked;
    in
    fetchTarball {
      url = "https://github.com/edolstra/flake-compat/archive/${lock.rev}.tar.gz";
      sha256 = lock.narHash;
    }
  )
  { src = ./.; }
).shellNix
