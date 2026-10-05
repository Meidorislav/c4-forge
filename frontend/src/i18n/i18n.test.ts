import { describe, expect, it } from "vitest";
import i18n, { defaultLanguage } from ".";

describe("i18n", () => {
  it("is initialized synchronously with the default language", () => {
    expect(i18n.isInitialized).toBe(true);
    expect(i18n.language).toBe(defaultLanguage);
  });

  it("resolves keys from the English catalog", () => {
    expect(i18n.t("app.name")).toBe("c4-forge");
  });

  it("rejects unknown keys at compile time", () => {
    // @ts-expect-error: the key does not exist in the catalog.
    expect(i18n.t("app.doesNotExist")).toBe("app.doesNotExist");
  });
});
