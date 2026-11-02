import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@/shared/ui";
import { EmergencyReleaseDialog } from "./EmergencyReleaseDialog";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

function setup(respond: (password: string) => Response) {
  const close = vi.fn();
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const body = (await req.clone().json()) as { password: string };
      seen.push(body.password);
      return respond(body.password);
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>
        <EmergencyReleaseDialog open onOpenChange={close} />
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return { close, seen };
}

afterEach(() => vi.unstubAllGlobals());

const submit = (password: string) => {
  fireEvent.change(screen.getByLabelText("Jelszó"), { target: { value: password } });
  fireEvent.click(screen.getByRole("button", { name: "Feloldás" }));
};

test("a lockout or rate limit shows the server's message", async () => {
  setup(() => json({ title: "Too many requests", detail: "try again in a minute", status: 429 }, 429));
  submit("barmilyen-jelszo");
  await screen.findByText(/try again in a minute/);
});

test("a wrong password stays in the dialog", async () => {
  const { close, seen } = setup(() => json({ title: "Unauthorized", detail: "wrong handle or password", status: 401 }, 401));
  submit("nem-jo-jelszo");
  await screen.findByText("Hibás jelszó.");
  expect(close).not.toHaveBeenCalled();
  expect(seen).toEqual(["nem-jo-jelszo"]);
  expect((screen.getByLabelText("Jelszó") as HTMLInputElement).value).toBe("");
});

test("the right password releases the stop and closes", async () => {
  const { close } = setup(() => json({ emergency_stop: { active: false } }));
  submit("jo-jelszo-12345");
  await waitFor(() => expect(close).toHaveBeenCalledWith(false));
});

test("no password, no request", () => {
  const { seen } = setup(() => json({}));
  expect((screen.getByRole("button", { name: "Feloldás" }) as HTMLButtonElement).disabled).toBe(true);
  expect(seen).toEqual([]);
});
