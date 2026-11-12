import { render, screen } from "@testing-library/react";
import type { SearchStatus } from "../model";
import { SearchStatusCard } from "./SearchStatusCard";

const search = (over: Partial<SearchStatus> = {}): SearchStatus => ({
  chunks: 1234,
  pending: 5,
  last_embed_at: "2026-11-10T08:00:00Z",
  models: ["bge-m3"],
  embedder: { configured: true, reachable: true, model: "bge-m3", last_ok_at: "2026-11-10T08:00:00Z" },
  reranker: { configured: false, reachable: false },
  ...over,
});

const card = () => screen.getByRole("heading", { level: 2, name: /Keresés/ }).parentElement!;

test("a reachable embedder: ok, with the index numbers and the model", () => {
  render(<SearchStatusCard search={search()} />);
  expect(card().textContent).toContain("rendben");
  expect(card().textContent).toContain("Beágyazásra vár5");
  expect(card().textContent).toContain("Tárolt modellekbge-m3");
  expect(card().textContent).toContain("Beágyazó szerver (embedding)");
  expect(screen.queryByRole("alert")).toBeNull();
});

test("an unreachable backend warns and shows its last error", () => {
  render(<SearchStatusCard search={search({ embedder: { configured: true, reachable: false, model: "bge-m3", last_error: "connection refused" } })} />);
  expect(card().textContent).toContain("figyelmeztetés");
  expect(screen.getByText("connection refused")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});

test("a model mismatch is an error with a clear warning", () => {
  render(<SearchStatusCard search={search({ model_mismatch: "stored bge-m3, served nomic-embed" })} />);
  expect(card().textContent).toContain("hiba");
  expect(screen.getByRole("alert").textContent).toContain("Modelleltérés");
  expect(screen.getByRole("alert").textContent).toContain("stored bge-m3, served nomic-embed");
});
