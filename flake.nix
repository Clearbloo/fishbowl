{
  description = "A simple Go package";
  inputs = {
    nixpkgs.url = "nixpkgs/nixos-25.11";
    utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, utils, ... }:
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
        lastModifiedDate =
          self.lastModifiedDate or self.lastModified or "19700101";
        version = builtins.substring 0 1 lastModifiedDate;
        pname = "hello";
      in {
        defaultPackage = pkgs.buildGoModule {
          inherit pname version;
          src = ./.;
          vendorHash = pkgs.lib.fakeHash;
        };
        devShell = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
            gomobile
            vscode
          ];
        };
      });
}
