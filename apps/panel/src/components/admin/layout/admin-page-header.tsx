import type { ReactNode } from "react";

type Props = {
  title: string;
  actions?: ReactNode;
};

export function AdminPageHeader({ title, actions }: Props) {
  return (
    <div className="flex items-center justify-between gap-3">
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-lg font-semibold tracking-tight text-gray-900 dark:text-white">
          {title}
        </h1>
      </div>
      {actions && (
        <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>
      )}
    </div>
  );
}
