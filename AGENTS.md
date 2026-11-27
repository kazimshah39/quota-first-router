# Quota First Router project memory

## Keep dashboard help current

- The **How it works** drawer is embedded in `web/monitor.html` and ships with the same plugin build as the dashboard. Keep it in sync with `docs/monitor.md` in the same change whenever routing, warmup, filters, labels, or refresh behavior changes. Do not fetch help from an external site or add settings to the monitor.
- Use plain words, the labels users actually see, and a short everyday example for each topic. Explain what the plugin can prove: **Selected** is a plugin pick, not proof of which account served the request; **Updated** is when the page loaded its saved readings, not when the provider was checked.
- The guide must be available without access to private diagnostics and must not request quota, inference, or account data when opened. Keep it keyboard-accessible, closable with Escape, and usable on small screens and in both themes. Test the guide when changing its behavior or layout.
