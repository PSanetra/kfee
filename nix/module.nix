# programs.kfee for NixOS (environment.systemPackages) and Home Manager (home.packages)
packagesAttr: self:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.kfee;
in
{
  options.programs.kfee = {
    enable = lib.mkEnableOption "kfee, a Kafka terminal client";
    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "kfee.packages.\${pkgs.stdenv.hostPlatform.system}.default";
      description = "The kfee package to install.";
    };
  };

  config = lib.mkIf cfg.enable (lib.setAttrByPath packagesAttr [ cfg.package ]);
}
