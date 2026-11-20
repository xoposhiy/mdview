# Local delivery

After a change to mdview's behavior, run `./scripts/release.ps1` before handing
the result back to the user. The script checks and installs the build and lets
open windows switch to it. Use `-CheckOnly` for a build and startup check when
the user asks for verification without an install.

See [local releases](README.md#local-releases) for the install path and window
update behavior.
