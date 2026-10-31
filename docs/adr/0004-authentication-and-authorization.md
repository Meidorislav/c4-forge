# ADR-0004: Authentication and authorization

- **Status:** Accepted
- **Date:** 2026-11-01
- **Related requirements:** AU-1…AU-7, NF-3, NF-10, NF-11, NF-12
- **Builds on:** ADR-0002 (realtime and actor identity), ADR-0003 (workspace data and history)

## Context

A self-hosted instance needs local accounts without an identity provider (AU-1,
NF-3). The first user is an instance administrator. An administrator manages
users (AU-4), and access to model data is scoped to a workspace (AU-3). A
browser session also authenticates the realtime channel (NF-10, NF-11).

OIDC (AU-2), teams (AU-5), commenter and the complete owner role (AU-3), and
API tokens (AU-7) are scheduled for v1. No public links are required for MVP
(AU-6). Changes must retain a real actor in the operation log (ADR-0002/0003).

NF-10 permits the minimal pre-authentication setup and login surfaces and
health probes. Other application routes require authentication; workspace
permissions and instance roles apply to their respective operations.

For later OIDC work, the repository instructions prohibit runtime network calls,
while AU-2 requires an identity provider. Even an air-gapped identity provider
requires network communication. The maintainer must resolve this boundary before
OIDC is designed or implemented; this decision permits no such calls.

## Options considered

### A. Local accounts and opaque server-side sessions

Authenticate email and password locally. A random cookie identifies a session
stored in PostgreSQL; authorization is checked on the server. OIDC can later
resolve an external identity to the same internal user and issue the same kind
of session.

- Works without an identity provider and supports immediate session revocation.
- Requires password lifecycle, CSRF protection and session storage.

### B. Signed browser tokens

Send a signed JWT to the browser and authorize from its claims.

- Avoids a session lookup on each request.
- Immediate disable, logout, role changes and permission revocation still need
  server-side state or short lifetimes. Claims can become stale. Browser token
  storage does not remove the need for CSRF protection when cookies are used.

### C. Require an external identity provider now

Delegates authentication, but makes a working provider a deployment prerequisite
and prevents local-only operation (AU-1, NF-3). Rejected.

## Decision

Choose **A**. The following is the contract for later roadmap steps;
none of these routes or tables are implemented by this ADR.

### Identity and account lifecycle

- Store one internal user ID (UUIDv7), email, instance role (`admin` or `user`),
  account status and timestamps. Credentials belong to the internal user;
  passwords, session tokens and one-time secrets never enter application logs.
- Use email as the local login identifier. Trim surrounding whitespace, compare
  a single canonical lowercase form for uniqueness, and retain a display form.
  Do not claim that possession of an email address has been verified: MVP does
  not require an email service.
- **Bootstrap:** the first account is created
  through a one-time setup flow using an operator-provided secret. The instance
  starts without that secret, but refuses bootstrap until it is configured.
  Generate a high-entropy secret out of band; do not supply it in a URL, log it,
  or commit it to compose files. Submit it in an HTTPS request body. Compare it
  in constant time. Once the first user is committed, bootstrap is permanently
  closed, even if the configured secret remains. Serialize the empty-instance
  check and user creation in one database transaction, so simultaneous requests
  cannot create two first administrators. Removing or rotating the secret before
  bootstrap does not reopen an initialized instance.
- After bootstrap, public self-registration is closed. In M3/3.3 an admin creates
  accounts or resets credentials; one-time temporary credentials are conveyed
  out of band, expire, and require a password change. No email delivery or other
  external service is required. A disabled account cannot create or use sessions;
  disabling or resetting it revokes existing sessions. Preserve the user identity
  for history rather than hard-deleting an actor referenced by the operation log.
- Do not allow disabling or demoting the last active instance admin. Enforce this
  under a transaction lock, including concurrent changes. Document a separate
  operator recovery procedure before M3/3.3 is complete; do not silently create
  another administrator on an initialized database.

### Passwords and abuse resistance

- Hash passwords with Argon2id, a fresh random 128-bit salt per password, and a
  256-bit output. Record algorithm, version and work factors alongside the hash;
  verify in constant time and rehash after a successful login when parameters
  need upgrading. Start at OWASP's minimum of 19 MiB, 2 passes, 1 lane; benchmark
  on the deployment target and increase as resources allow. Bound concurrent
  password verifications to avoid exhausting memory.
- For user-chosen single-factor passwords, require at least 15 Unicode code
  points and permit at least 64 characters; accept spaces and password managers,
  avoid mandatory character classes and periodic forced rotation. Check against
  a bundled, license-compatible common-password blocklist; no runtime breach
  service. Apply a documented request-size limit. Never truncate a password.
- Return a generic login failure for nonexistent, disabled and wrong-password
  accounts; use a comparable hash-verification path for nonexistent accounts.
  Rate-limit login and bootstrap attempts by account and source, with bounded
  memory and a way to recover from throttling. Apply the same policy through
  every ingress path; only trust proxy-supplied client addresses from explicitly
  configured trusted proxies.

### Session and browser security

