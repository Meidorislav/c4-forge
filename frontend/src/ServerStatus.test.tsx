import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ServerStatus } from "./ServerStatus";

function stubFetch(response: Response | Error) {
  const fetch = vi.fn(() =>
    response instanceof Error ? Promise.reject(response) : Promise.resolve(response),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("ServerStatus", () => {
  it("shows the server version", async () => {
    const fetch = stubFetch(json({ status: "ok", version: "1.2.3" }));
    render(<ServerStatus />);
    expect(screen.getByRole("status")).toHaveTextContent("Connecting to the server…");
    expect(await screen.findByText("Server version 1.2.3")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith("/healthz", expect.anything());
  });

  it.each([
    ["an error status", json({ error: "boom" }, 500)],
    ["a network error", new TypeError("Failed to fetch")],
    ["an unexpected body", json({ hello: "world" })],
  ])("shows the server as unavailable on %s", async (_, response) => {
    stubFetch(response);
    render(<ServerStatus />);
    expect(await screen.findByText("The server is unavailable.")).toBeInTheDocument();
  });
});
