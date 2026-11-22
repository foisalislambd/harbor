import { useEffect, useState } from "react";

export function browserOrigin() {
  if (typeof window === "undefined") return "";
  return window.location.origin;
}

export function publicOrigin(value: unknown) {
  if (typeof value !== "string") return "";
  const trimmed = value.trim().replace(/\/$/, "");
  if (!trimmed) return "";
  try {
    const url = new URL(trimmed);
    if (url.protocol !== "http:" && url.protocol !== "https:") return "";
    if (url.username || url.password || url.search || url.hash) return "";
    if (url.pathname !== "" && url.pathname !== "/") return "";
    return url.origin;
  } catch {
    return "";
  }
}

export function useExampleOrigin() {
  const [origin, setOrigin] = useState(browserOrigin);
  useEffect(() => {
    let ignore = false;
    fetch("/api/v1/health")
      .then((res) => (res.ok ? res.json() : null))
      .then((body: { public_url?: unknown } | null) => {
        const configured = publicOrigin(body?.public_url);
        if (!ignore && configured) setOrigin(configured);
      })
      .catch(() => undefined);
    return () => {
      ignore = true;
    };
  }, []);
  return origin;
}
