import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { Avatar } from "./Avatar";
import { Badge } from "./Badge";
import { Button } from "./Button";
import { Checkbox } from "./fields";
import { HoldButton } from "./HoldButton";
import { IconButton } from "./IconButton";
import { Dialog } from "./overlays";
import { toast, useToasts } from "./toast";
import { Toaster } from "./Toaster";

test("Button: variant, loading disables and marks busy", () => {
  const onClick = vi.fn();
  const { rerender } = render(<Button variant="primary" onClick={onClick}>Mentés</Button>);
  const btn = screen.getByRole("button", { name: "Mentés" });
  expect(btn.className).toContain("bg-accent");
  fireEvent.click(btn);
  expect(onClick).toHaveBeenCalledOnce();
  rerender(<Button variant="primary" loading onClick={onClick}>Mentés</Button>);
  expect(btn).toHaveProperty("disabled", true);
  expect(btn.getAttribute("aria-busy")).toBe("true");
});

test("IconButton: the label is the accessible name, the badge is part of it", () => {
  render(<IconButton label="Válaszra vár" icon={<span />} badge={2} />);
  expect(screen.getByRole("button", { name: "Válaszra vár (2)" })).toBeTruthy();
});

test("Avatar: humans are circles, agents rounded squares with a ring", () => {
  render(
    <>
      <Avatar kind="human" name="Norbi" />
      <Avatar kind="agent" name="backend-claude" emoji="🛠️" status="working" ringColor="var(--color-info)" />
    </>,
  );
  const human = screen.getByRole("img", { name: "Norbi" });
  const agent = screen.getByRole("img", { name: "backend-claude" });
  expect(human.className).toContain("rounded-full");
  expect(human.textContent).toBe("N");
  expect(agent.className).toContain("rounded-md");
  expect(agent.style.boxShadow).toContain("var(--color-info)");
  expect(agent.querySelector("[data-status=working]")).toBeTruthy();
});

test("Badge renders its content", () => {
  render(<Badge tone="success" mono>M</Badge>);
  expect(screen.getByText("M").className).toContain("font-mono");
});

test("Checkbox toggles", () => {
  const onChange = vi.fn();
  render(<Checkbox aria-label="Kész" onCheckedChange={onChange} />);
  fireEvent.click(screen.getByRole("checkbox", { name: "Kész" }));
  expect(onChange).toHaveBeenCalledWith(true);
});

describe("HoldButton", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test("confirms only after holding for the whole time", () => {
    const onConfirm = vi.fn();
    render(<HoldButton label="Mind áll" hint="Nyomd és tartsd" holdMs={2000} onConfirm={onConfirm} />);
    const btn = screen.getByRole("button", { name: "Mind áll" });

    fireEvent.pointerDown(btn);
    act(() => vi.advanceTimersByTime(1500));
    fireEvent.pointerUp(btn);
    act(() => vi.advanceTimersByTime(1000));
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.pointerDown(btn);
    act(() => vi.advanceTimersByTime(2100));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  test("works by holding Space", () => {
    const onConfirm = vi.fn();
    render(<HoldButton label="Mind áll" hint="Nyomd és tartsd" holdMs={1000} onConfirm={onConfirm} />);
    const btn = screen.getByRole("button", { name: "Mind áll" });
    fireEvent.keyDown(btn, { key: " " });
    act(() => vi.advanceTimersByTime(1100));
    expect(onConfirm).toHaveBeenCalledOnce();
  });
});

test("Dialog: opens with its title, closes with Esc and with the close button", () => {
  function Host() {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>Megnyitás</button>
        <Dialog open={open} onOpenChange={setOpen} title="Új csatorna" closeLabel="Bezárás">
          <input aria-label="Név" />
        </Dialog>
      </>
    );
  }
  render(<Host />);
  fireEvent.click(screen.getByRole("button", { name: "Megnyitás" }));
  expect(screen.getByRole("dialog", { name: "Új csatorna" })).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "Megnyitás" }));
  fireEvent.click(screen.getByRole("button", { name: "Bezárás" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

describe("toast", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    useToasts.setState({ items: [] });
  });

  test("shows, runs its action, keeps at most three, and expires", () => {
    const run = vi.fn();
    render(<Toaster closeLabel="Bezárás" />);
    act(() => {
      toast("Elküldve", { action: { label: "Visszavonás", run }, ms: 10000 });
    });
    fireEvent.click(screen.getByRole("button", { name: "Visszavonás" }));
    expect(run).toHaveBeenCalledOnce();
    expect(screen.queryByText("Elküldve")).toBeNull();

    act(() => {
      for (const t of ["a", "b", "c", "d"]) toast(t, { ms: 1000 });
    });
    expect(screen.getAllByRole("status").map((e) => e.textContent?.replace("Bezárás", ""))).toEqual(["b", "c", "d"]);
    act(() => vi.advanceTimersByTime(1100));
    expect(screen.queryAllByRole("status")).toHaveLength(0);
  });
});
