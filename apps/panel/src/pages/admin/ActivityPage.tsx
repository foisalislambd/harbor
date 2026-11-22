import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { api, formatWhen, type AuditEvent } from "@/lib/harbor-api";
import { Activity, Archive, FolderPlus, KeyRound, Server, UserRound, type LucideIcon } from "lucide-react";
import { useEffect, useState } from "react";

const actions: Record<string, { label: string; icon: LucideIcon }> = {
  "backup.created": { label: "Backup created", icon: Archive },
  "key.created": { label: "API key created", icon: KeyRound },
  "project.created": { label: "Project created", icon: FolderPlus },
  "instance.setup": { label: "Instance set up", icon: Server },
};

function actionOf(action: string) {
  return actions[action] ?? {
    label: action.replaceAll(".", " ").replaceAll("_", " "),
    icon: Activity,
  };
}

function actorLabel(actor: string) {
  if (actor.startsWith("user:")) return actor.slice(5);
  if (actor.startsWith("key:")) return actor.slice(4);
  return actor;
}

export default function ActivityPage() {
  const [items, setItems] = useState<AuditEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<{ items: AuditEvent[] }>("/api/v1/audit?limit=100")
      .then((res) => setItems(res.items))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load activity"));
  }, []);

  if (error) return <p className="text-sm text-error-500">{error}</p>;
  if (!items) return <AdminLoading message="Loading activity…" />;

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader
        title="Activity"
      />
      {items.length === 0 ? (
        <div className="admin-card">
          <AdminEmptyState icon={Activity} title="No activity yet" />
        </div>
      ) : (
        <div className="admin-card overflow-x-auto">
          <table className="admin-table w-full min-w-[720px] text-left text-sm">
            <thead>
              <tr>
                <th className="px-4 py-2.5 font-medium">When</th>
                <th className="px-4 py-2.5 font-medium">Action</th>
                <th className="px-4 py-2.5 font-medium">Actor</th>
                <th className="px-4 py-2.5 font-medium">Detail</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {items.map((event) => {
                const action = actionOf(event.action);
                const Icon = action.icon;
                const actor = actorLabel(event.actor);
                return (
                  <tr key={event.id}>
                    <td className="px-4 py-3 whitespace-nowrap text-gray-500">{formatWhen(event.created_at)}</td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center gap-2 font-medium text-gray-900 dark:text-white">
                        <span className="admin-icon-well h-7 w-7 rounded-md">
                          <Icon className="h-3.5 w-3.5" aria-hidden />
                        </span>
                        {action.label}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center gap-2 text-gray-700 dark:text-gray-200">
                        <UserRound className="h-3.5 w-3.5 shrink-0 text-gray-400" aria-hidden />
                        <span className="truncate">{actor}</span>
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-500">{event.meta || event.target || "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
