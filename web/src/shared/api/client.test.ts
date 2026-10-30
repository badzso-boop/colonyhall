import { ApiError, createApi, onUnauthorized, setCsrfToken, unwrap } from "./client";

// Tests run outside a page, so the client needs an absolute base URL.
const api = createApi("http://localhost/api/v1");

afterEach(() => {
  vi.unstubAllGlobals();
});

test("calls the generated paths under /api/v1 and returns the data", async () => {
  const fetchMock = vi.fn(async (req: Request) => {
    expect(new URL(req.url).pathname).toBe("/api/v1/channels/abc/messages");
    expect(new URL(req.url).searchParams.get("limit")).toBe("20");
    return new Response(JSON.stringify({ messages: [] }), { headers: { "Content-Type": "application/json" } });
  });
  vi.stubGlobal("fetch", fetchMock);
  const data = unwrap(
    await api.GET("/channels/{channelId}/messages", {
      params: { path: { channelId: "abc" }, query: { limit: 20 } },
      fetch: fetchMock,
    }),
  );
  expect(data.messages).toEqual([]);
  expect(fetchMock).toHaveBeenCalledOnce();
});

test("unwrap throws an ApiError with the problem detail", async () => {
  const fetchMock = vi.fn(
    async () =>
      new Response(JSON.stringify({ title: "Forbidden", status: 403, detail: "not allowed in this channel" }), {
        status: 403,
        headers: { "Content-Type": "application/problem+json" },
      }),
  );
  const result = await api.GET("/me", { fetch: fetchMock });
  expect(() => unwrap(result)).toThrow(ApiError);
  try {
    unwrap(result);
  } catch (e) {
    expect((e as ApiError).status).toBe(403);
    expect((e as ApiError).message).toBe("not allowed in this channel");
  }
});

test("state-changing requests carry the CSRF token, reads do not", async () => {
  setCsrfToken("csrf-1");
  const seen: Array<[string, string | null]> = [];
  const fetchMock = vi.fn(async (req: Request) => {
    seen.push([req.method, req.headers.get("X-CSRF-Token")]);
    return new Response("{}", { headers: { "Content-Type": "application/json" } });
  });
  await api.GET("/me", { fetch: fetchMock });
  await api.POST("/auth/logout", { fetch: fetchMock });
  expect(seen).toEqual([
    ["GET", null],
    ["POST", "csrf-1"],
  ]);
  setCsrfToken("");
});

test("a 401 calls the unauthorized handler, except for the login itself", async () => {
  const handler = vi.fn();
  onUnauthorized(handler);
  const fetch401 = vi.fn(async () => new Response("{}", { status: 401, headers: { "Content-Type": "application/json" } }));
  await api.GET("/me", { fetch: fetch401 });
  expect(handler).toHaveBeenCalledOnce();
  await api.POST("/auth/login", { body: { handle: "x", password: "y" }, fetch: fetch401 });
  expect(handler).toHaveBeenCalledOnce();
  onUnauthorized(() => {});
});
