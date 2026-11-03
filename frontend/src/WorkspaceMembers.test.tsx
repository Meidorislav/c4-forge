import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WorkspaceMembers } from "./WorkspaceMembers";
import { Workspaces } from "./Workspaces";

const owner = {
  userId: "first",
  email: "owner@example.test",
  role: "owner",
  disabled: false,
  mustChangePassword: false,
  isCurrentUser: true,
};
const viewer = {
  userId: "second",
  email: "viewer@example.test",
  role: "viewer",
  disabled: false,
  mustChangePassword: false,
  isCurrentUser: false,
};

describe("WorkspaceMembers", () => {
  it("adds existing accounts and changes roles with CSRF", async () => {
    let members = [owner];
    const mutations: { path: string; init: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (path === "/api/auth/csrf") return new Response(JSON.stringify({ csrfToken: "csrf" }));
        if (!init?.method) return new Response(JSON.stringify(members));
        mutations.push({ path, init });
        if (init.method === "POST") members = [owner, viewer];
        else members = [owner, { ...viewer, role: "editor" }];
        return new Response(null, { status: 204 });
      }),
    );
    const changed = vi.fn();
    render(<WorkspaceMembers workspaceId="workspace" onChanged={changed} onClose={vi.fn()} />);
    await screen.findByText(/owner@example.test/);
    fireEvent.change(screen.getByLabelText("Member email"), {
      target: { value: "viewer@example.test" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add member" }));
    await screen.findByText("viewer@example.test");
    expect(mutations[0]).toMatchObject({
      path: "/api/workspaces/workspace/members",
      init: { method: "POST", headers: { "X-CSRF-Token": "csrf" } },
    });
    expect(JSON.parse(String(mutations[0]?.init.body))).toEqual({
      email: "viewer@example.test",
      role: "viewer",
    });
    fireEvent.change(screen.getByLabelText("Workspace role for viewer@example.test"), {
      target: { value: "editor" },
    });
    await waitFor(() => expect(changed).toHaveBeenCalledTimes(2));
    expect(mutations[1]).toMatchObject({
      path: "/api/workspaces/workspace/members/second",
      init: { method: "PUT" },
    });
  });

  it("confirms ownership transfer and closes management when the caller becomes editor", async () => {
    let transferred = false;
    const mutations: RequestInit[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (path === "/api/auth/csrf") return new Response(JSON.stringify({ csrfToken: "csrf" }));
        if (init?.method) {
          mutations.push(init);
          transferred = true;
          return new Response(null, { status: 204 });
        }
        if (path.endsWith("/members")) return new Response(JSON.stringify([owner, viewer]));
        return new Response(
          JSON.stringify([
            {
              id: "workspace",
              name: "Payments",
              description: "",
              role: transferred ? "editor" : "owner",
              updatedAt: "2026-11-03T10:00:00Z",
            },
          ]),
        );
      }),
    );
    render(<Workspaces />);
    fireEvent.click(await screen.findByRole("button", { name: "Manage members" }));
    fireEvent.click(await screen.findByRole("button", { name: "Transfer ownership" }));
    expect(screen.getByText(/You will become an editor/)).toBeInTheDocument();
    expect(mutations).toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mutations).toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Transfer ownership" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm change" }));
    await waitFor(() =>
      expect(screen.queryByRole("heading", { name: "Workspace members" })).not.toBeInTheDocument(),
    );
    expect(mutations[0]).toMatchObject({
      method: "POST",
      body: JSON.stringify({ userId: "second" }),
    });
    expect(screen.queryByRole("button", { name: "Manage members" })).not.toBeInTheDocument();
  });

  it("requires removal confirmation and shows last-owner conflicts", async () => {
    const mutations = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (path === "/api/auth/csrf") return new Response(JSON.stringify({ csrfToken: "csrf" }));
        if (!init?.method) return new Response(JSON.stringify([owner]));
        mutations(init);
        return new Response(JSON.stringify({ error: "last_owner" }), { status: 409 });
      }),
    );
    render(<WorkspaceMembers workspaceId="workspace" onChanged={vi.fn()} onClose={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Remove member" }));
    expect(mutations).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Confirm change" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("at least one enabled owner");
    expect(mutations).toHaveBeenCalledWith(expect.objectContaining({ method: "DELETE" }));
    expect(screen.getByText(/owner@example.test.*you/)).toBeInTheDocument();
  });
});
