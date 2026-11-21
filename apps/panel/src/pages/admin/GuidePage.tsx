import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { agentPrompt } from "@/lib/agent-prompt";
import { useExampleOrigin } from "@/lib/example-origin";
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

export default function GuidePage() {
  const origin = useExampleOrigin();
  const prompt = agentPrompt(origin);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");
  const curl = `curl -X POST ${origin}/api/v1/push \\
  -H "Authorization: Bearer hb_your_project_key" \\
  -F "file=@backup.tar.gz" \\
  -F "name=nightly" \\
  -F "kind=archive" \\
  -F "source=my-app"`;
  const cli = `harbor push -file backup.tar.gz -token hb_your_project_key -endpoint ${origin} -name nightly -kind archive -source my-app`;

  return (
    <div className="admin-content space-y-3">
      <AdminPageHeader title="API guide" />
      <div className="admin-card">
        <div className="admin-card-body space-y-4 text-sm leading-relaxed text-gray-600 dark:text-gray-300">
          <p>
            This is the same panel that opens when you run the Harbor desktop app. Create a project, then create an API key. Any application that can send an HTTP request can store a backup here. The key only sees that one project. Backups stay on this machine. On Destinations you can add object storage or a Telegram account, then choose it on a project to keep a second copy there too. Telegram signs in with your api_id and api_hash and uploads each backup as a document. The examples below use this panel's address.
          </p>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Push a file</h2>
          <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs text-gray-100">{curl}</pre>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Harbor CLI</h2>
          <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs text-gray-100">{cli}</pre>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Other calls with the same key</h2>
          <ul className="list-disc space-y-1 pl-5">
            <li>GET {origin}/api/v1/backups</li>
            <li>GET {origin}/api/v1/backups/:id</li>
            <li>GET {origin}/api/v1/backups/:id/download</li>
            <li>DELETE {origin}/api/v1/backups/:id</li>
          </ul>
          <p>Kinds are archive, database, files, or other. Identical file contents are stored once and can be referenced by more than one snapshot.</p>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Give this to an AI agent</h2>
          <p>
            Copy the prompt and paste it into Cursor, ChatGPT, Claude, or another agent. The address below is this Harbor. Create a project API key in the panel, then paste that key into the chat yourself. The prompt does not contain the key.
          </p>
          <div className="flex justify-end">
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
              {copyState === "copied" ? <Check className="mr-2 h-4 w-4" /> : <Copy className="mr-2 h-4 w-4" />}
              {copyState === "copied" ? "Copied" : copyState === "failed" ? "Could not copy" : "Copy prompt"}
            </button>
          </div>
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-xl bg-gray-950 p-4 text-xs leading-relaxed text-gray-100">{prompt}</pre>
        </div>
      </div>
    </div>
  );
}
