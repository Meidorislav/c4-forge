import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { en } from "./locales/en";

export const defaultLanguage = "en";

export const resources = {
  en: { translation: en },
} as const;

// Catalogs are bundled, so initialization is synchronous and the first render
// already has every string.
void i18n.use(initReactI18next).init({
  resources,
  lng: defaultLanguage,
  fallbackLng: defaultLanguage,
  initAsync: false,
  interpolation: {
    // React escapes rendered values itself.
    escapeValue: false,
  },
});

export default i18n;
