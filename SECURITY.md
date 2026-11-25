# Security and support

Native plugins run inside CLIProxyAPI with access to credentials and traffic. Verify the release checksum and load only builds you trust. OAuth tokens are used only for fixed quota GET endpoints, never logged by this plugin, stored in Git, or refreshed independently.

Report a suspected secret leak or security defect privately through GitHub's **Report a vulnerability** option when enabled, or contact the repository owner without posting credentials or personal request content. Use public issues for non-sensitive bugs with a redacted configuration, CPA version, platform, and plugin version.

Version 0.1.0 is verified on CPA 8.1.0, Linux amd64/glibc 2.34+. Provider quota endpoints are implementation details, not a stable public contract. Updates may be needed when they change. See README and docs/operations.md for transport limitations, shadow mode, and rollback.
