import { ApiError, createApi, unwrap } from "./client";

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
