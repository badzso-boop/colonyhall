// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { Schemas } from "@/shared/api";
import { Button, Markdown, Select, toast } from "@/shared/ui";
import { useMessageAction } from "../api";

type Block = Schemas["MessageBlock"];

/**
 * An interactive message's blocks (round 56c): text, buttons, a select and
 * a table. A click goes to the server; what happens next (an agent's reply,
 * an extension's routine) arrives as a message.
 */
export function MessageBlocks({
  messageId,
  blocks,
  body,
  canAct,
}: {
  messageId: string;
  blocks: Block[];
  body: string;
  canAct: boolean;
}) {
  const { t } = useTranslation("conversation");
  const action = useMessageAction();
  const [done, setDone] = useState<string | null>(null);
  const [choice, setChoice] = useState<Record<string, string>>({});
  const act = (actionId: string, value?: string) =>
    action.mutate(
      { messageId, actionId, value },
      {
        onSuccess: (r) => {
          setDone(r.label);
          toast(t("blocks.sent", { label: r.label }));
        },
        onError: (e) => toast(e.message, { tone: "danger" }),
      },
    );
  return (
    <div className="mt-1 flex flex-col gap-2">
      {blocks.map((b, i) => {
        switch (b.type) {
          case "text":
            return b.text && b.text !== body ? (
              <Markdown
                key={i}
                text={b.text}
                inChat
                copyLabel={t("message.copy")}
                copiedLabel={t("message.copied")}
              />
            ) : null;
          case "buttons":
            return (
              <div key={i} className="flex flex-wrap gap-2">
                {(b.buttons ?? []).map((x) => (
                  <Button
                    key={x.action_id}
                    size="sm"
                    variant={
                      x.style === "primary"
                        ? "primary"
                        : x.style === "danger"
                          ? "danger"
                          : "secondary"
                    }
                    disabled={!canAct || action.isPending}
                    onClick={() => act(x.action_id)}
                  >
                    {x.label}
                  </Button>
                ))}
              </div>
            );
          case "select": {
            const id = b.action_id ?? "";
            return (
              <div key={i} className="flex flex-wrap items-center gap-2">
                <Select
                  aria-label={b.placeholder || t("blocks.choose")}
                  value={choice[id] ?? ""}
                  disabled={!canAct}
                  onChange={(e) =>
                    setChoice({ ...choice, [id]: e.target.value })
                  }
                >
                  <option value="">
                    {b.placeholder || t("blocks.choose")}
                  </option>
                  {(b.options ?? []).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </Select>
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={!canAct || !choice[id] || action.isPending}
                  onClick={() => act(id, choice[id])}
                >
                  {t("blocks.send")}
                </Button>
              </div>
            );
          }
          case "table":
            return (
              <div
                key={i}
                className="overflow-x-auto rounded-md border border-border"
              >
                <table className="w-full text-left text-ui">
                  <thead className="text-meta text-fg-muted">
                    <tr className="border-b border-border">
                      {(b.columns ?? []).map((c) => (
                        <th key={c} className="px-2 py-1 font-medium">
                          {c}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {(b.rows ?? []).map((row, r) => (
                      <tr key={r}>
                        {row.map((cell, c) => (
                          <td key={c} className="px-2 py-1">
                            {cell}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          default:
            return null;
        }
      })}
      {done ? (
        <p className="text-meta text-fg-muted">
          {t("blocks.done", { label: done })}
        </p>
      ) : null}
    </div>
  );
}
