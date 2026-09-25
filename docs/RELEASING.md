# Releasing

Releases are built and published by GitHub Actions (`.github/workflows/release.yml`) when a version tag is
pushed. SWIRL Card Manager finds new releases through the GitHub API and can install them itself.

## Version numbers

| Where | Example | Meaning |
|---|---|---|
| `ui/swirl/sw_version.h` | `2.11` | The menu version, shown in System > About SWIRL |
| `swirl/cardmanager/main.go`, `const version` | `2.11.0` | The app version; its major.minor must match the menu |
| The git tag | `v2.11.0` | Must equal the app version |

A Card Manager only fix is a patch release (`2.11.1`) and does not need a menu change. Any menu change bumps
the minor version (`2.12` / `2.12.0`).

## Making a release

1. Update the version numbers above. If the menu changed, build it and copy it in:
   ```sh
   swirl/build.sh
   cp swirl/build/gdemu/1ST_READ.BIN swirl/cardmanager/assets/1ST_READ.BIN
   ```
2. Add a section for the version to `CHANGELOG.md` (`## [2.12.0]`). It becomes the release notes and is shown in Card Manager's **What's new**.
3. Commit and push, and wait for CI to pass.
4. Tag and push the tag:
   ```sh
   git tag v2.12.0
   git push origin v2.12.0
   ```
   In GitHub Desktop: **History**, right click the commit, **Create Tag...**, then **Push origin**.

The workflow then checks the version numbers, builds the menu from source, runs the tests, builds
`SWIRL-Card-Manager.exe` with its version info, builds the Mac app on a GitHub Mac (the patched Flycast is
cached after its first build, which takes about 20 minutes), and publishes a release with:

| Asset | What |
|---|---|
| `SWIRL-Card-Manager.exe` | The Windows app, with the menu inside |
| `SWIRL-Card-Manager-macOS.dmg` | The Mac app (Apple Silicon and Intel), to drag into Applications |
| `SWIRL-Card-Manager-macOS.zip` | The same Mac app, for the in-app updater |
| `1ST_READ.BIN` | The menu on its own, for people who build menu discs with other tools |
| `SHA256SUMS.txt` | Checksums of all of them |

If something goes wrong, fix it, move the tag and run the workflow again from the **Actions** tab (**Run
workflow**, give the tag); it replaces the assets of an existing release.

## How the in app updater works

1. Once a day (or when you click **Check for updates**), Card Manager reads
   `https://api.github.com/repos/<owner>/<repo>/releases/latest`. Drafts and pre releases are ignored by that endpoint.
2. If the release's version is newer, it shows a bar with **What's new** (the release notes) and **Update now**.
3. **Update now** downloads `SWIRL-Card-Manager.exe` (on a Mac, `SWIRL-Card-Manager-macOS.zip`) and checks its SHA-256 against the value GitHub publishes for the asset, or against `SHA256SUMS.txt`. A mismatch stops the update.
4. The new version waits for the running copy to close, installs itself over the installed copy (on Windows keeping the desktop shortcut choice; on a Mac replacing the app in Applications) and starts.

Checks are anonymous; GitHub allows 60 per hour per internet connection, far more than a daily check needs.

## Forks

The repository the app checks is set at build time. The workflows pass
`-X main.updateRepo=${{ github.repository }}`, so a fork's builds look for the fork's releases and never
offer the original project's builds as updates. A local build checks `TheGlengineer/SWIRL` unless you set
`SWIRL_UPDATE_REPO`.
