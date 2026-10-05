export interface Health {
  status: string;
  version: string;
}

// fetchHealth reads the server's liveness endpoint.
export async function fetchHealth(signal?: AbortSignal): Promise<Health> {
  const res = await fetch("/healthz", { signal, headers: { Accept: "application/json" } });
  if (!res.ok) {
    throw new Error(`health check failed with status ${res.status}`);
  }
  const body: unknown = await res.json();
  if (!isHealth(body)) {
    throw new Error("unexpected health check response");
  }
  return body;
}

function isHealth(value: unknown): value is Health {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const record = value as Record<string, unknown>;
  return typeof record.status === "string" && typeof record.version === "string";
}
