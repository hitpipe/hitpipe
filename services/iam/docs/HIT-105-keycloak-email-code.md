# HIT-105 · Email verification in Keycloak

The module in `services/iam/keycloak` registers `hitpipe-email-code` as a Keycloak required action. The test realm offers registration from the sign-in screen and requires a code before it finishes the browser flow. Keycloak owns the user, password, and `emailVerified` flag. The realm uses the English login and email theme shipped in the provider JAR.

The provider targets Java 21 and Keycloak 26.8.1. The container build uses the pinned Keycloak distribution and Maven 3.9.11 on JDK 21. The included realm export is for the local test only. It enables registration and the required action, disables the built-in email link and duplicate email addresses, and redirects the test client to its loopback callback. See the [Keycloak extension guide](https://www.keycloak.org/docs/26.8.1/server_development/#_required_action_walkthrough).

The SHA-256 of `src/main/resources/META-INF/keycloak-email.sql` is `50f9fdfa632c7721521e95afd90d8b1fd7a536e1c5845c1062a0d16ef3c0038a`. Keycloak applies it through Liquibase to its own PostgreSQL database. The extension uses the active Keycloak JPA transaction and takes a local user lock before the rate and challenge locks. Verification consumes the challenge and updates `emailVerified` in the same transaction. The browser flow continues once that transaction commits. Issuance commits before sending mail. An SMTP failure still counts toward the recipient's resend limit.

The code has eight decimal digits and is valid for ten minutes. Five wrong attempts lock it. A recipient in the same realm can request a new code after 60 seconds, up to five times in a rolling hour. A newer generation replaces the previous challenge. The database stores keyed digests and timestamps. The code digest binds realm, user, exact email, authentication session, generation and expiry. A recipient digest preserves the resend window across browser flows and code-key rotation. The maintenance task purges expired data after its rate window. User removal deletes the challenge and keeps the recipient's rate window until it expires.

Configure `HITPIPE_EMAIL_CODE_KEY` and `HITPIPE_EMAIL_RATE_KEY` with distinct Base64 values that each decode to at least 32 random bytes. Configure `HITPIPE_EMAIL_CODE_KEY_VERSION` as a positive integer. Advance the version when changing the code key. Keep the rate key separate so the resend limit survives a code-key rotation. Configure SMTP on the realm, select the provider's login and email theme and enable the `hitpipe-email-code-account-events` listener to revoke challenges when an email changes. The application uses its configured Keycloak realm and needs no PostgreSQL account or realm administrator token for this action.

Run the integration checks in a disposable Linux guest with a local Docker engine, Compose, Node.js and the browsers installed by Playwright. From `services/iam/keycloak`, run the following commands.

```sh
npm --prefix tests ci --no-audit --no-fund
npm --prefix tests exec -- playwright install chromium
./tests/run.sh
```

The script builds the provider and starts Keycloak 26.8.1, PostgreSQL 18.7 and a Mailpit SMTP receiver. Database storage is temporary. The browser reaches both web interfaces over loopback and reads the test letter from Mailpit. The script generates fresh lab credentials and removes the containers and credential file on exit. Its results cover the test realm. HitPipe sessions, the production OIDC callback, BFF integration and external mail delivery belong to later work.
