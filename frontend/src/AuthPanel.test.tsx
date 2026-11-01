import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AuthPanel } from "./AuthPanel";

describe("AuthPanel", () => {
  it("bootstraps the first administrator with a CSRF token", async () => {
    let signedIn = false;
    const requests: { path: string; init?: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        requests.push({ path, init });
        if (path.endsWith("/status"))
          return new Response(
            JSON.stringify({
              initialized: signedIn,
              authenticated: signedIn,
              email: signedIn ? "admin@example.test" : undefined,
              role: "admin",
            }),
            { status: 200 },
          );
        if (path.endsWith("/csrf"))
          return new Response(JSON.stringify({ csrfToken: "csrf-example" }), { status: 200 });
        if (path.endsWith("/bootstrap")) {
          signedIn = true;
          return new Response("{}", { status: 201 });
        }
        return new Response("{}", { status: 404 });
      }),
    );
    render(<AuthPanel />);
    expect(
      await screen.findByRole("heading", { name: "Set up your administrator account" }),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "admin@example.test" } });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "A long secure passphrase" },
    });
    fireEvent.change(screen.getByLabelText("Setup secret"), { target: { value: "test-secret" } });
    fireEvent.click(screen.getByRole("button", { name: "Create administrator" }));
    expect(await screen.findByText("Signed in as admin@example.test")).toBeInTheDocument();
    const bootstrap = requests.find((request) => request.path.endsWith("/bootstrap"));
    expect(bootstrap?.init?.headers).toEqual({
      "Content-Type": "application/json",
      "X-CSRF-Token": "csrf-example",
    });
    expect(JSON.parse(String(bootstrap?.init?.body))).toEqual({
      email: "admin@example.test",
      password: "A long secure passphrase",
      secret: "test-secret",
    });
  });

  it("shows a generic error for failed login", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        if (path.endsWith("/status"))
          return new Response(JSON.stringify({ initialized: true, authenticated: false }), {
            status: 200,
          });
        if (path.endsWith("/csrf"))
          return new Response(JSON.stringify({ csrfToken: "csrf-example" }), { status: 200 });
        return new Response("{}", { status: 401 });
      }),
    );
    render(<AuthPanel />);
    await screen.findByRole("heading", { name: "Sign in" });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "admin@example.test" } });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "incorrect password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("incorrect"));
    expect(screen.getByRole("button", { name: "Sign in" })).toBeEnabled();
  });
});
