import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Workspaces } from "./Workspaces";

const initial = {
  id: "workspace-one",
  name: "Payments",
  description: "Card processing",
  role: "owner",
  updatedAt: "2026-11-03T10:00:00Z",
};

describe("Workspaces", () => {
  it("creates, edits and confirms deletion with CSRF protection", async () => {
    let items = [initial];
    const mutations: { path: string; init: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (path === "/api/auth/csrf") return new Response(JSON.stringify({ csrfToken: "token" }));
        if (!init?.method) return new Response(JSON.stringify(items));
        mutations.push({ path, init });
        if (init.method === "DELETE") {
          items = [];
          return new Response(null, { status: 204 });
        }
        items = [{ ...initial, ...JSON.parse(String(init.body)) }];
        return new Response(JSON.stringify(items[0]), {
          status: init.method === "POST" ? 201 : 200,
        });
      }),
    );
    render(<Workspaces />);
    await screen.findByRole("heading", { name: "Payments" });
    fireEvent.change(screen.getByLabelText("Workspace name"), { target: { value: "Billing" } });
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "Invoices" } });
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));
    await screen.findByRole("heading", { name: "Billing" });
    expect(mutations[0]).toMatchObject({
      path: "/api/workspaces",
      init: { method: "POST", headers: { "X-CSRF-Token": "token" } },
    });
    expect(JSON.parse(String(mutations[0]?.init.body))).toEqual({
      name: "Billing",
      description: "Invoices",
    });
    fireEvent.click(screen.getByRole("button", { name: "Edit workspace" }));
    expect(screen.getByLabelText("Workspace name")).toHaveValue("Billing");
    fireEvent.change(screen.getByLabelText("Workspace name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await screen.findByRole("heading", { name: "Renamed" });
    expect(mutations[1]?.init.method).toBe("PUT");
    fireEvent.click(screen.getByRole("button", { name: "Delete workspace" }));
    expect(screen.getByText(/all its model data and history/)).toBeInTheDocument();
    expect(mutations).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mutations).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Delete workspace" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete permanently" }));
    await screen.findByText("No workspaces found.");
    expect(mutations[2]).toMatchObject({
      path: "/api/workspaces/workspace-one",
      init: { method: "DELETE", headers: { "X-CSRF-Token": "token" } },
    });
  });

  it("filters and sorts through the API and hides owner controls for viewers", async () => {
    const requests: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        requests.push(path);
        return new Response(JSON.stringify([{ ...initial, role: "viewer" }]));
      }),
    );
    render(<Workspaces />);
    await screen.findByRole("heading", { name: "Payments" });
    expect(screen.queryByRole("button", { name: "Edit workspace" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete workspace" })).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Search workspaces"), {
      target: { value: "card & cash" },
    });
    fireEvent.change(screen.getByLabelText("Sort workspaces"), { target: { value: "name_asc" } });
    await waitFor(() =>
      expect(requests.at(-1)).toBe("/api/workspaces?q=card+%26+cash&sort=name_asc"),
    );
  });

  it("keeps form data and reports failed mutations", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (path === "/api/auth/csrf") return new Response(JSON.stringify({ csrfToken: "token" }));
        return init?.method ? new Response("{}", { status: 400 }) : new Response("[]");
      }),
    );
    render(<Workspaces />);
    await screen.findByText("No workspaces found.");
    fireEvent.change(screen.getByLabelText("Workspace name"), { target: { value: "New name" } });
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("200 characters");
    expect(screen.getByLabelText("Workspace name")).toHaveValue("New name");
    expect(screen.getByRole("button", { name: "Create workspace" })).toBeEnabled();
  });
});
