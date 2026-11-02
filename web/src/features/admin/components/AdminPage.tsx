import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useSession } from "@/features/identity";
import { EmptyState, PageHeader } from "@/shared/ui";

/**
 * The frame of an admin page: a 404-style empty state for everyone else (the
 * server answers 403 anyway), and no query runs until the role is known.
 */
export function AdminPage({ icon, title, ownerOnly, actions, children }: { icon: ReactNode; title: string; ownerOnly?: boolean; actions?: ReactNode; children: ReactNode }) {
  const { t } = useTranslation("admin");
  const session = useSession();
  const role = session.data?.actor.role;
  const allowed = ownerOnly ? role === "owner" : role === "owner" || role === "admin";
  if (!session.isSuccess) return <div className="flex-1" />;
  if (!allowed) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <PageHeader icon={icon} title={title} />
        <EmptyState icon={icon} title={t("notFound.title")} body={t(ownerOnly ? "notFound.owner" : "notFound.admin")} />
      </div>
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader icon={icon} title={title} actions={actions} />
      {children}
    </div>
  );
}
