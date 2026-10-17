# Waverless Developer Guide

## Table of Contents

- [1. Code Structure](#1-code-structure)
- [2. Core Design Patterns](#2-core-design-patterns)
- [3. Task Assignment](#3-task-assignment)
- [4. Graceful Shutdown](#4-graceful-shutdown)
- [5. Autoscaler Design](#5-autoscaler-design)
- [6. Provider Integration](#6-provider-integration)
- [7. Background Jobs](#7-background-jobs)
- [8. Testing](#8-testing)

---

## 1. Code Structure

```
waverless/
鈹溾攢鈹€ app/                      # Application layer
鈹?  鈹溾攢鈹€ handler/             # HTTP handlers (thin)
鈹?  鈹溾攢鈹€ middleware/          # Auth, logging, recovery
鈹?  鈹斺攢鈹€ router/              # Route definitions
鈹?
鈹溾攢鈹€ cmd/                      # Entry points
鈹?  鈹溾攢鈹€ main.go             # Application entry
鈹?  鈹溾攢鈹€ initializers.go     # Dependency injection
鈹?  鈹斺攢鈹€ jobs.go             # Background jobs
鈹?
鈹溾攢鈹€ internal/                 # Internal packages
鈹?  鈹溾攢鈹€ service/            # Business logic
鈹?  鈹?  鈹溾攢鈹€ endpoint/       # Endpoint CRUD + deployment
鈹?  鈹?  鈹溾攢鈹€ lifecycle/      # Worker lifecycle
鈹?  鈹?  鈹溾攢鈹€ task_service.go
鈹?  鈹?  鈹斺攢鈹€ worker_service.go
鈹?  鈹溾攢鈹€ model/              # Domain models
鈹?  鈹斺攢鈹€ vo/                 # Value objects (DTOs)
鈹?
鈹溾攢鈹€ pkg/                      # Shared packages
鈹?  鈹溾攢鈹€ autoscaler/         # Autoscaling engine
鈹?  鈹?  鈹溾攢鈹€ manager.go      # Control loop
鈹?  鈹?  鈹溾攢鈹€ decision_engine.go
鈹?  鈹?  鈹溾攢鈹€ executor.go
鈹?  鈹?  鈹斺攢鈹€ metrics_collector.go
鈹?  鈹溾攢鈹€ provider/           # Deployment providers
鈹?  鈹?  鈹溾攢鈹€ k8s/           # Kubernetes
鈹?  鈹?  鈹溾攢鈹€ novita/        # Novita Serverless
鈹?  鈹?  鈹斺攢鈹€ docker/        # Docker (dev)
鈹?  鈹溾攢鈹€ interfaces/         # Shared interfaces
鈹?  鈹斺攢鈹€ store/              # Data access
鈹?      鈹溾攢鈹€ postgres/         # SQL repositories (PostgreSQL/PostgreSQL dialects)
鈹?      鈹斺攢鈹€ redis/         # Redis operations
鈹?
鈹斺攢鈹€ config/                   # Config files
```

### Layer Responsibilities

| Layer | Responsibility |
|-------|----------------|
| Handler | HTTP request/response, validation |
| Service | Business logic, orchestration |
| Provider | Infrastructure abstraction |
| Store | Data persistence |

---

## 2. Core Design Patterns

### Dependency Injection

All dependencies wired in `cmd/initializers.go`:

```mermaid
flowchart TB
    Config --> Stores
    Stores --> Providers
    Providers --> Services
    Services --> Handlers
    Handlers --> Router
```

### Service Layer Pattern

```mermaid
flowchart LR
    Handler --> Service
    Service --> Store[(PostgreSQL/Redis)]
    Service --> Provider
    Service --> OtherService
```

### Transaction Pattern

Database operations that need atomicity are wrapped in transactions:
- Multiple operations execute as a single unit
- Automatic rollback on any failure
- Commit only when all operations succeed

### Async Non-Critical Updates

Non-critical operations (like statistics updates) run asynchronously to avoid blocking the main request flow.

---

## 3. Task Assignment

### Double-Check Pattern

Prevents race conditions when workers are draining:

```mermaid
sequenceDiagram
    Worker->>Service: PullJobs(worker_id)
    Service->>PostgreSQL: Check worker status
    
    alt DRAINING
        Service-->>Worker: Empty
    else ONLINE
        Service->>Redis: RPOP task
        Service->>PostgreSQL: Update IN_PROGRESS
        Service->>PostgreSQL: Re-check status
        
        alt Became DRAINING
            Service->>PostgreSQL: Revert PENDING
            Service->>Redis: LPUSH back
            Service-->>Worker: Empty
        else Still ONLINE
            Service-->>Worker: Task
        end
    end
```

### Atomic Task Selection

Task selection uses database-level locking to prevent race conditions:
- Select pending tasks with row-level lock
- Update status atomically
- Ensures no duplicate task assignment

---

## 4. Graceful Shutdown

### Flow

```mermaid
sequenceDiagram
    K8s->>Informer: SIGTERM
    Informer->>Lifecycle: OnWorkerDraining
    Lifecycle->>PostgreSQL: Set DRAINING
    
    loop Until done
        Worker->>Worker: Complete tasks
    end

    K8s->>Informer: Pod Deleted
    Informer->>Lifecycle: OnWorkerDelete
    Lifecycle->>PostgreSQL: Set OFFLINE
```

### Rolling Update Optimization

When deployment changes:

```mermaid
flowchart LR
    A[Deployment Change] --> B[OnDeploymentChange]
    B --> C{Worker Status}
    C -->|Idle| D[Cost = -1000<br/>Delete First]
    C -->|Busy| E[Cost = 1000<br/>Delete Last]
```

### Key Points

1. Informer detects pod termination signal
2. Worker marked DRAINING immediately
3. No new tasks assigned to DRAINING workers
4. Worker completes current tasks
5. Pod deleted after tasks complete
6. Worker marked OFFLINE

---

## 5. Autoscaler Design

### Control Loop

```mermaid
flowchart TB
    subgraph "Every 30s"
        A[Acquire Lock] --> B[Collect Metrics]
        B --> C[Calculate Resources]
        C --> D[Make Decisions]
        D --> E[Execute]
        E --> F[Release Lock]
    end
```

### Decision Engine

```mermaid
flowchart TB
    subgraph "Per Endpoint"
        M1[Pending Tasks]
        M2[Replicas]
        M3[Idle Time]
        M4[Priority]
    end

    M1 & M2 & M3 & M4 --> D{Decision}

    D -->|pending >= threshold| Up[Scale Up]
    D -->|idle >= timeout| Down[Scale Down]
    D -->|otherwise| None[No Action]
```

### Multi-Instance Safety

Uses Redis distributed lock to ensure only one instance makes scaling decisions at a time, preventing conflicting operations in multi-replica deployments.

### Priority & Preemption

1. Sort scale-up requests by priority
2. Minimum guarantee: 1 replica per endpoint with tasks
3. Remaining resources by priority
4. High priority can preempt low priority idle workers

### Starvation Protection

If endpoint waits > 5min without resources:
- Temporarily elevate priority
- Ensure eventual resource allocation

---

## 6. Provider Integration

### DeploymentProvider Interface

The provider interface defines standard operations:
- **Deploy**: Create new worker deployment
- **GetApp/ListApps**: Query deployment status
- **DeleteApp**: Remove deployment
- **ScaleApp**: Adjust replica count
- **UpdateDeployment**: Update deployment configuration
- **ListSpecs/GetSpec**: Query available resource specs
- **WatchReplicas**: Monitor replica changes
- **IsPodTerminating**: Check termination status

### Implementing New Provider

1. **Create package**: `pkg/provider/myprovider/`

2. **Implement the DeploymentProvider interface** with all required methods

3. **Implement lifecycle callbacks** (recommended):
   - OnWorkerStatusChange: Handle status transitions
   - OnWorkerDelete: Cleanup on worker removal
   - OnWorkerDraining: Stop task assignment
   - OnWorkerFailure: Record failure information
   - OnEndpointStatusChange: Track deployment health

4. **Register in provider factory**: Add initialization logic

5. **Register in lifecycle manager**: Connect lifecycle callbacks

### Provider Comparison

| Provider | Use Case | Lifecycle |
|----------|----------|-----------|
| K8s | Production | Full (Informers) |
| Novita | Serverless GPU | Full (Polling) |
| Docker | Development | Basic |

---

## 7. Background Jobs

### Job Manager (`cmd/jobs.go`)

| Job | Interval | Purpose |
|-----|----------|---------|
| CleanupOfflineWorkers | 30s | Mark stale workers offline |
| CleanupOrphanedTasks | 60s | Re-queue tasks from dead workers |
| CleanupTimedOutTasks | 60s | Fail tasks exceeding timeout |

### Orphaned Task Recovery

```mermaid
flowchart TB
    A[Find IN_PROGRESS tasks] --> B{Worker exists?}
    B -->|No| C[Re-queue]
    B -->|Yes| D{Worker OFFLINE?}
    D -->|Yes| C
    D -->|No| E[Keep]
```

---

## 8. Testing

### Unit Tests

```bash
go test ./...
go test -cover ./...
go test ./pkg/autoscaler/...
```

### Integration Tests

```bash
go test -tags=integration ./...
```

### Property-Based Tests

Located in `*_property_test.go` files, these tests verify system behavior across randomly generated inputs to catch edge cases.

### Key Log Points

- Task submission events
- Job pull operations
- Worker draining notifications
- Scaling decisions

---

## Contributing

1. Fork repository
2. Create feature branch
3. Write tests
4. Submit pull request

### Code Style

- Follow Go conventions
- Use `gofmt` and `golint`
- Add comments for exported functions
- Meaningful commit messages

---

**Document Version**: v3.0  
**Last Updated**: 2026-02
