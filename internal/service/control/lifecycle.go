package control

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// CommandState matches the Cloud wire contract.
type CommandState string

const (
	CommandAccepted    CommandState = "accepted"
	CommandDraining    CommandState = "draining"
	CommandTerminating CommandState = "terminating"
	CommandSucceeded   CommandState = "succeeded"
	CommandFailed      CommandState = "failed"
	CommandTimedOut    CommandState = "timed_out"
)

// LifecycleCommand is the persisted lifecycle command record.
type LifecycleCommand struct {
	CommandID          string
	State              CommandState
	WorkerID           string
	ExternalEndpointID string
	UpdatedAt          time.Time
}

// LifecycleStore is an in-process command status store (P0).
type LifecycleStore struct {
	mu   sync.RWMutex
	byID map[string]*LifecycleCommand
}

func NewLifecycleStore() *LifecycleStore {
	return &LifecycleStore{byID: make(map[string]*LifecycleCommand)}
}

func (s *LifecycleStore) Create(workerID, externalEndpointID string) *LifecycleCommand {
	cmd := &LifecycleCommand{
		CommandID:          uuid.NewString(),
		State:              CommandAccepted,
		WorkerID:           workerID,
		ExternalEndpointID: externalEndpointID,
		UpdatedAt:          time.Now().UTC(),
	}
	s.mu.Lock()
	s.byID[cmd.CommandID] = cmd
	s.mu.Unlock()
	return cmd
}

func (s *LifecycleStore) Get(commandID string) *LifecycleCommand {
	s.mu.RLock()
	cmd := s.byID[commandID]
	s.mu.RUnlock()
	if cmd == nil {
		return nil
	}
	copy := *cmd
	return &copy
}

func (s *LifecycleStore) UpdateState(commandID string, state CommandState) {
	s.mu.Lock()
	if cmd, ok := s.byID[commandID]; ok {
		cmd.State = state
		cmd.UpdatedAt = time.Now().UTC()
	}
	s.mu.Unlock()
}
