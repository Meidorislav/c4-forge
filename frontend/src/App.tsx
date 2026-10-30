import { useTranslation } from "react-i18next";
import "@xyflow/react/dist/style.css";
import { CanvasSpike } from "./canvas-spike/CanvasSpike";
import "./canvas-spike/spike.css";

export function App() {
  const { t } = useTranslation();
  const checks = [
    t("spike.checks.performance"),
    t("spike.checks.c4Node"),
    t("spike.checks.derivedEdges"),
    t("spike.checks.freeform"),
    t("spike.checks.boundArrow"),
    t("spike.checks.layers"),
    t("spike.checks.controlled"),
    t("spike.checks.presence"),
    t("spike.checks.export"),
  ];
  return (
    <main className="spike-app">
      <aside className="spike-sidebar">
        <span className="spike-eyebrow">{t("spike.eyebrow")}</span>
        <h1>{t("app.name")}</h1>
        <p>{t("app.tagline")}</p>
        <h2>{t("spike.checks.title")}</h2>
        <ol>
          {checks.map((check) => (
            <li key={check}>{check}</li>
          ))}
        </ol>
        <p className="spike-help">{t("spike.help")}</p>
      </aside>
      <CanvasSpike />
    </main>
  );
}
