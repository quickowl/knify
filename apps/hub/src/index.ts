import { createApp } from "./app";
import { EventHub } from "./events";
import type { HubEnv } from "./types";

export { EventHub };

export default {
  fetch(request: Request, env: HubEnv, ctx: ExecutionContext): Promise<Response> {
    return createApp(env).fetch(request, ctx);
  },

  async queue(batch: MessageBatch<{ deliveryId: string }>, env: HubEnv, ctx: ExecutionContext): Promise<void> {
    await createApp(env).queue(batch, ctx);
  }
};
