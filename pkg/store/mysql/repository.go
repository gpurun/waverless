package mysql

import (
	"fmt"

	"waverless/pkg/config"
)

// Repository aggregates all SQL repositories (MySQL or PostgreSQL)
type Repository struct {
	ds *Datastore

	Endpoint         *EndpointRepository
	Task             *TaskRepository
	TaskEvent        *TaskEventRepository
	TaskStatistics   *TaskStatisticsRepository
	ScalingEvent     *ScalingEventRepository
	AutoscalerConfig *AutoscalerConfigRepository
	Spec             *SpecRepository
	SpecCapacity     *SpecCapacityRepository
	Worker           *WorkerRepository
	Monitoring       *MonitoringRepository
	StatusEvent      *StatusEventRepository
}

// NewRepository creates a new MySQL repository with all sub-repositories.
func NewRepository(dsn string, proxyConfig *config.ProxyConfig) (*Repository, error) {
	ds, err := NewDatastore(dsn, proxyConfig)
	if err != nil {
		return nil, err
	}
	return newRepository(ds), nil
}

// NewPostgresRepository creates a new PostgreSQL repository with all sub-repositories.
func NewPostgresRepository(dsn string, proxyConfig *config.ProxyConfig) (*Repository, error) {
	ds, err := NewPostgresDatastore(dsn, proxyConfig)
	if err != nil {
		return nil, err
	}
	return newRepository(ds), nil
}

// NewRepositoryForDriver creates a repository for the given driver (postgres only at runtime).
func NewRepositoryForDriver(driver, dsn string, proxyConfig *config.ProxyConfig) (*Repository, error) {
	switch driver {
	case "", DialectPostgres, "postgresql":
		return NewPostgresRepository(dsn, proxyConfig)
	case DialectMySQL, "mariadb":
		return nil, fmt.Errorf("metadata driver %q is no longer supported; use postgres", driver)
	default:
		return nil, fmt.Errorf("unsupported metadata driver: %s (only postgres is supported)", driver)
	}
}

func newRepository(ds *Datastore) *Repository {
	return &Repository{
		ds:               ds,
		Endpoint:         NewEndpointRepository(ds),
		Task:             NewTaskRepository(ds),
		TaskEvent:        NewTaskEventRepository(ds),
		TaskStatistics:   NewTaskStatisticsRepository(ds),
		ScalingEvent:     NewScalingEventRepository(ds),
		AutoscalerConfig: NewAutoscalerConfigRepository(ds),
		Spec:             NewSpecRepository(ds),
		SpecCapacity:     NewSpecCapacityRepository(ds),
		Worker:           NewWorkerRepository(ds),
		Monitoring:       NewMonitoringRepository(ds),
		StatusEvent:      NewStatusEventRepository(ds),
	}
}

// GetDatastore returns the underlying datastore for transaction support
func (r *Repository) GetDatastore() *Datastore {
	return r.ds
}

// Close closes the database connection
func (r *Repository) Close() error {
	return r.ds.Close()
}
