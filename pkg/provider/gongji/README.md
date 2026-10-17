# Gongji (共绩算力) CoreDeploymentProvider

Independent OpenAPI provider peer to Novita/RunPod. **Not** a Kubernetes backend and **not** SkyPilot-only.

## API mapping (v1 `https://openapi.suanli.cn`)

| CoreDeploymentProvider | Gongji OpenAPI | Notes |
|------------------------|----------------|-------|
| `Deploy` | `POST /api/deployment/task/create` | Sets `dynamic_pod_strategy.switch=false`. `resources[].mark` comes from resource search or the logical AZ catalog. |
| `ScaleApp` | `POST /api/deployment/task/change_points` `{task_id, points}` | **Only** scale write path |
| `GetApp` / `GetAppStatus` | `GET /api/deployment/task/detail?task_id=` | desired=`points`, ready=`runing_points` |
| Pause / Recover | `POST /api/deployment/task/pause` / `recover` | Pause releases vendor points; recover resumes |
| `ListWorkers` / points | `GET /api/deployment/task/points` | Maps `worker_id` ↔ `point_id` |
| `TerminateWorker` | `POST /api/deployment/task/delete_pod` `{point_id}` | Targeted remove |
| `DeleteApp` | `POST /api/deployment/task/stop` `{task_id}` | Lifecycle teardown. Vendor delete is not recoverable |
| Region inventory | `GET /api/deployment/resource/search` | Polls marks, inventory, and price. Failure marks the snapshot stale, never zero |
| Job create / list / detail / stop | `POST /api/task/job/create`, `GET /api/task/job/search`, `GET /api/task/job/detail`, `POST /api/task/job/stop` | Batch job lifecycle. Image preheat is not in this path |

Auth: Header `token` + `version` + `timestamp` (ms); optional RSA-SHA256 `sign_str` when `signing_key` is set.

## Credentials (env / Secret)

| Env | Config field | Required |
|-----|--------------|----------|
| `GONGJI_ENABLED` | `gongji.enabled` | yes to activate |
| `GONGJI_TOKEN` | `gongji.token` | yes |
| `GONGJI_SIGNING_KEY` | `gongji.signing_key` | RSA PEM for sign mode |
| `GONGJI_BASE_URL` | `gongji.base_url` | default `https://openapi.suanli.cn` |
| `GONGJI_TENANT_ID` | `gongji.tenant_id` | for points list |
| `GONGJI_DEFAULT_RESOURCE_MARK` | `gongji.default_resource_mark` | Deploy resources[].mark |
| `GONGJI_DEFAULT_REGION_NAME` | `gongji.default_region_name` | optional |
| `GONGJI_REQUEST_TIMEOUT` | `gongji.request_timeout` | seconds |
| `GONGJI_POLL_INTERVAL` | `gongji.poll_interval` | seconds |
| `GONGJI_KEY_ID` | `gongji.key_id` | ops rotation id |
| `GONGJI_API_VERSION` | `gongji.api_version` | default `1.0.0` |
| `GONGJI_REGIONS_FILE` | `gongji.regions_file` | logical AZ registry, default `config/regions.yaml` |
| `GONGJI_REGION_STATE_FILE` | `gongji.region_state_file` | maintenance overlay when metadata DB is off |
| `GONGJI_INVENTORY_FRESH_SECONDS` | `gongji.inventory_fresh_seconds` | how long a successful pull stays sellable |

When `providers.metadata` is PostgreSQL (default), logical AZ bindings and every inventory sync are persisted in the metadata database (`provider_region_registry_meta`, `provider_region_bindings`, `provider_inventory_pulls`, `provider_inventory_offers`). Apply `migrations/006_gongji_provider_inventory.sql` on existing clusters, or use an updated `scripts/init_postgres.sql` for new installs. `config/regions.yaml` seeds bindings only when the DB registry is empty.

Never log token or signing_key.

## Admin API

When `gongji.enabled=true`, waverless exposes `/api/v1/gongji` on the existing admin router:

- `GET /regions` returns logical AZs only. `view=ops` adds physical region, inventory, price, and resource mark.
- `GET /regions/version` returns `registryVersion` (binding changes) and monotonic inventory pull versions.
- `GET /inventory/pulls` lists sync history (`success` / `failed`) with `page` / `pageSize`.
- `GET /resources` and `POST /resources/refresh` read and refresh the vendor snapshot.
- `PUT /regions/:logicalAzId/availability` sets `schedulable|maintenance|paused|retired` without calling Gongji or terminating workers.
- `PUT /regions/bindings` binds a logical AZ to a physical region and SKU that the latest snapshot already contains.
- `/serverless/tasks` covers create, detail, list, update, pause, recover, absolute `points`, delete, and point list/delete.
- `/jobs` covers create, list, detail, and stop.

Logical AZ definitions live in `config/regions.example.yaml`. Copy it to `config/regions.yaml`. A failed inventory pull keeps the last count and marks it stale; it does not report zero.

## Autoscaler attach

With `autoscaler.enabled=true` and `gongji.enabled=true` (or `providers.deployment=gongji`), Autoscaler starts **without** `k8s.enabled`. Executor calls `ScaleApp` only.
