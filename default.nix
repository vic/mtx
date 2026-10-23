{ pkgs ? import <nixpkgs> { } }:
pkgs.buildGoModule {
  pname = "mtx";
  version = "0.1.0";
  src = pkgs.lib.fileset.toSource {
    root = ./.;
    fileset = pkgs.lib.fileset.unions [ ./cmd ./internal ./go.mod ./go.sum ];
  };
  vendorHash = "sha256-+dX9/n2GOtYnS07LAsRWkx5Q9dpo//S5klvR5ea7bDg="; # recompute on first nix build (needs approval)
  env.CGO_ENABLED = 0;
  subPackages = [ "cmd/mtx" ];
  tags = [ "goolm" ];
}
