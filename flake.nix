{
  inputs = {
    flake-utils = {
      url = "github:numtide/flake-utils";
    };

    nixpkgs = {
      url = "github:NixOS/nixpkgs/nixos-unstable";
    }; 
  };

  outputs =
    {
      self,
      flake-utils,
      nixpkgs,
    }:
    
    flake-utils.lib.eachDefaultSystem (
      system:

      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
      in
        {
          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              btrfs-progs
              bzip2
              cfssl
              cloud-utils
              codex
              google-chrome
              curl
              dasel
              dosfstools
              gettext
              go
              go-containerregistry
              protobuf
              protoc-gen-go
              protoc-gen-go-grpc
              gptfdisk
              jq
              ko
              kubectl
              kubernetes-helm
              kustomize
              libxml2
              nodejs
              opencode
              openssl
              opentofu
              shellcheck
              tpm2-pkcs11
              tpm2-tools

              yamllint

              # Required by the Kustomize developer agent.
              bazel
              bazel-buildtools
              conftest
              git
              gnumake
              go-task
              golangci-lint
              gotools
              gitleaks
              just
              kpt
              kubeconform
              starlark

              # Optional language servers and additional validation tools.
              chart-testing
              gopls
              helm-ls
              kind
              kube-linter
              kustomize-lint
              starpls
              prek
              yaml-language-server
              
              # Optional container runtime for container-backed KRM functions. Choose one.
              podman

              # Github Shit
              gh
              
            ];
          };
          
        }
    );
}
