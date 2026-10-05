import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { App } from "./App";

describe("App", () => {
  it("shows the product name and tagline", () => {
    // The server status is covered by its own tests; keep it pending here.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => {})),
    );
    render(<App />);
    expect(screen.getByRole("heading", { name: "c4-forge" })).toBeInTheDocument();
    expect(
      screen.getByText("Model your software architecture with C4, together."),
    ).toBeInTheDocument();
  });
});
