# Local CMS Playwright runner

Run this from the repository root or this directory. Install dependencies and
the Chromium browser once:

```sh
cd scripts/local-pilot/playwright
npm ci
npx playwright install chromium
```

Start the CMS separately, then run the checks:

```sh
CMS_BASE_URL=http://127.0.0.1:42803 npm test
```

The runner checks `/health`, the staff admin page, and the storefront product
list. If `CMS-TEST-CREDENTIALS.txt` exists at the repository root, it signs in
with that local test owner and verifies the authenticated dashboard. The file
must remain private; credential-based login is restricted to loopback URLs.
Alternatively, provide `CMS_TEST_EMAIL` and `CMS_TEST_PASSWORD` in the
environment, or set `CMS_TEST_CREDENTIALS` to another local credentials file.

For the in-memory demo, where staff authentication is intentionally unavailable,
skip the login check:

```sh
CMS_BASE_URL=http://127.0.0.1:8081 npm test -- --no-login
```

Use `npm run test:headed` to show the browser, and append `-- --base-url URL`
to override `CMS_BASE_URL` for a single run.