## Unreleased

- Fixed the Trustloop runtime image to build bundled dynamic plugins (`tool-injector`, `openrouter-provider-routing`) with the repository workspace enabled, so they stay ABI-compatible with the locally-built `bifrost-http` binary instead of drifting to published module versions.
- Fixed the Trustloop image build to include the committed `go.work` workspace in the Docker context so transport builds resolve local `core` and plugin modules instead of published module versions.
- Aligned stale call sites and schema field names across `core`, transport handlers, and bundled plugins so the Trustloop runtime image builds successfully again.
- Reduced the Trustloop GitHub publication workflow to `linux/amd64` only because the deployment target does not require multi-architecture images and the multi-arch Buildx push was hanging in GitHub Actions.
