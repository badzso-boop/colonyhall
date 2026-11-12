import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { CircleHelp } from "lucide-react";
import { api, unwrap } from "@/shared/api";
import { useNow } from "@/shared/hooks";
import { Button, EmptyState, Sheet, Skeleton, Tab, TabList, TabPanel, Tabs } from "@/shared/ui";
import { questionKeys, useAnsweredQuestions, useOpenQuestions } from "../api";
import { answeredSince, isQuestion, type Question } from "../model";
import { closeQuestions, rememberAnswer, useQuestionsSheet } from "../store";
import { QuestionCard } from "./QuestionCard";

/** A question a chip asked for that is in neither list (e.g. expired): looked up among all. */
function useAnyQuestion(ref: string | null) {
  return useQuery({
    queryKey: [...questionKeys.all, "any"],
    enabled: ref !== null,
    queryFn: async () => unwrap(await api.GET("/questions")).questions,
    select: (list) => list.find((q) => ref !== null && isQuestion(q, ref)) ?? null,
  });
}

/**
 * "Válaszra vár" (round 34, part 3): a Sheet with Nyitott (to humans
 * and escalated) and Megválaszolt (last 7 days). A Q-12 chip opens it with
 * that question on top. A question answered here stays visible (answered)
 * until the Sheet closes, so the answer shows in place.
 */
export function QuestionsSheet() {
  const { t } = useTranslation("questions");
  const { open, focus, answered: answeredHere } = useQuestionsSheet();
  const now = useNow();
  const [tab, setTab] = useState("open");
  const waiting = useOpenQuestions();
  const answered = useAnsweredQuestions(open);
  const recent = answeredSince(answered.data ?? [], now);
  const focused = focus ? (waiting.data?.find((q) => isQuestion(q, focus)) ?? recent.find((q) => isQuestion(q, focus)) ?? null) : null;
  const lookup = useAnyQuestion(focus && !focused && waiting.isSuccess && answered.isSuccess ? focus : null);
  const shownFocus = focused ?? lookup.data ?? null;

  const openList = [...(waiting.data ?? [])];
  for (const q of Object.values(answeredHere)) if (!openList.some((x) => x.id === q.id)) openList.push(q);
  openList.sort((a, b) => a.created_at.localeCompare(b.created_at));
  const show = (q: Question) => answeredHere[q.id] ?? q;
  const remember = rememberAnswer;

  return (
    <Sheet open={open} onOpenChange={(o) => !o && closeQuestions()} title={t("title")} closeLabel={t("common:close")}>
      {focus ? (
        <section aria-label={t("focused")} className="mb-4 flex flex-col gap-2">
          {shownFocus ? (
            <QuestionCard question={show(shownFocus)} highlight onNavigate={closeQuestions} onAnswered={remember} />
          ) : lookup.isPending && lookup.fetchStatus !== "idle" ? (
            <Skeleton className="h-24" />
          ) : waiting.isSuccess && answered.isSuccess && !lookup.isFetching ? (
            <EmptyState title={t("notFound", { q: focus })} />
          ) : (
            <Skeleton className="h-24" />
          )}
          <Button size="sm" variant="ghost" className="self-start" onClick={() => useQuestionsSheet.setState({ focus: null })}>
            {t("showAll")}
          </Button>
        </section>
      ) : null}
      <Tabs value={tab} onValueChange={setTab}>
        <TabList label={t("title")}>
          <Tab value="open">{t("tabs.open", { count: waiting.data?.length ?? 0 })}</Tab>
          <Tab value="answered">{t("tabs.answered")}</Tab>
        </TabList>
        <TabPanel value="open" className="flex flex-col gap-3 pt-3">
          {waiting.isPending ? <Skeleton className="h-24" /> : null}
          {waiting.isSuccess && !openList.length ? <EmptyState icon={<CircleHelp className="size-8" />} title={t("empty")} /> : null}
          {openList
            .filter((q) => q.id !== shownFocus?.id)
            .map((q) => (
              <QuestionCard key={q.id} question={show(q)} onNavigate={closeQuestions} onAnswered={remember} />
            ))}
        </TabPanel>
        <TabPanel value="answered" className="flex flex-col gap-3 pt-3">
          {answered.isPending ? <Skeleton className="h-24" /> : null}
          {answered.isSuccess && !recent.length ? <EmptyState title={t("emptyAnswered")} /> : null}
          {recent.map((q) => (
            <QuestionCard key={q.id} question={q} onNavigate={closeQuestions} />
          ))}
        </TabPanel>
      </Tabs>
    </Sheet>
  );
}

/** A Q-12 chip in a message: opens the Sheet on that question. */
export function QuestionChip({ number }: { number: string }) {
  const { t } = useTranslation("questions");
  return (
    <button
      type="button"
      onClick={() => useQuestionsSheet.setState({ open: true, focus: number })}
      title={t("chipTitle", { q: number })}
      className="mx-0.5 inline-flex items-center gap-1 rounded-full border border-border bg-surface px-1.5 align-baseline font-mono text-meta font-semibold text-accent-text hover:border-border-strong"
    >
      <CircleHelp aria-hidden className="size-3" />
      {number}
    </button>
  );
}
