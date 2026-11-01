import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { PasswordChange, UserManagement } from "./UserManagement";

it("creates a user and displays the one-time credential", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      if (path.endsWith("/csrf")) return new Response(JSON.stringify({ csrfToken: "csrf" }));
      if (path.endsWith("/users") && init?.method === "POST")
        return new Response(
          JSON.stringify({ temporaryPassword: "temporary-secret", expiresAt: "tomorrow" }),
          { status: 201 },
        );
      return new Response(
        JSON.stringify([
          {
            id: "1",
            email: "person@example.test",
            role: "user",
            disabled: false,
            mustChangePassword: true,
          },
        ]),
      );
    }),
  );
  render(<UserManagement />);
  await screen.findByText(/person@example.test/);
  fireEvent.change(screen.getByLabelText("Email"), { target: { value: "person@example.test" } });
  fireEvent.click(screen.getByRole("button", { name: "Create user" }));
  expect(await screen.findByText("temporary-secret")).toBeInTheDocument();
});

it("submits the required password change", async () => {
  const changed = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string) =>
      path.endsWith("/csrf")
        ? new Response(JSON.stringify({ csrfToken: "csrf" }))
        : new Response("{}"),
    ),
  );
  render(<PasswordChange onChanged={changed} />);
  fireEvent.change(screen.getByLabelText("New password"), {
    target: { value: "new long password phrase" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Change password" }));
  await waitFor(() => expect(changed).toHaveBeenCalledOnce());
});
