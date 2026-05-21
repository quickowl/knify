# Knify AgentCanvas Hub Worker

TypeScript Cloudflare Worker for the AgentCanvas v1 HTTP/SSE API.

## Local Development

```sh
npm install
npm run typecheck
npm test
HUB_TOKEN=agentcanvas-dev-token npm run start-local
```

`start-local` runs Wrangler locally, persists D1/R2/DO state under `.data-worker/` by default, and injects `HUB_TOKEN` as a local Worker var. Set `AGENTCANVAS_ADDR=127.0.0.1:8799` or `AGENTCANVAS_DATA=/tmp/hub-data` to override.

When launching from simulator or other stripped-down shells, set `AGENTCANVAS_NODE_BIN` to a Node 22+ binary so Wrangler does not fall back to an older system Node.

## Cloudflare Resources

Bindings expected by `wrangler.jsonc`:

- `DB`: D1 database named `knify_agentcanvas_hub`.
- `ASSETS_BUCKET`: R2 bucket named `knify-agentcanvas-hub-assets`.
- `AGENTCANVAS_ASSET_PUBLIC_BASE_URL`: public R2 bucket base URL. Image blocks are decorated as `<base>/assets/<assetId>`.
- `EVENTS`: Durable Object namespace backed by `EventHub`.
- `FEEDBACK_QUEUE`: Queue named `knify-agentcanvas-feedback-deliveries`.

Create or confirm resources before the first deploy:

```sh
npx wrangler d1 create knify_agentcanvas_hub
npx wrangler r2 bucket create knify-agentcanvas-hub-assets
npx wrangler queues create knify-agentcanvas-feedback-deliveries
npx wrangler d1 migrations apply knify_agentcanvas_hub --remote
```

Copy the D1 `database_id` from `wrangler d1 create/list` into `wrangler.jsonc`.

## Secrets

Do not commit production bearer tokens or Unkey root keys. Configure them as Worker secrets:

```sh
npx wrangler secret put HUB_TOKEN
npx wrangler secret put AGENTCANVAS_UNKEY_ROOT_KEY
```

Hosted browser clients should usually use Unkey API keys scoped by workspace metadata. The static `HUB_TOKEN` remains for internal automation.

## Deploy

```sh
npm run deploy
```

This port intentionally does not commit a Cloudflare `account_id` or custom `routes` block. Add those in your deployment environment before deploying. After deploy, verify:

```sh
curl -fsS https://<hub-host>/v1/healthz
```

Then run an API smoke: post a canvas, open `/v1/events`, upload/fetch an asset, and create/exchange/self-test a viewer link.

## Delivery Boundary

The Worker stores feedback and emits SSE for all providers. Active delivery is limited to edge-safe HTTP targets: `cursor`, `webhook`, and `generic_cloud`. Local command providers are not executed by Cloudflare Workers.
