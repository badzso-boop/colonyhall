import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { closeQuestions, openQuestions } from "../store";
import { QuestionChip, QuestionsSheet } from "./QuestionsSheet";

type Q = Schemas["Question"];
const q = (over: Partial<Q>): Q => ({
  id: "q12", number: 12, formatted_number: "Q-12", workspace_id: "w", asked_by: "ag", asked_by_handle: "backend-claude",
  to_human: true, channel_id: "c1", channel_name: "general", thread_root_id: "t1", text: "Melyik adatbázis?", options: ["Postgres", "SQLite"],
  default_option: "Postgres", deadline: new Date(Date.now() + 3_600_000).toISOString(), state: "open", created_at: new Date().toISOString(), ...over,
});

function setup(lists: { open: Q[]; escalated?: Q[]; answered?: Q[]; all?: Q[] }) {
  const posts: Array<{ path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });
      if (req.method === "POST") {
        const body = await req.clone().json();
        posts.push({ path: u.pathname, body });
        return json({ ...lists.open[0], state: "answered", answer: (body as { answer: string }).answer, answered_by_handle: "norbi", answered_at: new Date().toISOString() });
      }
      if (u.pathname.endsWith("/actors")) return json({ actors: [] });
      const state = u.searchParams.get("state");
      if (state === "open") return json({ questions: lists.open });
      if (state === "escalated") return json({ questions: lists.escalated ?? [] });
      if (state === "answered") return json({ questions: lists.answered ?? [] });
      return json({ questions: lists.all ?? [] });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map());
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <QuestionChip number="Q-3" />
        <QuestionsSheet />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { posts };
}

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => closeQuestions());
});

test("open questions with options, default, countdown and thread link; an answer shows in place", async () => {
  const { posts } = setup({ open: [q({})], escalated: [q({ id: "q5", number: 5, formatted_number: "Q-5", to_human: false, state: "escalated", options: [] })] });
  act(() => openQuestions());
  const card = await screen.findByRole("article", { name: "Q-12" });
  expect(within(card).getByText("Melyik adatbázis?")).toBeTruthy();
  expect(within(card).getByText("Alapértelmezés: Postgres")).toBeTruthy();
  expect(within(card).getByText(/határidő: (59:5\d|1:00:00)/)).toBeTruthy();
  expect(within(card).getByRole("link", { name: /#general/ }).getAttribute("href")).toBe("/c/c1?thread=t1");
  expect(screen.getByRole("article", { name: "Q-5" }).textContent).toContain("eszkalálva");
  expect(screen.getByRole("tab", { name: "Nyitott (2)" })).toBeTruthy();

  fireEvent.click(within(card).getByRole("button", { name: "SQLite" }));
  await waitFor(() => expect(posts).toEqual([{ path: "/api/v1/questions/q12/answer", body: { answer: "SQLite" } }]));
  await waitFor(() => expect(screen.getByRole("article", { name: "Q-12" }).textContent).toContain("@norbi válaszolt"));
  expect(screen.getByRole("article", { name: "Q-12" }).textContent).toContain("SQLite");
});

test("a free-text answer", async () => {
  const { posts } = setup({ open: [q({ options: [] })] });
  act(() => openQuestions());
  const box = await screen.findByRole("textbox", { name: "Válasz erre: Q-12" });
  fireEvent.change(box, { target: { value: "  Postgres 16  " } });
  fireEvent.click(screen.getByRole("button", { name: "Válasz" }));
  await waitFor(() => expect(posts[0]?.body).toEqual({ answer: "Postgres 16" }));
});

test("a Q chip opens the Sheet on that question, looking among all when it is closed", async () => {
  setup({ open: [q({})], all: [q({ id: "q3", number: 3, formatted_number: "Q-3", state: "expired", text: "Régi kérdés" })] });
  fireEvent.click(screen.getByRole("button", { name: "Q-3" }));
  const focused = await screen.findByRole("region", { name: "A kért kérdés" });
  await waitFor(() => expect(within(focused).getByText("Régi kérdés")).toBeTruthy());
  expect(within(focused).getByText("lejárt")).toBeTruthy();
  expect(within(focused).queryByRole("button", { name: "Válasz" })).toBeNull();
});
