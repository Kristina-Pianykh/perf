{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-25.05";
  };

  outputs =
    { nixpkgs, ... }:
    let
      system = "aarch64-darwin";
      pkgs = nixpkgs.legacyPackages.${system};
    in
    {
      devShells.${system}.default = pkgs.mkShell {
        name = "perf";
        packages = with pkgs; [
          # unix coreutils
          gnumake
          gnutar
          curl

          # go-specific
          golangci-lint
          go

          # misc
          teller
        ];
      };
      formatter.${system} = pkgs.alejandra;
    };
}
