import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Bell, MoreHorizontal, Search, ShieldAlert, Trash2 } from "lucide-react";
import { applyTheme, readTheme, themes, type Theme } from "@/shared/theme";
import {
  Avatar,
  Badge,
  BrandMark,
  Button,
  Checkbox,
  Dialog,
  EmptyState,
  HoldButton,
  IconButton,
  Input,
  Kbd,
  Menu,
  MenuContent,
  MenuItem,
  MenuSeparator,
  MenuTrigger,
  Select,
  Sheet,
  Skeleton,
  Switch,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Textarea,
  Tooltip,
  toast,
} from "@/shared/ui";

// The colour tokens, in the order of tokens.css.
const colourTokens = [
  "bg", "surface", "surface-raised", "surface-sunken", "border", "border-strong",
  "fg", "fg-muted", "fg-subtle", "accent", "accent-hover", "accent-fg", "accent-soft", "accent-text",
  "danger", "danger-soft", "warning", "warning-soft", "success", "success-soft", "info", "info-soft",
  "terminal-bg", "terminal-fg",
];

const typeScale = ["display", "page", "title", "body", "ui", "meta"] as const;
const typeClass: Record<(typeof typeScale)[number], string> = {
  display: "text-display font-semibold",
  page: "text-page font-semibold",
  title: "text-title font-semibold",
  body: "text-body",
  ui: "text-ui",
  meta: "text-meta",
};

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 border-t border-border pt-6">
      <h2 className="text-page font-semibold">{title}</h2>
      {children}
    </section>
  );
}

