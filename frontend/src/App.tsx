import { useTranslation } from "react-i18next";
import { ServerStatus } from "./ServerStatus";

export function App() {
  const { t } = useTranslation();
  return (
    <main>
      <h1>{t("app.name")}</h1>
      <p>{t("app.tagline")}</p>
      <ServerStatus />
    </main>
  );
}
