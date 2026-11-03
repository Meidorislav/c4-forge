import { type FormEvent, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

type Member = {
  userId: string;
  email: string;
  role: "owner" | "editor" | "viewer";
  disabled: boolean;
  mustChangePassword: boolean;
  isCurrentUser: boolean;
};

type Props = { workspaceId: string; onChanged: () => void; onClose: () => void };

export function WorkspaceMembers({ workspaceId, onChanged, onClose }: Props) {
  const { t } = useTranslation();
  const [members, setMembers] = useState<Member[]>([]);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("viewer");
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [confirmation, setConfirmation] = useState<{
    kind: "remove" | "transfer";
    member: Member;
  } | null>(null);
  const base = `/api/workspaces/${workspaceId}`;

  // biome-ignore lint/correctness/useExhaustiveDependencies: A successful membership mutation invalidates this list.
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    void fetch(`${base}/members`, { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error("members unavailable");
        const result = (await response.json()) as Member[];
        if (!controller.signal.aborted) setMembers(result);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(t("members.error"));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [base, revision, t]);

  async function action(method: string, suffix: string, body?: object) {
    setPending(true);
    setError("");
    try {
      const csrfResponse = await fetch("/api/auth/csrf");
      if (!csrfResponse.ok) throw new Error("csrf unavailable");
      const { csrfToken } = (await csrfResponse.json()) as { csrfToken: string };
      const response = await fetch(`${base}${suffix}`, {
        method,
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
        body: body ? JSON.stringify(body) : undefined,
      });
      if (!response.ok) {
        const result = (await response.json()) as { error?: string };
        setError(
          result.error === "last_owner"
            ? t("members.lastOwner")
            : result.error === "member_unavailable"
              ? t("members.unavailable")
              : t("members.error"),
        );
        return;
      }
      setEmail("");
      setConfirmation(null);
      setRevision((value) => value + 1);
      onChanged();
    } catch {
      setError(t("members.error"));
    } finally {
      setPending(false);
    }
  }

  function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void action("POST", "/members", { email, role });
  }

  return (
    <section aria-labelledby="members-heading">
      <h3 id="members-heading">{t("members.title")}</h3>
      <p>{t("members.hint")}</p>
      <button type="button" disabled={pending} onClick={onClose}>
        {t("members.close")}
      </button>
      <form onSubmit={add}>
        <fieldset disabled={pending}>
          <legend>{t("members.add")}</legend>
          <label htmlFor="member-email">{t("members.email")}</label>
          <input
            id="member-email"
            type="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
          <label htmlFor="member-role">{t("members.role")}</label>
          <select id="member-role" value={role} onChange={(event) => setRole(event.target.value)}>
            <option value="viewer">{t("members.viewer")}</option>
            <option value="editor">{t("members.editor")}</option>
          </select>
          <button type="submit">{t("members.add")}</button>
        </fieldset>
      </form>
      {error && <p role="alert">{error}</p>}
      {loading && <p role="status">{t("members.loading")}</p>}
      <ul aria-busy={loading}>
        {members.map((member) => (
          <li key={member.userId}>
            <p>
              {member.email} {member.isCurrentUser && t("members.you")}{" "}
              {member.disabled && t("members.disabled")}
            </p>
            <select
              aria-label={t("members.roleFor", { email: member.email })}
              value={member.role}
              disabled={pending}
              onChange={(event) =>
                void action("PUT", `/members/${member.userId}`, { role: event.target.value })
              }
            >
              <option value="viewer">{t("members.viewer")}</option>
              <option value="editor">{t("members.editor")}</option>
              <option value="owner" disabled={member.disabled || member.mustChangePassword}>
                {t("members.owner")}
              </option>
            </select>
            <button
              type="button"
              disabled={pending}
              onClick={() => setConfirmation({ kind: "remove", member })}
            >
              {t("members.remove")}
            </button>
            {!member.isCurrentUser && !member.disabled && !member.mustChangePassword && (
              <button
                type="button"
                disabled={pending}
                onClick={() => setConfirmation({ kind: "transfer", member })}
              >
                {t("members.transfer")}
              </button>
            )}
          </li>
        ))}
      </ul>
      {confirmation && (
        <fieldset disabled={pending}>
          <legend>{t("members.confirmTitle")}</legend>
          <p>
            {confirmation.kind === "transfer"
              ? t("members.confirmTransfer", { email: confirmation.member.email })
              : t("members.confirmRemove", { email: confirmation.member.email })}
          </p>
          <button
            type="button"
            onClick={() => {
              if (confirmation.kind === "transfer")
                void action("POST", "/transfer", { userId: confirmation.member.userId });
              else void action("DELETE", `/members/${confirmation.member.userId}`);
            }}
          >
            {t("members.confirm")}
          </button>
          <button type="button" onClick={() => setConfirmation(null)}>
            {t("workspaces.cancel")}
          </button>
        </fieldset>
      )}
    </section>
  );
}
