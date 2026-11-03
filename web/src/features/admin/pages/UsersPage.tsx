import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Users } from "lucide-react";
import { useSession } from "@/features/identity";
import type { Schemas } from "@/shared/api";
import { fullTime } from "@/shared/lib";
import { Avatar, Badge, Button, Dialog, EmptyState, Input, Notice, Select, Skeleton } from "@/shared/ui";
import { useCreateUser, useResetPassword, useSetRole, useSetUserState, useUsers } from "../api";
import { AdminPage } from "../components/AdminPage";
import { SecretOnce } from "../components/SecretOnce";
import { generatePassword, MIN_PASSWORD, roles, validHandle } from "../model";

type Row = Schemas["UserRow"];
type Action = { kind: "role" | "password" | "state"; row: Row } | { kind: "new" } | null;

const roleTone = { owner: "accent", admin: "info", member: "neutral", observer: "neutral" } as const;

/** Felhasználók (/admin/users): owner only; the server refuses everyone else as well. */
export default function UsersPage() {
  const { t } = useTranslation("admin");
  const [action, setAction] = useState<Action>(null);
  return (
    <AdminPage
      ownerOnly
      icon={<Users className="size-5" />}
      title={t("users.title")}
      actions={
        <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => setAction({ kind: "new" })}>
          {t("users.new")}
        </Button>
      }
    >
      <UsersTable onAction={setAction} />
      {action?.kind === "new" ? <NewUserDialog onClose={() => setAction(null)} /> : null}
      {action?.kind === "role" ? <RoleDialog row={action.row} onClose={() => setAction(null)} /> : null}
      {action?.kind === "password" ? <PasswordDialog row={action.row} onClose={() => setAction(null)} /> : null}
      {action?.kind === "state" ? <StateDialog row={action.row} onClose={() => setAction(null)} /> : null}
    </AdminPage>
  );
}

