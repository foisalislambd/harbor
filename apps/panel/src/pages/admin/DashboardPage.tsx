import { DashboardWelcome } from "@/components/admin/dashboard/dashboard-welcome";
import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { formatBytes, formatWhen, api, type Stats } from "@/lib/harbor-api";
import { Archive, Clock, FolderKanban, HardDrive } from "lucide-react";
import { Link } from "react-router-dom";
import { useEffect, useState } from "react";

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api<Stats>("/api/v1/stats")
      .then((data) => {
        if (!cancelled) setStats(data);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load dashboard");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (error) return <p className="text-sm text-error-500">{error}</p>;
  if (!stats) return <AdminLoading message="Loading vault…" />;

  const cards = [
    { label: "Projects", value: stats.projects.toLocaleString(), icon: FolderKanban },
    { label: "Backups", value: stats.backups.toLocaleString(), icon: Archive },
    { label: "Stored", value: formatBytes(stats.storage_bytes), icon: HardDrive },
    { label: "Last 24 hours", value: stats.backups_last_24h.toLocaleString(), icon: Clock },
  ];

  return (
    <div className="admin-content space-y-6">
      <DashboardWelcome />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {cards.map((card) => {
          const Icon = card.icon;
          return (
            <div key={card.label} className="admin-card">
              <div className="admin-card-body flex items-center justify-between gap-3">
                <div>
                  <p className="text-xs font-medium tracking-wide text-gray-500 uppercase">{card.label}</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{card.value}</p>
                </div>
                <Icon className="h-5 w-5 text-brand-500" />
              </div>
            </div>
          );
        })}
      </div>

      {stats.overdue_schedules > 0 && (
        <div className="rounded-xl border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-800 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-200">
          {stats.overdue_schedules} expected backup{stats.overdue_schedules === 1 ? " is" : "s are"} overdue.
        </div>
      )}

      <div className="grid gap-4 xl:grid-cols-5">
        <section className="admin-card xl:col-span-3">
          <div className="admin-card-body">
            <AdminPageHeader
              title="Recent backups"
              actions={
                <Link to="/backups" className="text-sm font-medium text-brand-600 hover:text-brand-700">
                  View all
                </Link>
              }
            />
            {stats.recent_backups.length === 0 ? (
              <AdminEmptyState
                icon={Archive}
                title="No backups yet"
                description="Create a project, copy its API key, and push an archive from any application."
              />
            ) : (
              <ul className="divide-y divide-gray-100 dark:divide-gray-800">
                {stats.recent_backups.map((backup) => (
                  <li key={backup.id} className="flex items-center justify-between gap-3 py-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-gray-900 dark:text-white">{backup.name}</p>
                      <p className="truncate text-xs text-gray-500">
                        {backup.project_name} · {backup.kind} · {formatBytes(backup.size_bytes)}
                      </p>
                    </div>
                    <span className="shrink-0 text-xs text-gray-500">{formatWhen(backup.created_at)}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>
        <section className="admin-card xl:col-span-2">
          <div className="admin-card-body">
            <AdminPageHeader title="Activity" />
            {stats.recent_activity.length === 0 ? (
              <p className="text-sm text-gray-500">Actions on this instance will show up here.</p>
            ) : (
              <ul className="space-y-3">
                {stats.recent_activity.map((event) => (
                  <li key={event.id}>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">{event.action}</p>
                    <p className="text-xs text-gray-500">
                      {event.actor} · {formatWhen(event.created_at)}
                    </p>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
