import "i18next";
import type { en } from "./locales/en";

// Makes t() keys type-checked against the English catalog.
declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: {
      translation: typeof en;
    };
  }
}
