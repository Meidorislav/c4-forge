import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchHealth } from "./api/health";

type State = { kind: "connecting" } | { kind: "ready"; version: string } | { kind: "unavailable" };

// ServerStatus shows whether the UI can reach the server, and its version.
export function ServerStatus() {
  const { t } = useTranslation();
  const [state, setState] = useState<State>({ kind: "connecting" });

  useEffect(() => {
    const controller = new AbortController();
    fetchHealth(controller.signal).then(
      (health) => setState({ kind: "ready", version: health.version }),
      () => {
        // An aborted request means the component went away; nothing to show.
        if (!controller.signal.aborted) {
          setState({ kind: "unavailable" });
        }
      },
    );
    return () => controller.abort();
  }, []);

  let text: string;
  switch (state.kind) {
    case "connecting":
      text = t("status.connecting");
      break;
    case "ready":
      text = t("status.version", { version: state.version });
      break;
    case "unavailable":
      text = t("status.unavailable");
      break;
  }
  return <p role="status">{text}</p>;
}
