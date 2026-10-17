# Gongji (共绩算力) CoreDeploymentProvider

Independent OpenAPI provider peer to Novita/RunPod. **Not** a Kubernetes backend and **not** SkyPilot-only.

## API mapping (v1 `https://openapi.suanli.cn`)

| CoreDeploymentProvider | Gongji OpenAPI | Notes |
|------------------------|----------------|-------|
| `Deploy` | `POST /api/deployment/task/create` | Sets `dynamic_pod_strategy.switch=false` |
| `ScaleApp` | `POST /api/deployment/task/change_points` `{task_id, points}` | **Only** scale write path |
| `GetApp` / `GetAppStatus` | `POST /api/deployment/task/detail` | desired=`points`, ready=`runing_points` |
| `ListWorkers` / points | `GET /api/deployment/task/points` | Maps `worker_id` ↔ `point_id` |
| `TerminateWorker` | `POST /api/deployment/task/delete_pod` `{point_id}` | Targeted remove |
| `DeleteApp` | `POST /api/deployment/task/stop` `{task_id}` | Lifecycle teardown |

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

Never log token or signing_key.

## Autoscaler attach

With `autoscaler.enabled=true` and `gongji.enabled=true` (or `providers.deployment=gongji`), Autoscaler starts **without** `k8s.enabled`. Executor calls `ScaleApp` only.
