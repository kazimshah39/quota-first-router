# Quota First Router project memory

## Live preview before release

- After each change to the plugin or monitor, run the focused checks, then deploy a versioned preview to the locally installed live CLIProxyAPI. Verify that it loaded and that unrelated settings are unchanged. Keep the previous library and rollback details; never overwrite a loaded native library.
- Tell the user where to review it in the UI. Do not tag, publish a GitHub release, or update the public store until the user confirms that they manually verified the preview and explicitly approves the release.

## Keep dashboard help current

- The **How it works** drawer is embedded in `web/monitor.html` and ships with the same plugin build as the dashboard. Keep it in sync with `docs/monitor.md` in the same change whenever routing, warmup, filters, labels, or refresh behavior changes. Do not fetch help from an external site or add settings to the monitor.
- Use plain words, the labels users actually see, and a short everyday example for each topic. Explain what the plugin can prove: **Selected** is a plugin pick, not proof of which account served the request; **Last read** is the latest successful quota check across monitored accounts, not when the page loaded or when every account was checked.
- The guide must be available without access to private diagnostics and must not request quota, inference, or account data when opened. Keep it keyboard-accessible, closable with Escape, and usable on small screens and in both themes. Test the guide when changing its behavior or layout.
