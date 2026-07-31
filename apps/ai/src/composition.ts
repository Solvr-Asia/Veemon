import { createContainer, asValue, asFunction, type AwilixContainer } from "awilix";
import type { Runnable, RunnableConfig } from "@langchain/core/runnables";
import type { BaseCheckpointSaver } from "@langchain/langgraph";
import type { ApiClient } from "@veemon/api-client";
import type { Config } from "./config";
import type { Graph } from "./application/graph";
import type { UserDirectory } from "./domain/users";
import { createApi } from "./infrastructure/api/client";
import { createApiUserDirectory } from "./infrastructure/api/user-directory";
import { createChatModel } from "./infrastructure/llm/model";
import { createCheckpointer } from "./infrastructure/checkpointer";
import { makeListUsersTool } from "./application/tools/list-users";
import { buildAssistantAgent } from "./application/agents/assistant";
import { buildUserReportWorkflow } from "./application/workflows/user-report";

export interface Container {
  checkpointer: BaseCheckpointSaver;
  agents: Record<string, Graph>;
  workflows: Record<string, Graph>;
}

// The Awilix cradle: everything registered in buildContainer, resolved by name
// via `container.cradle`. Private to this module — callers only ever see the
// public `Container` shape buildContainer returns.
interface Cradle {
  config: Config;
  checkpointer: BaseCheckpointSaver;
  api: ApiClient;
  users: UserDirectory;
  assistant: Graph;
  userReport: Graph;
}

// Adapts a compiled LangGraph runnable to the transport-facing Graph interface,
// mapping our { threadId } into LangGraph's config and requesting `updates`
// streaming. `streamMode` is a LangGraph (Pregel) option not present on the base
// RunnableConfig type, so the options object is asserted — the runtime object is
// the compiled graph, which honours it.
function toGraph(compiled: Runnable): Graph {
  return {
    invoke: (input, { threadId }) =>
      compiled.invoke(input, { configurable: { thread_id: threadId } }),
    stream: (input, { threadId }) =>
      compiled.stream(input, {
        configurable: { thread_id: threadId },
        streamMode: "updates",
      } as RunnableConfig),
  };
}

// Defers building a graph until its first use, memoizing the result. Used for
// the agent so the server can boot (and serve /health + workflows) without an
// ANTHROPIC_API_KEY — the model is only constructed when an agent is invoked,
// at which point a missing key surfaces as a clear per-request error.
function lazyGraph(factory: () => Graph): Graph {
  let built: Graph | undefined;
  const get = () => (built ??= factory());
  return {
    invoke: (input, opts) => get().invoke(input, opts),
    stream: (input, opts) => get().stream(input, opts),
  };
}

// Composition root: the one place that wires domain ports to infrastructure
// adapters and assembles the graphs. Registered into an Awilix container so
// dependencies resolve by name (container.cradle) instead of sequential
// variable wiring; everything else still depends only on interfaces.
export async function buildContainer(config: Config): Promise<Container> {
  // Awilix resolves registrations synchronously, so anything that needs an
  // await (e.g. PostgresSaver's table setup) must be built *before*
  // registration and stored with asValue — an asFunction factory can't await.
  const checkpointer = await createCheckpointer(config);

  const container: AwilixContainer<Cradle> = createContainer<Cradle>();

  container.register({
    config: asValue(config),
    checkpointer: asValue(checkpointer),

    api: asFunction(({ config }) => createApi(config)).singleton(),

    users: asFunction(({ api }) => createApiUserDirectory(api)).singleton(),

    // The agent needs the model (and thus the API key) — build it lazily so
    // the server can boot (and serve /health + workflows) without an
    // ANTHROPIC_API_KEY.
    assistant: asFunction(({ config, checkpointer, users }) =>
      lazyGraph(() =>
        toGraph(
          buildAssistantAgent({
            model: createChatModel(config),
            tools: [makeListUsersTool(users)],
            checkpointer,
          }),
        ),
      ),
    ).singleton(),

    // Workflows don't touch the model, so build eagerly.
    userReport: asFunction(({ users, checkpointer }) =>
      toGraph(buildUserReportWorkflow({ users, checkpointer })),
    ).singleton(),
  });

  const { assistant, userReport } = container.cradle;

  return {
    checkpointer,
    agents: { assistant },
    workflows: { "user-report": userReport },
  };
}
