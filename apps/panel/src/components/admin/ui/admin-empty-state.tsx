import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

type Props = {
  icon: LucideIcon;
  title: string;
  action?: ReactNode;
};

export function AdminEmptyState({ icon: Icon, title, action }: Props) {
  return (
    <div className="flex flex-col items-center px-6 py-8 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gray-100 dark:bg-white/10">
        <Icon className="h-6 w-6 text-gray-400 dark:text-gray-300" aria-hidden />
      </div>
      <h3 className="mt-4 text-base font-semibold text-gray-900 dark:text-white">{title}</h3>
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}
