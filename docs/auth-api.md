# Local authentication API

Internal browser-facing API for roadmap 3.2, following ADR-0004. Responses are
JSON with `Cache-Control: no-store`. The configured external origin is required
for POST requests; clients send it in `Origin`. Credentials and CSRF values are
never passed in URLs. The browser sends cookies only to the application's origin.

- `GET /api/auth/status` returns `initialized` and `authenticated`; for an
  authenticated user it also returns `email` and `role`.
- `GET /api/auth/csrf` returns `{ "csrfToken": "..." }`. Before sign-in it
  also sets a short-lived, HttpOnly pre-authentication context cookie. Once
  signed in, the returned token is bound to the session. Call again after login,
  logout or application restart.
- `POST /api/auth/bootstrap` accepts `email`, `password` and `secret`, with the
  CSRF token in `X-CSRF-Token`. It creates the first administrator and signs in.
  It fails if bootstrap is already complete or not configured.
- `POST /api/auth/login` accepts `email` and `password`, with the CSRF token in
  `X-CSRF-Token`. A successful response sets a fresh authenticated session cookie.
  Invalid credentials have the same response for missing or disabled users.
- `POST /api/auth/logout` requires an authenticated session and its CSRF token.
  It revokes that session and clears the cookie.
- `GET /api/auth/me` returns the current user or `401` without a valid session.

Authentication failures return `401`, CSRF/origin failures return `403`, invalid
inputs return `400`, unavailable setup returns `409`, throttling returns `429`,
and unexpected storage failures return `500`. Authenticated operations never
accept an actor ID from the browser. No account-management or workspace API is
introduced here (roadmap 3.3 and 4.1).

## Deployment and development

Set `C4FORGE_PUBLIC_ORIGIN` to the exact public HTTPS origin, and terminate TLS
at a reverse proxy. Keep the backend reachable only by that proxy. The Compose
port is bound to localhost; its plain HTTP endpoint is not a production login
endpoint. Set `C4FORGE_BOOTSTRAP_SECRET` to a secret of at least 32 bytes before
starting the service. Enter it when creating the first administrator, then remove
it from the environment and restart. Setup remains closed after initialization,
even when the secret is still configured. With no secret configured, setup is
unavailable. Protect the secret as a credential and do not commit it.

`C4FORGE_SESSION_IDLE` and `C4FORGE_SESSION_ABSOLUTE` accept Go durations and
default to `30m` and `12h`. For local development, `make run` binds to loopback
and enables HTTP cookies for the Vite origin on localhost. This mode requires
both the listener and configured origin to be loopback addresses; it must not
be used for a shared deployment. Set the bootstrap secret in the shell before
starting the server.

Login throttles are held in process memory and use the direct peer address,
not forwarded headers. They reset on restart; a reverse proxy may make several
users share a limit. This implementation is for a single application instance.
An application restart also invalidates outstanding CSRF tokens, so clients
request a fresh token before each action; authenticated sessions remain stored
in PostgreSQL.
