// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { fireEvent, render, screen } from "@testing-library/react";
import { SideSlot } from "./SideSlot";

// jsdom does no layout: the slot is as wide as its style, the main column 900px.
beforeEach(() => {
  localStorage.clear();
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(function (this: HTMLElement) {
    return this.tagName === "MAIN" ? 900 : parseInt(this.style.width || "0", 10);
  });
});
afterEach(() => vi.restoreAllMocks());

function setup(content: "thread" | "files") {
  return render(
    <div>
      <main />
      <SideSlot side="right" content={content} width="wide">
        <p>pane</p>
      </SideSlot>
    </div>,
  );
}

const slotWidth = () => (screen.getByRole("separator").parentElement as HTMLElement).style.width;

describe("SideSlot", () => {
  it("opens a file browser wider than a thread", () => {
    const { unmount } = setup("thread");
    expect(slotWidth()).toBe("440px");
    unmount();
    setup("files");
    expect(slotWidth()).toBe("760px");
  });

  it("widens with the keyboard, remembers it, and keeps the chat its room", () => {
    setup("thread");
    const sep = screen.getByRole("separator");
    fireEvent.keyDown(sep, { key: "ArrowLeft" });
    expect(slotWidth()).toBe("464px");
    expect(JSON.parse(localStorage.getItem("colony.pane.widths") ?? "{}")).toEqual({ "wide.thread": 464 });
    fireEvent.keyDown(sep, { key: "End" });
    // 464 + 900 shared, the chat keeps 360.
    expect(slotWidth()).toBe("1004px");
    fireEvent.doubleClick(sep);
    expect(slotWidth()).toBe("440px");
  });
});
