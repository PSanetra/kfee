{
  description = "kfee, a Kafka terminal client, a fork of ktea";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = "git-${self.shortRev or self.dirtyShortRev or "unknown"}";
    in
    {
      packages = forAllSystems (pkgs: rec {
        kfee = pkgs.callPackage ./nix/package.nix { inherit version; };
        default = kfee;
      });

      overlays.default = final: _prev: {
        kfee = final.callPackage ./nix/package.nix { inherit version; };
      };

      nixosModules.default = import ./nix/module.nix [ "environment" "systemPackages" ] self;
      homeManagerModules.default = import ./nix/module.nix [ "home" "packages" ] self;

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            protobuf
            vhs
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
