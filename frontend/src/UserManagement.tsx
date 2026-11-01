import { type FormEvent, useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

type User = {
  id: string;
  email: string;
  role: string;
  disabled: boolean;
  mustChangePassword: boolean;
};
type TemporaryCredential = { temporaryPassword: string; expiresAt: string };

async function post(path: string, body?: object): Promise<Response> {
  const csrfResponse = await fetch("/api/auth/csrf");
  if (!csrfResponse.ok) throw new Error("csrf");
  const { csrfToken } = (await csrfResponse.json()) as { csrfToken: string };
  return fetch(path, {
    method: "POST",
    headers: { "X-CSRF-Token": csrfToken, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
}

export function PasswordChange({ onChanged }: { onChanged: () => void }) {
  const { t } = useTranslation();
  const [password, setPassword] = useState("");
  const [error, setError] = useState(false);
  const [pending, setPending] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(false);
    try {
      const response = await post("/api/auth/change-password", { password });
      if (!response.ok) throw new Error("change failed");
      setPassword("");
      onChanged();
    } catch {
      setError(true);
    } finally {
      setPending(false);
    }
  }
  return (
    <section>
      <h2>{t("users.changeTitle")}</h2>
      <p>{t("auth.passwordHint")}</p>
      <form onSubmit={submit}>
        <label htmlFor="new-password">{t("users.newPassword")}</label>
        <input
          id="new-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
        <button disabled={pending} type="submit">
          {t("users.changePassword")}
        </button>
      </form>
      {error && <p role="alert">{t("users.error")}</p>}
    </section>
  );
}

export function UserManagement() {
  const { t } = useTranslation();
  const [users, setUsers] = useState<User[]>([]);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("user");
  const [credential, setCredential] = useState<TemporaryCredential | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const load = useCallback(async () => {
    const response = await fetch("/api/auth/users");
    if (!response.ok) throw new Error("users");
    setUsers((await response.json()) as User[]);
  }, []);
  useEffect(() => {
    void load().catch(() => setError(t("users.error")));
  }, [load, t]);
  async function action(path: string, body?: object) {
    setPending(true);
    setError("");
    setCredential(null);
    try {
      const response = await post(path, body);
      if (!response.ok) {
        setError(response.status === 409 ? t("users.conflict") : t("users.error"));
        return;
      }
      const result = (await response.json()) as Partial<TemporaryCredential>;
      if (result.temporaryPassword && result.expiresAt)
        setCredential({ temporaryPassword: result.temporaryPassword, expiresAt: result.expiresAt });
      setEmail("");
      await load();
    } catch {
      setError(t("users.error"));
    } finally {
      setPending(false);
    }
  }
  function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void action("/api/auth/users", { email, role });
  }
  return (
    <section>
      <h2>{t("users.title")}</h2>
      <form onSubmit={create}>
        <label htmlFor="user-email">{t("auth.email")}</label>
        <input
          id="user-email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <label htmlFor="user-role">{t("users.role")}</label>
        <select id="user-role" value={role} onChange={(e) => setRole(e.target.value)}>
          <option value="user">{t("users.user")}</option>
          <option value="admin">{t("users.admin")}</option>
        </select>
        <button type="submit" disabled={pending}>
          {t("users.create")}
        </button>
      </form>
      {credential && (
        <div role="status">
          <p>{t("users.credentialHint")}</p>
          <code>{credential.temporaryPassword}</code>
          <p>{t("users.expires", { value: credential.expiresAt })}</p>
        </div>
      )}
      {error && <p role="alert">{error}</p>}
      <ul>
        {users.map((user) => (
          <li key={user.id}>
            <span>
              {user.email} — {user.role === "admin" ? t("users.admin") : t("users.user")}{" "}
              {user.disabled && t("users.disabled")}
            </span>
            {!user.disabled && (
              <>
                <button
                  disabled={pending}
                  type="button"
                  onClick={() => void action(`/api/auth/users/${user.id}/reset`)}
                >
                  {t("users.reset")}
                </button>
                <button
                  disabled={pending}
                  type="button"
                  onClick={() => void action(`/api/auth/users/${user.id}/disable`)}
                >
                  {t("users.disable")}
                </button>
              </>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
