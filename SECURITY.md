# Security and support

The 0.2.0 monitor adds public generic static resources and a management-authenticated private snapshot endpoint. Account labels/history may be personal data. It stores only bounded allowlisted diagnostics in memory, never prompts/tokens/raw headers/provider bodies. Entered fallback keys stay in page memory; Forget/401/403 clears private DOM and no new credential persistence is added. Host-saved login remains host-owned. Details and release gates: docs/monitor.md.

Native plugins run inside CLIProxyAPI with access to credentials and traffic. Verify the release checksum and load only builds you trust. OAuth tokens are used only for fixed quota GET endpoints, never logged by this plugin, stored in Git, or refreshed independently.

Report a suspected secret leak or security defect privately through GitHub's **Report a vulnerability** option when enabled, or contact the repository owner without posting credentials or personal request content. Use public issues for non-sensitive bugs with a redacted configuration, CPA version, platform, and plugin version.

Versions 0.1.0/0.1.1 are Linux-amd64 only. Version 0.1.2 passed native CPA 8.1.0 acceptance on all six Darwin/Linux/Windows × amd64/arm64 targets; exact tested hosts and runtime boundaries are in docs/platform-support.md. Monitoring remains paused and is not included. Native acceptance uses synthetic credentials and temporary trust only on disposable runners, with trust removed on cleanup. Provider quota endpoints are implementation details, not a stable public contract. Updates may be needed when they change. See README and docs/operations.md for transport limitations, shadow mode, and rollback.
