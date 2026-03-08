{
  description = "A simple Go package";
  inputs = {
    nixpkgs.url = "nixpkgs/nixos-25.11";
    utils.url = "github:numtide/flake-utils";
  };
  outputs = { nixpkgs, utils, ... }:
    utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.android_sdk.accept_license = true;
          config.allowUnfreePredicate = pkg:
            builtins.elem (pkgs.lib.getName pkg) [
              "vscode"
              "android-sdk-build-tools"
              "android-sdk-cmdline-tools"
              "android-sdk-platform-tools"
              "android-sdk-platforms"
              "android-sdk-tools"
              "platform-tools"
              "platforms"
              "build-tools"
              "tools"
              "cmake"
              "android-sdk-ndk"
              "ndk"
              "cmdline-tools"
            ];
        };
      in {
        devShell = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            go-tools # Static linting
            gotools # More tools like call graphs
            # gomobile
            # vscode
          ];
        };
      });
}
