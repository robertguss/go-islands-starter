# Initial release checklist

- [ ] Review the implementation, dependencies, copied-source notices, migrations, grants, and CI configuration.
- [ ] Run the GitHub Actions workflow on the exact release revision, including Socket's clean-install check and generated-source drift.
- [ ] Verify real Google Workspace login, denied domains, unadmitted/disabled users, session revocation, and callback behavior behind the deployed HTTPS proxy.
- [ ] Verify the Render runtime login is restricted and migration credentials are absent; test hosted health, shutdown, and backup restoration.
- [ ] Review retention and privacy boundaries for the actual downstream data.
- [ ] Validate one downstream application before broadening the starter.
- [ ] Tag a release and record the reviewed revision, container digest, acceptance results, and known limits in CHANGELOG.md.

Local acceptance evidence belongs in IMPLEMENTATION.md. These account-backed and release-review checks remain explicit human/operator tasks.
