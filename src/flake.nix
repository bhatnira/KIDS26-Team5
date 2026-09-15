# Nix flake for Antelope — an AI-augmented Nextflow pipeline management platform
# (Go/Gin backend + Vue 3 frontend, dispatched to HashiCorp Nomad).
#
# Prerequisites:
#   Install Nix:   https://nixos.org/download/
#   Enable flakes: https://nixos.wiki/wiki/Flakes#Enable_flakes
#
# Usage:
#   nix build              # Build the self-contained server (embedded UI) + CLI
#   nix run                # Build and run `antelope`
#   ./result/bin/antelope --help
#   ./result/bin/antelope-cli --help
#   nix build .#docker     # Build an OCI image tarball (mirrors the Dockerfile)
#   docker load < result   # …then load it into Docker/Podman
#   nix develop            # Drop into the dev shell (Go + pnpm toolchain)
#   nix flake check        # Build + run pre-commit (nixfmt / statix / deadnix)
#   nix fmt                # Format every .nix file with nixfmt
#
# ── First-build bootstrap (filling in the two placeholder hashes) ─────────────
# Two fixed-output hashes can only be known after a fetch attempt. They start as
# `lib.fakeHash`; Nix fails the first build and prints the correct value, which
# you paste back in. Do this once (and again whenever the matching lockfile
# changes):
#
#   1. pnpmDeps.hash  — changes when web_src/pnpm-lock.yaml changes.
#   2. vendorHash     — changes when go.mod / go.sum change.
#
# Run `nix build` and replace each `lib.fakeHash` with the "got:" hash Nix
# reports, one at a time (pnpmDeps is fetched first, vendorHash second).
{
  description = "Antelope — AI-augmented Nextflow pipeline management platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    pre-commit-hooks = {
      url = "github:cachix/git-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      pre-commit-hooks,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        inherit (pkgs) lib;

        # Single source of truth for the package version. There are no git tags
        # yet, so this is set by hand here (mirrors VERSION in the Makefile, which
        # falls back to `git describe`). Bump on release.
        version = "0.1.0";

        # Toolchain — bump versions here only.
        #   go.mod requires Go 1.25.x; package.json wants Node ^20.19 || >=22.12.
        go = pkgs.go_1_25;
        nodejs = pkgs.nodejs_22;
        inherit (pkgs) pnpm; # pnpm 10 on nixos-unstable; lockfile is v9 → fetcherVersion 3

        # buildGoModule bakes in nixpkgs' *default* Go (currently 1.26.x), and it
        # prepends that to nativeBuildInputs — so pinning `go` only via
        # nativeBuildInputs gets shadowed on PATH. Override the builder's own `go`
        # argument to actually control the compiler used for vendoring and build.
        buildGoModule = pkgs.buildGoModule.override { inherit go; };

        # Shared, filtered source tree. Strips build outputs, dependency caches,
        # editor state, and the large .docx proposals so they don't bust the
        # input hash (and so the Go build sandbox stays lean).
        src = lib.cleanSourceWith {
          src = ./.;
          filter =
            path: _type:
            let
              base = baseNameOf (toString path);
              rel = lib.removePrefix (toString ./. + "/") (toString path);
            in
            !(
              base == "result"
              || lib.hasPrefix "result-" base
              || base == ".direnv"
              || base == ".idea"
              || base == ".git"
              || base == "bin"
              || base == "dist"
              || base == "node_modules"
              || rel == "web_src/dist"
              || rel == "web_src/node_modules"
              || lib.hasSuffix ".docx" base
            );
        };

        # ── Frontend: compile the Vue app to web_src/dist with pnpm ────────────
        # Built as its own derivation so its (heavy) pnpm dependency closure is
        # fetched and cached independently of the Go build.
        frontend = pkgs.stdenv.mkDerivation (finalAttrs: {
          pname = "antelope-frontend";
          inherit version src;

          # The Vite project lives in web_src/; build from there.
          sourceRoot = "${finalAttrs.src.name}/web_src";

          pnpmDeps = pkgs.fetchPnpmDeps {
            inherit (finalAttrs)
              pname
              version
              src
              sourceRoot
              ;
            fetcherVersion = 3;
            # Bootstrap: replace with the hash Nix prints on the first build.
            hash = "sha256-w0smmgR7G3PjzovIuYQjDoC11LhgdlsVLWBBhD4RXcY=";
          };

          nativeBuildInputs = [
            nodejs
            pnpm
            pkgs.pnpmConfigHook
          ];

          buildPhase = ''
            runHook preBuild
            pnpm run build
            runHook postBuild
          '';

          installPhase = ''
            runHook preInstall
            mkdir -p $out
            cp -R dist $out/dist
            runHook postInstall
          '';
        });

        # ── Backend: build the embedded server + CLI with buildGoModule ────────
        antelope = buildGoModule {
          pname = "antelope";
          inherit version src;

          # Bootstrap: replace with the hash Nix prints on the first build.
          vendorHash = "sha256-XLEVm0pyDyi1ax9zh+BnNDHRGBCRnPvFERlMlKu19nM=";

          # Pure-Go dependency tree (postgres/minio/go-git/etc.); no cgo needed.
          env.CGO_ENABLED = 0;

          nativeBuildInputs = [ go ];

          # The Makefile builds two binaries with *different* tags and ldflag
          # symbols (server: main.Version/main.Tags with the `embed,timetzdata`
          # tags; CLI: main.version, no embed). That's awkward to express with
          # buildGoModule's single `tags`/`ldflags`, so we drive both go builds
          # by hand. buildGoModule's installPhase then copies $GOPATH/bin → $out.
          buildPhase = ''
            runHook preBuild

            # Stage the compiled frontend where //go:embed all:web_src/dist looks.
            rm -rf web_src/dist
            cp -R ${frontend}/dist web_src/dist

            echo "building antelope (server, embedded UI)…"
            go build \
              -tags "embed,timetzdata" \
              -ldflags "-s -w -X main.Version=${version} -X main.Tags=embed,timetzdata" \
              -o "$GOPATH/bin/antelope" .

            echo "building antelope-cli…"
            go build \
              -ldflags "-s -w -X main.version=${version}" \
              -o "$GOPATH/bin/antelope-cli" ./cmd/cli

            runHook postBuild
          '';

          # The server's own tests reach out to Postgres/Redis/Nomad; skip them
          # in the sandbox. A binary smoke test runs under `checks` instead.
          doCheck = false;

          meta = {
            description = "AI-augmented Nextflow bioinformatics pipeline management platform";
            homepage = "https://github.com/HaidYi/antelope";
            license = lib.licenses.mit;
            mainProgram = "antelope";
            platforms = lib.platforms.unix;
          };
        };

        # ── Container image (mirrors the Dockerfile's runtime stage) ──────────
        # The Dockerfile's final stage is just alpine + ca-certificates + tzdata
        # holding the embedded server binary, with `antelope web` as the
        # entrypoint. buildLayeredImage builds the same thing from the Nix
        # package — no Go/pnpm toolchain in the image. Config is supplied at
        # runtime via a bind-mounted config.yaml or ANTELOPE_ env vars.
        dockerImage = pkgs.dockerTools.buildLayeredImage {
          name = "antelope";
          tag = version;

          contents = [
            antelope
            pkgs.dockerTools.caCertificates # /etc/ssl/certs for TLS (OIDC, S3, SMTP)
            pkgs.tzdata
          ];

          config = {
            # Entrypoint is the binary; Cmd defaults to `web` but can be
            # overridden (e.g. `docker run … monitor`) — the server is a single
            # binary with web/monitor subcommands.
            Entrypoint = [ "${antelope}/bin/antelope" ];
            Cmd = [ "web" ];
            WorkingDir = "/app";
            ExposedPorts = {
              "8086/tcp" = { };
            };
          };
        };

        preCommitCheck = pre-commit-hooks.lib.${system}.run {
          src = ./.;
          hooks = {
            nixfmt = {
              enable = true;
              settings.width = 100;
            };
            statix.enable = true;
            deadnix.enable = true;
          };
          excludes = [
            "^web_src/"
            "^vendor/"
            "^go\\.sum$"
          ];
        };
      in
      {
        packages = {
          default = antelope;
          inherit antelope frontend;
          # Build with: nix build .#dockerImage
          # Load with:  docker load < result   (or: podman load < result)
          docker = dockerImage;
        };

        apps.default = {
          type = "app";
          program = "${antelope}/bin/antelope";
        };

        checks = {
          inherit antelope;
          pre-commit-check = preCommitCheck;

          # Confirms the produced server binary links and runs.
          antelope-smoke = pkgs.runCommand "antelope-smoke" { nativeBuildInputs = [ antelope ]; } ''
            antelope --version > $out
          '';
        };

        devShells.default = pkgs.mkShell {
          packages = [
            # Backend toolchain
            go
            pkgs.gopls
            pkgs.gotools # goimports
            pkgs.gofumpt
            pkgs.golangci-lint
            pkgs.delve
            pkgs.go-swag # `make swagger` (provides the `swag` binary)

            # Frontend toolchain
            nodejs
            pnpm

            # Generic build helpers used by the Makefile / Justfile
            pkgs.git
            pkgs.gnumake
            pkgs.just

            # Nix tooling
            pkgs.nixfmt
            pkgs.statix
            pkgs.deadnix
          ];

          # Pin GOROOT to the same toolchain used for the package build.
          env = {
            GOROOT = "${go}/share/go";
          };

          inherit (preCommitCheck) shellHook;
        };

        formatter = pkgs.nixfmt;
      }
    );
}