/** The living style guide at /app/design (docs/07-design-system.md). */
export default function DesignPage() {
  const { t } = useTranslation("design");
  const [theme, setTheme] = useState<Theme>(readTheme);
  const [dialog, setDialog] = useState(false);
  const [sheet, setSheet] = useState(false);
  const close = t("common:close");

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto flex max-w-5xl flex-col gap-6 px-4 py-8">
        <header className="flex flex-wrap items-center gap-3">
          <BrandMark className="size-8" />
          <h1 className="flex-1 text-display font-semibold">{t("title")}</h1>
          <Select
            aria-label={t("common:theme.label")}
            className="w-36"
            value={theme}
            onChange={(e) => {
              const next = e.target.value as Theme;
              setTheme(next);
              applyTheme(next);
            }}
          >
            {themes.map((th) => (
              <option key={th} value={th}>
                {t(`common:theme.${th}`)}
              </option>
            ))}
          </Select>
        </header>
        <p className="text-fg-muted">{t("intro")}</p>

        <Section title={t("sections.colours")}>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-6">
            {colourTokens.map((name) => (
              <div key={name} className="flex flex-col gap-1">
                <span className="h-12 rounded-md border border-border" style={{ background: `var(--color-${name})` }} />
                <code className="text-meta text-fg-muted">{name}</code>
              </div>
            ))}
          </div>
        </Section>

        <Section title={t("sections.type")}>
          {typeScale.map((k) => (
            <div key={k} className="flex items-baseline gap-4">
              <span className="w-32 shrink-0 text-meta text-fg-subtle">{t(`typeNames.${k}`)}</span>
              <span className={typeClass[k]}>{t("sample.body")}</span>
            </div>
          ))}
          <div className="flex items-baseline gap-4">
            <span className="w-32 shrink-0 text-meta text-fg-subtle">JetBrains Mono</span>
            <code className="font-mono text-ui">{t("sample.mono")}</code>
          </div>
        </Section>

        <Section title={t("sections.buttons")}>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="primary">{t("sample.save")}</Button>
            <Button variant="secondary">{t("sample.cancel")}</Button>
            <Button variant="ghost">{t("sample.ghost")}</Button>
            <Button variant="danger" icon={<Trash2 className="size-4" />}>{t("sample.delete")}</Button>
            <Button variant="primary" loading>{t("sample.loading")}</Button>
            <Button variant="secondary" size="sm">{t("sample.cancel")}</Button>
            <IconButton label={t("sample.notify")} icon={<Bell className="size-5" />} badge={2} />
            <IconButton label={t("sample.search")} icon={<Search className="size-4" />} size="sm" />
          </div>
        </Section>

        <Section title={t("sections.fields")}>
          <div className="grid max-w-xl gap-3">
            <Input placeholder={t("sample.search")} aria-label={t("sample.search")} />
            <Textarea placeholder={t("sample.message")} aria-label={t("sample.message")} />
            <Select aria-label={t("sample.select")} defaultValue="general">
              <option value="general">#general</option>
              <option value="backend">#backend</option>
            </Select>
            <label className="flex items-center gap-2 text-ui">
              <Checkbox defaultChecked /> {t("sample.done")}
            </label>
            <label className="flex items-center gap-2 text-ui">
              <Switch defaultChecked /> {t("sample.notify")}
            </label>
          </div>
        </Section>

        <Section title={t("sections.badges")}>
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="accent">3</Badge>
            <Badge>12</Badge>
            <Badge tone="success">{t("sample.badgeDone")}</Badge>
            <Badge tone="warning">{t("sample.badgeRunning")}</Badge>
            <Badge tone="danger">{t("sample.badgeError")}</Badge>
            <Badge tone="info">{t("sample.badgeInfo")}</Badge>
            {["M", "A", "D", "?", "R"].map((g) => (
              <Badge key={g} mono tone={g === "A" ? "success" : g === "D" ? "danger" : g === "M" ? "warning" : g === "R" ? "info" : "neutral"}>
                {g}
              </Badge>
            ))}
          </div>
        </Section>

        <Section title={t("sections.avatars")}>
          <div className="flex flex-wrap items-center gap-4">
            <Avatar kind="human" name="Norbi" size="lg" />
            <Avatar kind="human" name="Anna" />
            <Avatar kind="agent" name="backend-claude" emoji="🛠️" size="lg" status="working" ringColor="var(--color-info)" />
            <Avatar kind="agent" name="docs-agy" emoji="📚" status="idle" ringColor="var(--color-success)" />
            <Avatar kind="agent" name="reviewer" emoji="🔍" status="offline" />
            <Avatar kind="agent" name="paused" emoji="⏸" status="paused" size="sm" />
          </div>
        </Section>

        <Section title={t("sections.menus")}>
          <div className="flex flex-wrap items-center gap-3">
            <Menu>
              <MenuTrigger asChild>
                <Button icon={<MoreHorizontal className="size-4" />}>{t("sample.menu")}</Button>
              </MenuTrigger>
              <MenuContent align="start">
                <MenuItem>{t("sample.menuReply")}</MenuItem>
                <MenuItem>{t("sample.menuCopy")}</MenuItem>
                <MenuSeparator />
                <MenuItem danger>{t("sample.menuRetract")}</MenuItem>
              </MenuContent>
            </Menu>
            <Tooltip content={t("sample.tooltipText")}>
              <Button variant="ghost">{t("sample.tooltip")}</Button>
            </Tooltip>
            <span className="flex items-center gap-1 text-ui text-fg-muted">
              <Kbd>Ctrl</Kbd> <Kbd>K</Kbd>
            </span>
          </div>
          <Tabs defaultValue="a" className="max-w-xl">
            <TabList label={t("sections.menus")}>
              <Tab value="a">{t("sample.tabA")}</Tab>
              <Tab value="b">{t("sample.tabB")}</Tab>
            </TabList>
            <TabPanel value="a" className="py-3 text-ui">{t("sample.tabAText")}</TabPanel>
            <TabPanel value="b" className="py-3 text-ui">{t("sample.tabBText")}</TabPanel>
          </Tabs>
        </Section>

        <Section title={t("sections.overlays")}>
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => setDialog(true)}>{t("sample.openDialog")}</Button>
            <Button onClick={() => setSheet(true)}>{t("sample.openSheet")}</Button>
          </div>
          <Dialog
            open={dialog}
            onOpenChange={setDialog}
            title={t("sample.dialogTitle")}
            description={t("sample.dialogBody")}
            closeLabel={close}
            footer={
              <>
                <Button onClick={() => setDialog(false)}>{t("sample.cancel")}</Button>
                <Button variant="primary" onClick={() => setDialog(false)}>{t("sample.save")}</Button>
              </>
            }
          >
            <Input aria-label={t("sample.dialogTitle")} placeholder="#release" />
          </Dialog>
          <Sheet open={sheet} onOpenChange={setSheet} title={t("sample.sheetTitle")} closeLabel={close}>
            <p className="text-ui text-fg-muted">{t("sample.sheetBody")}</p>
          </Sheet>
        </Section>

        <Section title={t("sections.feedback")}>
          <div className="flex flex-wrap items-center gap-3">
            <Button onClick={() => toast(t("sample.toast"), { action: { label: t("sample.undo"), run: () => undefined } })}>
              {t("sample.toastButton")}
            </Button>
            <HoldButton
              label={t("sample.hold")}
              hint={t("sample.holdHint")}
              icon={<ShieldAlert className="size-4" />}
              onConfirm={() => toast(t("sample.held"), { tone: "danger" })}
            />
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="rounded-lg border border-border bg-surface">
              <EmptyState icon={<Bell className="size-8" />} title={t("sample.emptyTitle")} body={t("sample.emptyBody")} />
            </div>
            <div className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-4">
              <Skeleton className="h-4 w-1/3" />
              <Skeleton />
              <Skeleton className="h-4 w-5/6" />
            </div>
          </div>
        </Section>

        <Section title={t("sections.brand")}>
          <div className="flex flex-wrap items-center gap-4">
            <div className="grid size-20 place-items-center rounded-lg bg-terminal-bg">
              <BrandMark className="size-12" />
            </div>
            <div className="grid size-20 place-items-center rounded-lg border border-border bg-surface">
              <BrandMark className="size-12" />
            </div>
          </div>
        </Section>
      </div>
    </div>
  );
}