function UsersTable({ onAction }: { onAction: (a: Action) => void }) {
  const { t, i18n } = useTranslation("admin");
  const session = useSession();
  const users = useUsers(true);
  const me = session.data?.actor.id;
  if (users.isPending) return <Skeleton className="m-4 h-32" />;
  if (users.isError) return <p className="p-4 text-danger">{users.error.message}</p>;
  if (!users.data.length) return <EmptyState title={t("users.empty")} />;
  return (
    <div className="min-h-0 flex-1 overflow-auto p-3 sm:p-4">
      <table className="w-full min-w-[46rem] text-ui">
        <thead className="border-b border-border text-left text-meta text-fg-subtle">
          <tr>
            <th className="px-2 py-1.5 font-medium">{t("users.name")}</th>
            <th className="px-2 py-1.5 font-medium">{t("users.role")}</th>
            <th className="px-2 py-1.5 font-medium">{t("users.state")}</th>
            <th className="px-2 py-1.5 font-medium">{t("users.lastSeen")}</th>
            <th className="px-2 py-1.5 text-right font-medium">{t("users.sessions")}</th>
            <th className="px-2 py-1.5 text-right font-medium">{t("users.actions")}</th>
          </tr>
        </thead>
        <tbody>
          {users.data.map((row) => {
            const a = row.actor;
            const self = a.id === me;
            const stopped = a.state === "stopped";
            return (
              <tr key={a.id} className="border-b border-border/60 last:border-0">
                <td className="px-2 py-2">
                  <span className="flex items-center gap-2">
                    <Avatar kind="human" name={a.display_name || a.handle} emoji={a.avatar_emoji} size="sm" />
                    <span className="min-w-0">
                      <span className="block truncate font-medium">{a.display_name || a.handle}</span>
                      <span className="block truncate text-meta text-fg-muted">@{a.handle}</span>
                    </span>
                  </span>
                </td>
                <td className="px-2">
                  <Badge tone={roleTone[a.role]}>{t(`roles.${a.role}`)}</Badge>
                </td>
                <td className="px-2">{stopped ? <Badge tone="danger">{t("users.disabled")}</Badge> : <span>{t("users.enabled")}</span>}</td>
                <td className="px-2 whitespace-nowrap text-fg-muted">
                  {row.last_active_at ? fullTime(row.last_active_at, i18n.language) : t("users.never")}
                  {row.last_login_at ? <span className="block text-meta text-fg-subtle">{t("users.lastLogin", { when: fullTime(row.last_login_at, i18n.language) })}</span> : null}
                </td>
                <td className="tabular px-2 text-right">{row.active_sessions}</td>
                <td className="px-2 text-right whitespace-nowrap">
                  {self ? (
                    <span className="text-meta text-fg-subtle">{t("users.self")}</span>
                  ) : (
                    <span className="inline-flex gap-1">
                      <Button size="sm" variant="ghost" onClick={() => onAction({ kind: "role", row })}>
                        {t("users.roleAction")}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => onAction({ kind: "password", row })}>
                        {t("users.passwordAction")}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => onAction({ kind: "state", row })}>
                        {t(stopped ? "users.enable" : "users.disable")}
                      </Button>
                    </span>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function RoleSelect({ value, onChange }: { value: string; onChange: (v: (typeof roles)[number]) => void }) {
  const { t } = useTranslation("admin");
  return (
    <label className="flex flex-col gap-1 text-ui">
      <span className="font-medium">{t("users.role")}</span>
      <Select value={value} onChange={(e) => onChange(e.target.value as (typeof roles)[number])}>
        {roles.map((r) => (
          <option key={r} value={r}>
            {t(`roles.${r}`)}
          </option>
        ))}
      </Select>
      <span className="text-meta text-fg-subtle">{t(`roleHelp.${value}`)}</span>
    </label>
  );
}

/** The password fields: typed by the owner, or generated and then shown once. */
function PasswordField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation("admin");
  const short = value !== "" && value.length < MIN_PASSWORD;
  return (
    <div className="flex flex-col gap-1 text-ui">
      <label htmlFor="admin-new-password" className="font-medium">
        {t("users.initialPassword")}
      </label>
      <div className="flex gap-2">
        <Input id="admin-new-password" type="text" autoComplete="new-password" spellCheck={false} value={value} aria-invalid={short} onChange={(e) => onChange(e.target.value)} className="min-w-0 flex-1 font-mono" />
        <Button type="button" onClick={() => onChange(generatePassword())}>
          {t("users.generate")}
        </Button>
      </div>
      <span className={short ? "text-meta text-danger" : "text-meta text-fg-subtle"}>{t("users.passwordHint", { min: MIN_PASSWORD })}</span>
    </div>
  );
}

function NewUserDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation("admin");
  const create = useCreateUser();
  const [handle, setHandle] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<(typeof roles)[number]>("member");
  const [password, setPassword] = useState("");
  const [created, setCreated] = useState<string | null>(null);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate({ handle, display_name: name, role, password }, { onSuccess: () => setCreated(password) });
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("users.newTitle")} closeLabel={t("close")}>
      {created ? (
        <div className="flex flex-col gap-3">
          <p className="text-ui">{t("users.createdBody", { handle })}</p>
          <SecretOnce secret={created} />
          <div className="flex justify-end">
            <Button variant="primary" onClick={onClose}>
              {t("done")}
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-3">
          {create.error ? <Notice tone="danger">{create.error.message}</Notice> : null}
          <label className="flex flex-col gap-1 text-ui">
            <span className="font-medium">{t("users.handle")}</span>
            <Input value={handle} autoFocus aria-invalid={handle !== "" && !validHandle(handle)} onChange={(e) => setHandle(e.target.value.trim().toLowerCase())} />
            <span className="text-meta text-fg-subtle">{t("users.handleHint")}</span>
          </label>
          <label className="flex flex-col gap-1 text-ui">
            <span className="font-medium">{t("users.displayName")}</span>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <RoleSelect value={role} onChange={setRole} />
          <PasswordField value={password} onChange={setPassword} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="submit" variant="primary" disabled={!validHandle(handle) || password.length < MIN_PASSWORD} loading={create.isPending}>
              {t("users.create")}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function RoleDialog({ row, onClose }: { row: Row; onClose: () => void }) {
  const { t } = useTranslation("admin");
  const set = useSetRole();
  const current = row.actor.role === "owner" ? "admin" : row.actor.role;
  const [role, setRole] = useState<(typeof roles)[number]>(current);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("users.roleTitle", { handle: row.actor.handle })} closeLabel={t("close")}>
      <div className="flex flex-col gap-3">
        {set.error ? <Notice tone="danger">{set.error.message}</Notice> : null}
        <RoleSelect value={role} onChange={setRole} />
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" disabled={role === row.actor.role} loading={set.isPending} onClick={() => set.mutate({ id: row.actor.id, role }, { onSuccess: onClose })}>
            {t("users.saveRole")}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

function PasswordDialog({ row, onClose }: { row: Row; onClose: () => void }) {
  const { t } = useTranslation("admin");
  const reset = useResetPassword();
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("users.passwordTitle", { handle: row.actor.handle })} closeLabel={t("close")}>
      {done ? (
        <div className="flex flex-col gap-3">
          <SecretOnce secret={password} />
          <div className="flex justify-end">
            <Button variant="primary" onClick={onClose}>
              {t("done")}
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-ui">{t("users.passwordBody")}</p>
          {reset.error ? <Notice tone="danger">{reset.error.message}</Notice> : null}
          <PasswordField value={password} onChange={setPassword} />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button variant="danger" disabled={password.length < MIN_PASSWORD} loading={reset.isPending} onClick={() => reset.mutate({ id: row.actor.id, password }, { onSuccess: () => setDone(true) })}>
              {t("users.passwordAction")}
            </Button>
          </div>
        </div>
      )}
    </Dialog>
  );
}

function StateDialog({ row, onClose }: { row: Row; onClose: () => void }) {
  const { t } = useTranslation("admin");
  const set = useSetUserState();
  const stopped = row.actor.state === "stopped";
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t(stopped ? "users.enableTitle" : "users.disableTitle", { handle: row.actor.handle })} closeLabel={t("close")}>
      <div className="flex flex-col gap-3">
        <p className="text-ui">{t(stopped ? "users.enableBody" : "users.disableBody")}</p>
        {set.error ? <Notice tone="danger">{set.error.message}</Notice> : null}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant={stopped ? "primary" : "danger"} loading={set.isPending} onClick={() => set.mutate({ id: row.actor.id, state: stopped ? "active" : "stopped" }, { onSuccess: onClose })}>
            {t(stopped ? "users.enable" : "users.disable")}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
