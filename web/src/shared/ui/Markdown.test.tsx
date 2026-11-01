import { fireEvent, render, screen } from "@testing-library/react";
import { Markdown } from "./Markdown";

const labels = { copyLabel: "Másolás", copiedLabel: "Másolva" };

test("renders the formatting as elements", () => {
  const { container } = render(<Markdown text={"**bold** _em_ ~~del~~ `code`\n\n- a\n- b\n\n> q"} {...labels} />);
  expect(container.querySelector("strong")?.textContent).toBe("bold");
  expect(container.querySelector("em")?.textContent).toBe("em");
  expect(container.querySelector("del")?.textContent).toBe("del");
  expect(container.querySelector("code")?.textContent).toBe("code");
  expect(container.querySelectorAll("li")).toHaveLength(2);
  expect(container.querySelector("blockquote")?.textContent).toBe("q");
});

test("safety: no HTML, no images, only http(s) links with noopener", () => {
  const { container } = render(
    <Markdown text={'<img src=x onerror=alert(1)> <script>x</script> [bad](javascript:alert(1)) ![pic](https://evil/p.png) [ok](https://example.com)'} {...labels} />,
  );
  expect(container.querySelector("img")).toBeNull();
  expect(container.querySelector("script")).toBeNull();
  const links = [...container.querySelectorAll("a")];
  expect(links.every((a) => /^https:/.test(a.getAttribute("href") ?? ""))).toBe(true);
  const ok = links.find((a) => a.textContent === "ok")!;
  expect(ok.getAttribute("rel")).toBe("noopener noreferrer");
  expect(ok.getAttribute("target")).toBe("_blank");
  expect(container.textContent).toContain("<script>x</script>");
});

test("mentions become chips only for known handles", () => {
  const onMention = vi.fn();
  render(<Markdown text="@norbi and @nobody" knownHandles={new Set(["norbi"])} onMention={onMention} {...labels} />);
  fireEvent.click(screen.getByRole("button", { name: "@norbi" }));
  expect(onMention).toHaveBeenCalledWith("norbi");
  expect(screen.queryByRole("button", { name: "@nobody" })).toBeNull();
});

test("file references become buttons only with a handler, and can be refused", () => {
  const onFileClick = vi.fn();
  const { rerender } = render(<Markdown text="see src/main.go:12-20" {...labels} />);
  expect(screen.queryByRole("button")).toBeNull();
  rerender(<Markdown text="see src/main.go:12-20" onFileClick={onFileClick} {...labels} />);
  fireEvent.click(screen.getByRole("button", { name: /main.go/ }));
  expect(onFileClick).toHaveBeenCalledWith(expect.objectContaining({ path: "src/main.go", startLine: 12, endLine: 20 }));
  rerender(<Markdown text="see src/main.go:12-20" onFileClick={onFileClick} canOpenFile={() => false} {...labels} />);
  expect(screen.queryByRole("button")).toBeNull();
});

test("references become chips only with renderChip, never inside code", () => {
  const id = "0b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f21";
  const text = `**Q-12** asks; see D-3 and \`Q-99\` [error:${id}] [approval:${id}]`;
  const { container, rerender } = render(<Markdown text={text} {...labels} />);
  expect(container.textContent).toContain("Q-12");
  expect(container.querySelector("[data-chip]")).toBeNull();
  rerender(
    <Markdown text={text} renderChip={(c) => (c.kind === "approval" ? null : <span data-chip={c.kind}>{c.ref}</span>)} {...labels} />,
  );
  const chips = [...container.querySelectorAll("[data-chip]")].map((e) => `${e.getAttribute("data-chip")}:${e.textContent}`);
  expect(chips).toEqual(["question:Q-12", "decision:D-3", `error:${id}`]);
  expect(container.querySelector("strong [data-chip]")).toBeTruthy();
  expect(container.querySelector("code")?.textContent).toBe("Q-99");
  expect(container.textContent).not.toContain("approval:");
});

test("code blocks keep their text and have a copy button", () => {
  const { container } = render(<Markdown text={"```go\nfmt.Println(\"<b>\")\n```"} {...labels} />);
  expect(container.querySelector("pre code")?.textContent).toBe('fmt.Println("<b>")');
  expect(screen.getByRole("button", { name: "Másolás" })).toBeTruthy();
});
