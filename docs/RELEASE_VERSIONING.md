# Release Versioning

Omnidex uses pride release codenames based on National Dex order.

| Release | Codename | National Dex | Meaning |
| --- | --- | ---: | --- |
| `v0.1.0-alpha` | Bulbasaur | 001 | First alpha release. |
| `v0.2.0` | Ivysaur | 002 | Current growth release. |
| future maturity release | Venusaur | 003 | Reserved for the point where Omnidex is consistently strong, autonomous, and successful enough to deserve the mature codename. |

Notes:
- Use the official spelling `Venusaur` for the mature release codename.
- The release codename should be embedded in binaries through `internal/version` and `scripts/build-release.sh`.
- Patch releases keep the same codename unless the release meaning changes substantially.
- Major maturity jumps should follow the National Dex progression instead of arbitrary codenames.
