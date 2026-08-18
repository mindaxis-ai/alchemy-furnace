# Desktop release

Desktop releases use one packaging chain locally and in GitHub Actions:

1. Next.js exports static assets for Go embedding.
2. `build-python-runtime.sh` builds a target-native, self-contained Python
   runtime and copies the engine source into it.
3. Wails builds `AlchemyFurnace.app` or `AlchemyFurnace.exe` with release
   metadata injected through Go linker flags.
4. `ci-assemble.sh` installs the runtime, signs and packages the macOS bundle,
   or builds the Windows NSIS installer.
5. The release job checks the complete asset contract, writes SHA256 sums, and
   uploads everything to the tag's GitHub Release.

## Automatic release

Create and push a semantic-version tag:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The `Release desktop` workflow can also be dispatched manually for an existing
tag. It is callable from another workflow through `workflow_call`, with `tag`
as its input.

## Local package

Run on the target architecture so Python wheels and native Wails libraries
match the package:

```bash
make desktop-package PLATFORM=darwin-arm64 VERSION=v0.2.0
make desktop-package PLATFORM=darwin-amd64 VERSION=v0.2.0
make desktop-package PLATFORM=windows-amd64 VERSION=v0.2.0
```

Required tools are Go, Node.js, pnpm, and Wails. Windows additionally needs
NSIS; macOS uses the system `codesign`, `hdiutil`, and `ditto` tools.

## Signing

macOS bundles are ad-hoc signed after the Python runtime is assembled. This is
important: modifying `Contents/Resources` after signing invalidates the bundle.
Set `MACOS_SIGN_IDENTITY` only on a runner where that identity and its private
key are already installed. Apple notarization and Windows Authenticode signing
remain credential-dependent deployment steps.
