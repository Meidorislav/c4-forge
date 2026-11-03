import { type FormEvent, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { WorkspaceMembers } from "./WorkspaceMembers";

type Workspace = {
  id: string;
  name: string;
  description: string;
  role: "owner" | "editor" | "viewer";
  updatedAt: string;
};

export function Workspaces() {
  const { t } = useTranslation();
  const [membersFor, setMembersFor] = useState<string | null>(null);
  const [items, setItems] = useState<Workspace[]>([]);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState("modified_desc");
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [deleting, setDeleting] = useState<Workspace | null>(null);
  const nameInput = useRef<HTMLInputElement>(null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: Successful mutations invalidate the list without changing its filters.
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    const parameters = new URLSearchParams({ q: query, sort });
    void fetch(`/api/workspaces?${parameters}`, { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error("load failed");
        const result = (await response.json()) as Workspace[];
        if (!controller.signal.aborted) setItems(result);
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setItems([]);
          setError(t("workspaces.error"));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [query, sort, revision, t]);

  function clearForm() {
    setEditing(null);
    setName("");
    setDescription("");
  }

  async function mutate(method: string, id?: string) {
    setPending(true);
    setError("");
    try {
      const csrf = await fetch("/api/auth/csrf");
      if (!csrf.ok) throw new Error("csrf failed");
      const { csrfToken } = (await csrf.json()) as { csrfToken: string };
      const response = await fetch(id ? `/api/workspaces/${id}` : "/api/workspaces", {
        method,
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
        body: method === "DELETE" ? undefined : JSON.stringify({ name, description }),
      });
      if (!response.ok) {
        setError(response.status === 400 ? t("workspaces.invalid") : t("workspaces.error"));
        return;
      }
      clearForm();
      setDeleting(null);
      setRevision((value) => value + 1);
    } catch {
      setError(t("workspaces.error"));
    } finally {
      setPending(false);
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void mutate(editing ? "PUT" : "POST", editing ?? undefined);
  }

  return (
    <section aria-labelledby="workspaces-heading">
      <h2 id="workspaces-heading">{t("workspaces.title")}</h2>
      <form onSubmit={submit}>
        <fieldset disabled={pending}>
          <legend>{editing ? t("workspaces.edit") : t("workspaces.create")}</legend>
          <label htmlFor="workspace-name">{t("workspaces.name")}</label>
          <input
            id="workspace-name"
            ref={nameInput}
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
          />
          <label htmlFor="workspace-description">{t("workspaces.description")}</label>
          <textarea
            id="workspace-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
          <button type="submit">{editing ? t("workspaces.save") : t("workspaces.create")}</button>
          {editing && (
            <button type="button" onClick={clearForm}>
              {t("workspaces.cancel")}
            </button>
          )}
        </fieldset>
      </form>
      <label htmlFor="workspace-search">{t("workspaces.search")}</label>
      <input
        id="workspace-search"
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      <label htmlFor="workspace-sort">{t("workspaces.sort")}</label>
      <select id="workspace-sort" value={sort} onChange={(event) => setSort(event.target.value)}>
        <option value="modified_desc">{t("workspaces.newest")}</option>
        <option value="modified_asc">{t("workspaces.oldest")}</option>
        <option value="name_asc">{t("workspaces.nameAsc")}</option>
        <option value="name_desc">{t("workspaces.nameDesc")}</option>
      </select>
      {error && <p role="alert">{error}</p>}
      {loading ? (
        <p role="status">{t("workspaces.loading")}</p>
      ) : items.length === 0 && !error ? (
        <p>{t("workspaces.empty")}</p>
      ) : null}
      <ul aria-busy={loading}>
        {items.map((item) => (
          <li key={item.id}>
            <h3>{item.name}</h3>
            <p style={{ whiteSpace: "pre-wrap" }}>{item.description}</p>
            <p>{t("workspaces.modified", { value: new Date(item.updatedAt).toLocaleString() })}</p>
            {item.role === "owner" && (
              <>
                <button type="button" disabled={pending} onClick={() => setMembersFor(item.id)}>
                  {t("members.manage")}
                </button>
                <button
                  type="button"
                  disabled={pending}
                  onClick={() => {
                    setEditing(item.id);
                    setName(item.name);
                    setDescription(item.description);
                    setDeleting(null);
                    nameInput.current?.focus();
                  }}
                >
                  {t("workspaces.edit")}
                </button>
                <button type="button" disabled={pending} onClick={() => setDeleting(item)}>
                  {t("workspaces.delete")}
                </button>
              </>
            )}
          </li>
        ))}
      </ul>
      {membersFor && items.some((item) => item.id === membersFor && item.role === "owner") && (
        <WorkspaceMembers
          key={membersFor}
          workspaceId={membersFor}
          onClose={() => setMembersFor(null)}
          onChanged={() => setRevision((value) => value + 1)}
        />
      )}
      {deleting && (
        <fieldset disabled={pending}>
          <legend>{t("workspaces.confirmTitle")}</legend>
          <p>{t("workspaces.confirm", { name: deleting.name })}</p>
          <button
            type="button"
            disabled={pending}
            onClick={() => void mutate("DELETE", deleting.id)}
          >
            {t("workspaces.confirmDelete")}
          </button>
          <button type="button" disabled={pending} onClick={() => setDeleting(null)}>
            {t("workspaces.cancel")}
          </button>
        </fieldset>
      )}
    </section>
  );
}
