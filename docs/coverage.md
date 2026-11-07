# Software listed by we-have-a-problem.com

The site we-have-a-problem.com lists, under
[Who has patched?](https://emudhra.we-have-a-problem.com/#developers), the
software whose publishers have released a fix. This page says how eMudScan
handles each entry on Windows. It follows that list as of 31 October 2026.

- **By version:** eMudScan recognizes the software and compares its version
  with the fixed one.
- **By contents:** eMudScan tests what the software contains (its certificate
  bundle, root store or block list), whatever its version.
- **By contents, then version:** eMudScan tests the root store built into the
  program and, when it still holds an affected root, compares the program’s
  version with the fixed one.
- **Checked by Windows:** on Windows, the software asks Windows to check
  certificates, so the Windows block list protects it, and eMudScan checks
  Windows.
- **Not on a Windows PC:** an operating system, a server or a cloud service.
- **Not covered:** eMudScan does not check it, and says so in the scan.
- **Fix pending:** no fixed version yet.

## Operating systems and distributions

Not on a Windows PC: Alma Linux, Alpine Linux, Amazon Linux, Arch Linux,
Bottlerocket, Chainguard, Debian, Devuan, Fedora, FreeBSD (ca_root_nss and
caroot), Freedesktop SDK, Gentoo, GNU Guix, Homebrew, NetBSD, OmniOS, OpenBSD,
OpenHarmony and HarmonyOS NEXT, openSUSE Leap and Tumbleweed, Oracle Linux,
RHEL, Rocky Linux, SLES, SteamOS and Ubuntu.

Linux distributions installed in WSL keep their own root store. When WSL is
installed, eMudScan shows **Not covered by this scan**, with the command that
updates them.

## Root store bundles and libraries

| Software | eMudScan |
|---|---|
| Cloudflare CFSSL trust store | Not on a Windows PC: used to build certificate chains for servers. |
| Composer ca-bundle | By contents: its `cacert.pem` is tested like every certificate bundle. |
| curl CA bundle | By contents: every `cacert.pem` is tested, and curl is checked with the bundle it uses. |
| Elixir castore | By contents: its `cacerts.pem` is tested like every certificate bundle. |
| Erlang certifi | By contents: its `cacerts.pem` is tested like every certificate bundle. |
| Kubernetes cert-manager | Not on a Windows PC: runs in a Kubernetes cluster. |
| NSS | Firefox and Thunderbird receive Mozilla’s block list (OneCRL). eMudScan reads the one included in other Firefox-based browsers. |
| Perl Mozilla::CA | By contents: its `cacert.pem` is tested like every certificate bundle. |
| Python certifi | By version and by contents, in every Python environment. |
| Ruby excon | By contents: its `cacert.pem` is tested like every certificate bundle. |
| Rust rustls-webpki | By contents: the block list that version 0.103.16 adds, and the root store of the program that uses it (usually webpki-roots). |
| Rust webpki-roots and webpki-root-certs | By contents: the root store built into the program. |

## Language runtimes

| Software | eMudScan |
|---|---|
| Bun | By contents, then version. |
| Dart and Flutter | By version: the Dart SDK and the Flutter SDK (Dart 3.13.7 and Flutter 3.47.8 or later are fixed), or by the block list in `dart.exe`. Apps built with Flutter by contents: the block list in their engine (`flutter_windows.dll`). Programs compiled with `dart compile exe` are not recognized. |
| Deno | By contents, then version. |
| Electron | By version, in every Electron application. |
| Go | By version, for each Go installation. Programs built with Go ask Windows to check certificates, unless they carry their own root store, which eMudScan tests. |
| Java | By contents: the trusted certificates (`cacerts`) and the block list (`blocked.certs`) of every Java, including Oracle JDK, OpenJDK and the JetBrains Runtime, and copies inside other software. |
| Node.js | By version, including copies inside other software. |
| Python | By version, and by the block list of its `ssl` module. |

## Applications and tools

| Software | eMudScan |
|---|---|
| ArgoCD | Checked by Windows: its command-line tool is a Go program, which asks Windows by default. The server is not on a Windows PC. |
| Bazel | By contents: the Java that Bazel extracts into its install base the first time a version runs, attributed to Bazel. Before that, this Java is only inside `bazel.exe`, compressed, and is not checked. |
| certbot | Fix pending. |
| Claude Code | By contents, then version. |
| Conda | By contents, then version. |
| containerd | Not on a Windows PC: a container runtime for servers. Fix pending. |
| Docker CLI | By contents, then version. |
| Docker Desktop | By version. |
| Docker Engine | Not covered: it runs in Docker Desktop’s Linux virtual machine. eMudScan shows **Not covered by this scan** when Docker is installed. |
| Git for Windows | By version. |
| GitLab | Not on a Windows PC: a server. |
| GitLab runner | Checked by Windows: a Go program, which asks Windows by default. |
| GitHub CLI | By contents, then version. |
| Godot | Fix pending. |
| gRPC | By contents: its `roots.pem` (in Python’s grpcio package, for example) is tested like every certificate bundle. |
| Immich | Not on a Windows PC: a server. |
| JetBrains IDEs | By contents: the Java runtime they include (JetBrains Runtime). By version: the IDE itself (IntelliJ IDEA, PyCharm, PhpStorm, RubyMine, WebStorm, Rider, CLion, DataSpell, DataGrip, GoLand, RustRover and Gateway), whose fixed versions also fix its built-in browser (JCEF). |
| JetBrains Toolbox App | By contents: the Java runtime it includes (`bin\jre`). Its jetbrainsd service, which keeps its own list of trusted certificates and is installed by the Toolbox App and the IDEs: by contents, then version (0.9.9728). |
| LibreOffice | Fix pending. |
| litellm | By contents: the certifi of its Python environment. |
| mise | By contents, then version. |
| mitmproxy | By contents: its certificate bundle, including the one packed inside its standalone programs when they are in a folder of the PATH. |
| Obsidian | By version. |
| pip | By version and by contents, in every Python environment. |
| pixi | By contents, then version. |
| Plex Media Server | By contents: its `Resources\cacert.pem` is tested like every certificate bundle. When it is outdated, eMudScan recommends updating Plex Media Server. |
| pypi-publish GitHub Action | Not on a Windows PC: runs on GitHub. |
| Rustup | By contents, then version. |
| sigstore | By contents: the certifi of its Python environment. |
| Tailscale | Checked by Windows: a Go program, which asks Windows by default. |
| uv | By contents, then version. |
| Visual Studio Code | By version. |
| yt-dlp | By version, wherever it is on the scanned drives: 2026.10.26 completes the fix that 2026.10.21 began. |

## Keeping this page up to date

That list changes as publishers release fixes (see its
[history](https://github.com/galadran/we-have-a-problem.com/commits/main/)).
When an entry is added or a fixed version changes, update this page with the
fixed versions in `cmd/emudscan/model.go` (`fixRules`), in the same commit.
