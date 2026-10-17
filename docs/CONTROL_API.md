# Control API (`/control/v1`)

Service-to-service HTTP surface for **gpurun/cloud** (`apps/gpus`). All routes require a platform bearer token and use JSON bodies with **camelCase** field names matching the Cloud TypeScript client.

## Authentication

Set a dedicated service key on the waverless server:

| Source | Variable / config |
|--------|-------------------|
| Environment (preferred) | `WAVERLESS_SERVICE_API_KEY` or `WAVERLESS_SERVICE_SECRET` |
| Config file | `server.service_api_key` |
| Fallback | `server.api_key` when service key is unset |

Send on every request:

```http
Authorization: Bearer <service-api-key>
Content-Type: application/json
```

Optional headers: `X-Request-Id`, `Idempotency-Key` (lifecycle POSTs).

If no service key is configured, `/control/v1` returns `401` (control plane stays closed).

## Base URL

Cloud uses `WAVERLESS_BASE_URL` (alias `WAVERLESS_CONTROL_API_URL`). Paths below are relative to that origin.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/control/v1/endpoints` | Create endpoint (M1 placement: `placementMode` `auto` \| `pin`) |
| `GET` | `/control/v1/endpoints/{externalEndpointId}/runtime` | Runtime sync by Cloud `computeBaseId` |
| `GET` | `/control/v1/endpoints/{externalEndpointId}/workers` | Paginated workers (`page`, `pageSize`) |
| `PATCH` | `/control/v1/endpoints/{externalEndpointId}` | Patch elastic bounds / region allowlist |
| `POST` | `/control/v1/endpoints/{externalEndpointId}/scale` | Update min/max replicas and reconcile desired count |
| `DELETE` | `/control/v1/endpoints/{externalEndpointId}` | Delete deployment + metadata |
| `POST` | `/control/v1/endpoints/{externalEndpointId}/workers/{workerId}/terminate` | Drain/terminate worker (returns lifecycle command) |
| `GET` | `/control/v1/lifecycle-commands/{commandId}` | Lifecycle command status |

`externalEndpointId` is the stable Cloud id (`computeBaseId`). Waverless maps it to an internal endpoint name and stores it in `endpoints.external_endpoint_id`.

### Create (M1 auto)

`POST /control/v1/endpoints` accepts `gpuType`, `placementMode: "auto"`, optional `regionAllowlist`, and **does not** require `providerSkuId`. Waverless runs `pkg/placement` + Provider Hub on deploy and returns `activeSlice` (provider, region, SKU, `sliceId`, `unitCostSnapshot`) in the response when a slice exists.

Pin mode may pass `logicalAzId`, `providerId`, `physicalRegionId`, and `providerSkuId`.

## Verify (manual)

```bash
export WAVERLESS_BASE_URL=https://waverless.example
export KEY="$WAVERLESS_SERVICE_API_KEY"

# Create (auto)
curl -sS -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"externalEndpointId":"cb-test-1","name":"demo","image":"registry.example/app:latest","gpuType":"A100","gpuCount":1,"minReplicas":0,"maxReplicas":2,"scaleUpWaitSeconds":60,"placementMode":"auto","actor":{"type":"tenant_user","id":"u1","tenantId":"t1"}}' \
  "$WAVERLESS_BASE_URL/control/v1/endpoints" | jq .

# Runtime
curl -sS -H "Authorization: Bearer $KEY" \
  "$WAVERLESS_BASE_URL/control/v1/endpoints/cb-test-1/runtime" | jq .

# Scale bounds
curl -sS -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"minReplicas":1,"maxReplicas":3}' \
  "$WAVERLESS_BASE_URL/control/v1/endpoints/cb-test-1/scale"

# Workers
curl -sS -H "Authorization: Bearer $KEY" \
  "$WAVERLESS_BASE_URL/control/v1/endpoints/cb-test-1/workers?page=1&pageSize=20" | jq .
```

## Related surfaces

- `/api/v1/*` — Admin UI (name-keyed)
- `/portal/v1/*` — Portal southbound (name-keyed sync)
- `/control/v1/*` — Cloud control (external id-keyed)
