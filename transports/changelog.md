## Unreleased

- Fixed the Trustloop image build to include the committed `go.work` workspace in the Docker context so transport builds resolve local `core` and plugin modules instead of published module versions.
- Aligned stale call sites and schema field names across `core`, transport handlers, and bundled plugins so the Trustloop runtime image builds successfully again.