- Issue a fresh, opaque, cryptographically random 256-bit session token on login.
  Store only a hash of the token in PostgreSQL, with user ID, creation, last
  activity, idle expiry, absolute expiry and revocation state. Do not accept
  caller-chosen session IDs. Regenerate the session on authentication and after
  credential changes. Logout revokes the server-side session and clears the cookie.
- Use a host-only `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/` cookie with no
  `Domain`; use the `__Host-` prefix in HTTPS deployments. `Lax` permits an
  OIDC redirect later, but does not replace CSRF protection. Require HTTPS at
  the external origin; a trusted reverse proxy may terminate TLS. Local
  development over plain HTTP needs an explicit loopback-only development mode;
  never infer an insecure mode from untrusted request headers. Forwarded headers
  are trusted only from explicitly configured proxies.
- Enforce server-side idle and absolute expiry. Defaults are 30 minutes
  idle and 12 hours absolute, configurable by the operator. Normal authenticated
  requests renew idle activity, never the absolute deadline. Long-lived realtime
  connections must not extend an expired session indefinitely. In M4/4.3,
  valid user activity on a WebSocket may renew idle expiry; heartbeats alone do
  not. The absolute deadline remains fixed.
- Use a per-session synchronizer CSRF token, delivered in a same-origin response
  and returned in a custom header on state-changing HTTP requests. Validate the
  token server-side; do not place it in a URL, a log or the session cookie. GET,
  HEAD and OPTIONS must not change state. Apply origin checks as defense in depth.
  Login and bootstrap also need login-CSRF protection: use a separate short-lived
  pre-authentication CSRF context and validate it before creating a session.
- Use a configured external origin (scheme, host and port), not an untrusted
  `Host` or forwarded header, as the comparison target. Reject cross-origin
  state-changing browser requests. Do not enable wildcard credentialed CORS.

### Authorization and realtime

- Every protected request first resolves an active, unexpired session to an
  enabled internal user. Instance administration requires instance `admin`.
  Instance admin status alone does not grant workspace membership or access to
  workspace data. Workspace access requires an explicit membership; no membership
  means no access. Responses must not expose another workspace's contents.
- In MVP, the creator receives `owner` membership (4.1); owners and editors can
  read and edit, viewers can read but cannot edit or manage membership. Owners
  manage workspace members (4.2). `commenter` and the remaining owner lifecycle
  behavior are deferred to v1. Before implementing M4, specify ownership
  transfer and last-owner protection without silently weakening these rules.
- Authorize every workspace read and write on the server, including imports,
  exports and future non-browser callers. Resolve the actor ID from the session,
  never a client-supplied actor. Authorize before calling the model store.
- In M4/4.3, reject WebSocket upgrades without an active session, an exact
  configured `Origin`, and workspace read permission. A viewer may receive
  authorized snapshots and updates but cannot submit persistent changes.
  Recheck session validity and the relevant permission for messages and before
  delivering workspace data. Close connections when a user is disabled, a
  session is revoked or expires, or membership is removed. Define and test
  revocation ordering with in-flight writes and broadcasts as part of 4.3.
- Under NF-10, only the minimal pre-authentication setup/login surfaces and
  minimal health/readiness responses are available without a session. These
  surfaces expose no workspace content or administrative actions. All other
  application routes require authentication, with membership checks whenever
  a workspace is accessed.

### OIDC and later access methods

OIDC is not implemented in MVP. A later decision must define the integration,
trust configuration and NF-3 boundary before implementation. Keep internal user
IDs independent of email and of the authentication method. An eventual external
identity maps by the pair `(issuer, subject)`, not by matching an unverified or
changeable email. Do not automatically merge a local account by email. Local
accounts and recovery remain available without contacting an identity provider.
OIDC, external key retrieval, automatic provisioning, API-token authorization,
teams and anonymous sharing are outside this step; each needs its own threat
model and access rules when scheduled.

## Consequences

- PostgreSQL is required for sign-in and session checks as well as model data;
  the current readiness check already depends on it.
- Opaque sessions give the server an authoritative revocation point but add
  database reads and require careful handling of active WebSocket connections.
- HTTPS and a configured public origin need deployment documentation. Local
  HTTP remains a deliberate development-only mode, not a production default.
- A one-time bootstrap secret protects the first administrator from an earlier
  network visitor but adds an operator setup step.
- MFA, SSO, team membership, service tokens and public links are not provided
  by the MVP design. Passwords alone are not phishing-resistant.

## Verification in later steps

- **3.2:** bootstrap race and secret checks; password hashing and rate limits;
  generic login failures; session rotation, expiry, logout and disable; cookie
  flags, HTTPS/proxy configuration and pre-authenticated/authenticated CSRF;
  protected route denial; migration and restart against PostgreSQL 16 and 18.
- **3.3:** admin-only user management, temporary credential expiry and forced
  change, session revocation, concurrent last-admin protection, history retention
  and the documented recovery procedure.
- **4.1–4.3:** membership isolation for reads/writes; viewer restrictions;
  actor provenance; origin checks, revoked/expired sessions, role changes and
  unauthorized WebSocket upgrades/messages/broadcasts.

## References

- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [OWASP WebSocket Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html)
- [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html)
- [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)
