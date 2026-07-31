import { describe, it, expect, afterEach } from "bun:test";
import { MemorySaver } from "@langchain/langgraph";
import type { Config } from "./config";
import { buildContainer } from "./composition";

function testConfig(): Config {
  return {
    port: 4111,
    anthropicApiKey: undefined,
    model: { name: "claude-opus-4-8", maxTokens: 8192 },
    api: { baseUrl: "http://x", serviceToken: null },
    checkpointer: { kind: "memory" },
  };
}

const originalFetch = globalThis.fetch;

describe("buildContainer (Awilix composition root)", () => {
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("resolves the checkpointer and the registered agent/workflow names", async () => {
    const container = await buildContainer(testConfig());

    // Checks the *concrete* wiring (a real MemorySaver for this config), not
    // just that some object with the right shape came back.
    expect(container.checkpointer).toBeInstanceOf(MemorySaver);
    expect(Object.keys(container.agents)).toEqual(["assistant"]);
    expect(Object.keys(container.workflows)).toEqual(["user-report"]);
  });

  it("wires api -> user directory -> workflow end-to-end through the container", async () => {
    // Stub the one thing the resolved dependency chain actually talks to
    // (fetch), then invoke the resolved workflow and check its output — this
    // proves the container really wired api/users into the workflow, not just
    // that buildContainer() didn't throw.
    globalThis.fetch = (async () =>
      new Response(
        JSON.stringify({
          success: true,
          data: [{ id: "1", email: "a@b.c", name: "A", phone: "", status: "active", createdAt: "" }],
          meta: { page: 2, size: 5, total: 10, totalPages: 2 },
        }),
        { headers: { "Content-Type": "application/json" } },
      )) as unknown as typeof fetch;

    const container = await buildContainer(testConfig());
    const result = (await container.workflows["user-report"]!.invoke(
      { page: 2, size: 5 },
      { threadId: "t1" },
    )) as { count: number; total?: number };

    expect(result.count).toBe(1);
    expect(result.total).toBe(10); // echoed from the stubbed response's meta
  });
});
