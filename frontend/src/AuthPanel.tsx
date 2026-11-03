import { type FormEvent, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { PasswordChange, UserManagement } from "./UserManagement";
import { Workspaces } from "./Workspaces";

type AuthStatus = {
  initialized: boolean;
  authenticated: boolean;
  email?: string;
  role?: string;
  mustChangePassword?: boolean;
};
type ViewState =
  | { kind: "loading" }
  | { kind: "ready"; status: AuthStatus }
  | { kind: "unavailable" };

async function loadStatus(signal?: AbortSignal): Promise<AuthStatus> {
  const response = await fetch("/api/auth/status", { signal });
  if (!response.ok) throw new Error("status unavailable");
  return response.json() as Promise<AuthStatus>;
}

async function csrfToken(): Promise<string> {
  const response = await fetch("/api/auth/csrf");
  if (!response.ok) throw new Error("csrf unavailable");
  const data = (await response.json()) as { csrfToken: string };
  return data.csrfToken;
}

export function AuthPanel() {
  const { t } = useTranslation();
  const [view, setView] = useState<ViewState>({ kind: "loading" });
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    loadStatus(controller.signal).then(
      (status) => setView({ kind: "ready", status }),
      () => {
        if (!controller.signal.aborted) setView({ kind: "unavailable" });
      },
    );
    return () => controller.abort();
  }, []);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (view.kind !== "ready") return;
    setPending(true);
    setError("");
    try {
      const csrf = await csrfToken();
      const setup = !view.status.initialized;
      const response = await fetch(setup ? "/api/auth/bootstrap" : "/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify(setup ? { email, password, secret } : { email, password }),
      });
      if (!response.ok) {
        setError(
          response.status === 429
            ? t("auth.rateLimited")
            : response.status === 409
              ? t("auth.setupUnavailable")
              : response.status === 400
                ? t("auth.invalidInput")
                : t("auth.failed"),
        );
        return;
      }
      setPassword("");
      setSecret("");
      setView({ kind: "ready", status: await loadStatus() });
    } catch {
      setError(t("auth.unavailable"));
    } finally {
      setPending(false);
    }
  }

  async function logout() {
    setPending(true);
    setError("");
    try {
      const csrf = await csrfToken();
      const response = await fetch("/api/auth/logout", {
        method: "POST",
        headers: { "X-CSRF-Token": csrf },
      });
      if (!response.ok) throw new Error("logout failed");
      setView({ kind: "ready", status: await loadStatus() });
    } catch {
      setError(t("auth.unavailable"));
    } finally {
      setPending(false);
    }
  }

  if (window.isSecureContext === false) return <p role="alert">{t("auth.httpsRequired")}</p>;
  if (view.kind === "loading") return <p>{t("auth.loading")}</p>;
  if (view.kind === "unavailable") return <p role="alert">{t("auth.unavailable")}</p>;
  if (view.status.authenticated) {
    return (
      <section aria-labelledby="account-heading">
        <h2 id="account-heading">{t("auth.signedIn")}</h2>
        <p>{t("auth.signedInAs", { email: view.status.email ?? "" })}</p>
        {view.status.mustChangePassword ? (
          <PasswordChange
            onChanged={() => {
              void loadStatus().then((status) => setView({ kind: "ready", status }));
            }}
          />
        ) : (
          <>
            <Workspaces />
            {view.status.role === "admin" && <UserManagement />}
          </>
        )}
        {error && <p role="alert">{error}</p>}
        <button type="button" disabled={pending} onClick={logout}>
          {t("auth.signOut")}
        </button>
      </section>
    );
  }

  const setup = !view.status.initialized;
  return (
    <section aria-labelledby="auth-heading">
      <h2 id="auth-heading">{setup ? t("auth.setupTitle") : t("auth.signIn")}</h2>
      {setup && <p>{t("auth.setupDescription")}</p>}
      <form onSubmit={submit}>
        <label htmlFor="email">{t("auth.email")}</label>
        <input
          id="email"
          type="email"
          autoComplete="username"
          required
          value={email}
          onChange={(event) => setEmail(event.target.value)}
        />
        <label htmlFor="password">{t("auth.password")}</label>
        <input
          id="password"
          type="password"
          autoComplete={setup ? "new-password" : "current-password"}
          required
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        {setup && (
          <>
            <p>{t("auth.passwordHint")}</p>
            <label htmlFor="secret">{t("auth.setupSecret")}</label>
            <input
              id="secret"
              type="password"
              autoComplete="off"
              required
              value={secret}
              onChange={(event) => setSecret(event.target.value)}
            />
          </>
        )}
        {error && <p role="alert">{error}</p>}
        <button type="submit" disabled={pending}>
          {setup ? t("auth.createAdmin") : t("auth.signIn")}
        </button>
      </form>
    </section>
  );
}
