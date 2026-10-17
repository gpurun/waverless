# Waverless Database Deployment

This document describes PostgreSQL metadata storage for Waverless. Redis remains
the queue, heartbeat, cache, and distributed-lock store.

## Overview

Waverless uses PostgreSQL 17 for persistent metadata, initialized from the canonical
`scripts/init_postgres.sql` schema. Production deployments should use a persistent
volume and manage credentials through Kubernetes Secrets.

## Architecture

```
鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
鈹? Waverless App  鈹?
鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
         鈹?
    鈹屸攢鈹€鈹€鈹€鈹粹攢鈹€鈹€鈹€鈹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
    鈹?        鈹?             鈹?
鈹屸攢鈹€鈹€鈻尖攢鈹€鈹€鈹€鈹?鈹屸攢鈹€鈻尖攢鈹€鈹€鈹€鈹€鈹€鈹?鈹屸攢鈹€鈹€鈹€鈻尖攢鈹€鈹€鈹€鈹?
鈹?Redis  鈹?鈹侾ostgreSQL鈹?鈹?  K8s   鈹?鈹?Cache  鈹?鈹侱atabase 鈹?鈹?Cluster 鈹?
鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
```

## Default Configuration

**Service**: `waverless-postgres-svc` (ClusterIP)
**Port**: 5432
**Database**: `waverless`

**Credentials**:
- User: `wavespeed`
- Password: configure with a Kubernetes Secret for non-development deployments

## Database Schema

The PostgreSQL deployment automatically initializes all 15 metadata tables:

### Core Tables

1. **tasks** - Task lifecycle and execution tracking
2. **endpoints** - Endpoint configuration and metadata
3. **autoscaler_config** - Autoscaler settings per endpoint

### Event Tracking

4. **task_events** - Detailed task event audit log
5. **scaling_events** - Autoscaling event history

### Statistics & Monitoring

6. **task_statistics** - Pre-aggregated task statistics (global/per-endpoint)
7. **worker_events** - Worker lifecycle and idle tracking
8. **worker_resource_snapshots** - GPU/CPU utilization snapshots
9. **endpoint_minute_stats** - Minute-level endpoint aggregation
10. **endpoint_hourly_stats** - Hourly endpoint aggregation
11. **endpoint_daily_stats** - Daily endpoint aggregation
12. **status_events** - Worker status and pending-phase history

## Deployment

### Automatic Deployment

The PostgreSQL database is automatically deployed when using the deployment script:

```bash
./deploy.sh install
```

Deployment sequence:
1. Namespace creation
2. RBAC setup
3. ConfigMap creation
4. **Redis deployment** 鈫?Cache layer
5. **PostgreSQL deployment** 鈫?Metadata database
6. Waverless API server
7. Web UI

### Manual Deployment

If deploying manually:

```bash
# Create schema ConfigMap and apply PostgreSQL deployment
kubectl create configmap waverless-postgres-init \
  --from-file=init_postgres.sql=scripts/init_postgres.sql -n wavespeed
kubectl apply -f k8s/postgres-deployment.yaml

# Wait for PostgreSQL to be ready
kubectl wait --for=condition=ready pod -l app=waverless-postgres -n wavespeed --timeout=180s

# Verify PostgreSQL is running
kubectl get pods -n wavespeed -l app=waverless-postgres
```

## Database Initialization

The database is initialized on first startup from `scripts/init_postgres.sql`:

- Creates all required tables
- Sets up indexes for optimal performance
- Initializes global statistics record

## Resource Configuration

**Requests**:
- Memory: 512Mi
- CPU: 200m

**Limits**:
- Memory: 1Gi
- CPU: 500m

## Storage

The PostgreSQL StatefulSet provisions a 20Gi PersistentVolumeClaim by default.

### Upgrading to Persistent Storage

1. Create a PersistentVolumeClaim:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: wavespeed
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
  storageClassName: your-storage-class
```

2. Update `k8s/postgres-deployment.yaml`:

```yaml
volumes:
- name: postgres-data
  persistentVolumeClaim:
    claimName: postgres-pvc
