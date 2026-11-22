import { quietButtonClass } from "@/lib/fields";
import { Check, Copy } from "lucide-react";
import { useState } from "react";

async function copyPrompt(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    return;
  } catch {
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.focus();
    area.select();
    const ok = document.execCommand("copy");
    area.remove();
    if (!ok) throw new Error("copy failed");
  }
}

export function CopyAgentButton({ prompt }: { prompt: string }) {
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");

  return (
    <button
      type="button"
      className={quietButtonClass}
      onClick={() => {
        void copyPrompt(prompt)
          .then(() => setCopyState("copied"))
          .catch(() => setCopyState("failed"))
          .finally(() => window.setTimeout(() => setCopyState("idle"), 2000));
      }}
    >
      {copyState === "copied" ? <Check className="h-4 w-4" aria-hidden /> : <Copy className="h-4 w-4" aria-hidden />}
      {copyState === "copied" ? "Copied" : copyState === "failed" ? "Could not copy" : "Copy for AI agent"}
    </button>
  );
}