```

## Health Checks

**Liveness Probe**: `pg_isready -U wavespeed -d waverless` every 10 seconds
**Readiness Probe**: `pg_isready -U wavespeed -d waverless` every 5 seconds

## Accessing the Database

### From within the cluster

```bash
psql -h waverless-postgres-svc -U wavespeed -d waverless
```

### Using kubectl port-forward

```bash
# Port forward to localhost
kubectl port-forward -n wavespeed svc/waverless-postgres-svc 5432:5432

# Connect from local machine
psql -h 127.0.0.1 -U wavespeed -d waverless
```

### From a debug pod

```bash
kubectl run -it --rm debug --image=postgres:17 --restart=Never -n wavespeed -- \
  psql -h waverless-postgres-svc -U wavespeed -d waverless
```

## Backup & Restore

### Backup

```bash
# Export database to file
kubectl exec -n wavespeed statefulset/waverless-postgres -- \
  pg_dump -U wavespeed -d waverless > backup.sql
```

### Restore

```bash
# Import from backup file
kubectl exec -i -n wavespeed statefulset/waverless-postgres -- \
  psql -U wavespeed -d waverless < backup.sql
```

## Monitoring

### Check PostgreSQL status

```bash
# View logs
kubectl logs -n wavespeed -l app=waverless-postgres -f

# Check resource usage
kubectl top pod -n wavespeed -l app=waverless-postgres

# View pod details
kubectl describe pod -n wavespeed -l app=waverless-postgres
```

### Query statistics

```sql
-- Check table sizes
SELECT relname AS table_name,
       pg_size_pretty(pg_total_relation_size(quote_ident(relname))) AS total_size
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(quote_ident(relname)) DESC;

-- View recent tasks
SELECT task_id, endpoint, status, created_at, completed_at
FROM tasks
ORDER BY created_at DESC
LIMIT 10;

-- Check global statistics
SELECT * FROM task_statistics WHERE scope_type = 'global';

-- View task totals by endpoint
SELECT endpoint, status, COUNT(*) AS task_count
FROM tasks
GROUP BY endpoint, status
ORDER BY endpoint, status;
```

## Troubleshooting

### PostgreSQL pod not starting

```bash
# Check pod status
kubectl get pods -n wavespeed -l app=waverless-postgres

# View events
kubectl describe pod -n wavespeed -l app=waverless-postgres

# Check logs
kubectl logs -n wavespeed -l app=waverless-postgres
```

### Connection issues

```bash
# Test connectivity from Waverless pod
kubectl exec -it -n wavespeed deployment/waverless -- \
  nc -zv waverless-postgres-svc 5432

# Verify service
kubectl get svc -n wavespeed waverless-postgres-svc
```

### Reset database

```bash
# Delete the PostgreSQL pod (StatefulSet recreates it using its persistent volume)
kubectl delete pod -n wavespeed -l app=waverless-postgres

# Or delete entire deployment
kubectl delete -f k8s/postgres-deployment.yaml
kubectl apply -f k8s/postgres-deployment.yaml
```

## Security Considerations

鈿狅笍 **Important**: The default credentials are for development/testing only!

### For Production:

1. **Use Kubernetes Secrets** for database credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
  namespace: wavespeed
type: Opaque
stringData:
  user: wavespeed
  password: <strong-random-password>
```

2. **Update deployment** to use secrets:

```yaml
env:
- name: POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: postgres-secret
      key: password
```

3. **Enable TLS/SSL** for encrypted connections
4. **Use network policies** to restrict database access
5. **Enable audit logging** for compliance
6. **Regular backups** to persistent storage

## Using a Managed PostgreSQL Service

Point `postgres.host`, `postgres.port`, and the associated secret-backed
credentials at the managed service, set `providers.metadata: postgres`, and do
not apply `k8s/postgres-deployment.yaml`. Apply `scripts/init_postgres.sql` to
the database before starting Waverless.

---

**Last Updated**: 2026-10-17
