package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// RunEntryResponse represents a run entry for API responses and event publishing
type RunEntryResponse struct {
	ID               int64                  `json:"id"`
	SessionID        *string                `json:"session_id,omitempty"`
	ServiceName      string                 `json:"service_name"`
	ScopeName        *string                `json:"scope_name,omitempty"`
	Name             string                 `json:"name"`
	StartTimestampNs int64                  `json:"start_timestamp_ns"`
	EndTimestampNs   *int64                 `json:"end_timestamp_ns,omitempty"`
	TraceID          *string                `json:"trace_id,omitempty"`
	ProcessorID      *int64                 `json:"processor_id,omitempty"`
	RunType          *int16                 `json:"run_type,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

// ToRunEntryResponse converts a database Run to RunEntryResponse
func ToRunEntryResponse(r *dbconfig.Run) RunEntryResponse {
	return RunEntryResponse{
		ID:               r.ID,
		SessionID:        r.SessionID,
		ServiceName:      r.ServiceName,
		ScopeName:        r.ScopeName,
		Name:             r.Name,
		StartTimestampNs: r.StartTimestampNs,
		EndTimestampNs:   r.EndTimestampNs,
		TraceID:          r.TraceID,
		ProcessorID:      r.ProcessorID,
		RunType:          r.RunType,
		Metadata:         r.Metadata,
		CreatedAt:        r.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:        r.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

// RunService provides a public interface for run management operations
type RunService struct {
	runRepo  dbconfig.RunRepositoryInterface
	eventBus event.EventBusInterface
}

// NewRunService creates a new run service instance
func NewRunService(ctx context.Context, db *sql.DB, schema string) (*RunService, error) {
	runRepo, err := dbconfig.NewRunRepository(ctx, db, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to create run repository: %w", err)
	}
	return &RunService{
		runRepo: runRepo,
	}, nil
}

// NewRunServiceWithEventBus creates a new run service instance with event bus support
func NewRunServiceWithEventBus(ctx context.Context, db *sql.DB, schema string, eventBus event.EventBusInterface) (*RunService, error) {
	runRepo, err := dbconfig.NewRunRepository(ctx, db, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to create run repository: %w", err)
	}
	return &RunService{
		runRepo:  runRepo,
		eventBus: eventBus,
	}, nil
}

// closeOverlappingRun closes any existing run with the same name that has a null end timestamp
func (s *RunService) closeOverlappingRun(ctx context.Context, name string, newStartTimestampNs int64) error {
	existingRunID, err := s.runRepo.GetActiveIdByName(ctx, name)
	if err != nil {
		log.Printf("Failed to check for existing run: %v", err)
		return fmt.Errorf("failed to check for existing run: %w", err)
	}

	if existingRunID == 0 {
		return nil // No overlapping run found
	}

	log.Printf("Found existing active run: ID=%d, closing it before creating new run", existingRunID)

	// Close the existing run with end timestamp = new start timestamp - 1 nanosecond
	endTimestampNs := newStartTimestampNs - 1
	err = s.runRepo.EndRun(ctx, existingRunID, endTimestampNs)
	if err != nil {
		log.Printf("Failed to close existing run: %v", err)
		return fmt.Errorf("failed to close existing run: %w", err)
	}

	log.Printf("Closed existing run: ID=%d, endTimestampNs=%d", existingRunID, endTimestampNs)
	return nil
}

// CreateRun creates a new run with the specified parameters
func (s *RunService) CreateRun(ctx context.Context, serviceName, name string, startTimestampNs int64, runType *int16) (int64, error) {
	log.Printf("Creating run: service=%s, name=%s, startTimestampNs=%d, runType=%v", serviceName, name, startTimestampNs, runType)

	// Close any overlapping run before creating the new one
	err := s.closeOverlappingRun(ctx, name, startTimestampNs)
	if err != nil {
		return 0, err
	}

	// Create the new run
	metadata := map[string]interface{}{}
	runID, err := s.runRepo.Create(ctx, serviceName, nil, name, startTimestampNs, nil, nil, nil, runType, metadata)
	if err != nil {
		log.Printf("Failed to create run: %v", err)
		return 0, fmt.Errorf("failed to create run: %w", err)
	}

	log.Printf("Run created successfully: ID=%d", runID)

	// After successful creation, publish a run.created event
	if s.eventBus != nil {
		// Get the created run to include in the event
		run, err := s.runRepo.GetByID(ctx, runID)
		if err != nil {
			log.Printf("Failed to get created run for event publishing: %v", err)
		} else {
			runResponse := ToRunEntryResponse(run)
			s.eventBus.Publish(ctx, event.Event{
				Type:      EventTypeRunCreated.ToEventType(),
				Timestamp: time.Now(),
				Schema:    "otel", // Use default schema for run events
				Data:      runResponse,
				IDs:       []string{fmt.Sprintf("%d", runID)},
			})
		}
	}

	return runID, nil
}

// CreateRunWithSessionID creates a new run with session ID
func (s *RunService) CreateRunWithSessionID(ctx context.Context, sessionID *string, serviceName, name string, startTimestampNs int64, runType *int16) (int64, error) {
	log.Printf("Creating run with session ID: sessionID=%v, service=%s, name=%s, startTimestampNs=%d, runType=%v", sessionID, serviceName, name, startTimestampNs, runType)

	// Close any overlapping run before creating the new one
	err := s.closeOverlappingRun(ctx, name, startTimestampNs)
	if err != nil {
		return 0, err
	}

	// Create the new run with session ID
	metadata := map[string]interface{}{}
	runID, err := s.runRepo.CreateWithSessionID(ctx, sessionID, serviceName, nil, name, startTimestampNs, nil, nil, nil, runType, metadata)
	if err != nil {
		log.Printf("Failed to create run: %v", err)
		return 0, fmt.Errorf("failed to create run: %w", err)
	}

	log.Printf("Run created successfully: ID=%d", runID)

	// After successful creation, publish a run.created event
	if s.eventBus != nil {
		// Get the created run to include in the event
		run, err := s.runRepo.GetByID(ctx, runID)
		if err != nil {
			log.Printf("Failed to get created run for event publishing: %v", err)
		} else {
			runResponse := ToRunEntryResponse(run)
			s.eventBus.Publish(ctx, event.Event{
				Type:      EventTypeRunCreated.ToEventType(),
				Timestamp: time.Now(),
				Schema:    "otel", // Use default schema for run events
				Data:      runResponse,
				IDs:       []string{fmt.Sprintf("%d", runID)},
			})
		}
	}

	return runID, nil
}

// EndRun ends a run by setting the end timestamp
func (s *RunService) EndRun(ctx context.Context, name string, endTimestampNs int64) error {
	log.Printf("Ending run: name=%s, endTimestampNs=%d", name, endTimestampNs)

	// Find the most recent run with this name
	runID, err := s.runRepo.GetActiveIdByName(ctx, name)
	if err != nil {
		log.Printf("Failed to get active run ID: %v", err)
		return fmt.Errorf("failed to get active run ID: %w", err)
	}

	if runID == 0 {
		log.Printf("No active run found with name: %s", name)
		return fmt.Errorf("no active run found with name: %s", name)
	}

	log.Printf("Found active run: ID=%d", runID)

	// End the run
	err = s.runRepo.EndRun(ctx, runID, endTimestampNs)
	if err != nil {
		log.Printf("Failed to end run: %v", err)
		return fmt.Errorf("failed to end run: %w", err)
	}

	log.Printf("Run ended successfully: ID=%d", runID)

	// After successful end, publish a run.updated event
	if s.eventBus != nil {
		// Get the updated run to include in the event
		run, err := s.runRepo.GetByID(ctx, runID)
		if err != nil {
			log.Printf("Failed to get ended run for event publishing: %v", err)
		} else {
			runResponse := ToRunEntryResponse(run)
			s.eventBus.Publish(ctx, event.Event{
				Type:      EventTypeRunUpdated.ToEventType(),
				Timestamp: time.Now(),
				Schema:    "otel", // Use default schema for run events
				Data:      runResponse,
				IDs:       []string{fmt.Sprintf("%d", runID)},
			})
		}
	}

	return nil
}

// GetRunByID returns a run by its ID
func (s *RunService) GetRunByID(ctx context.Context, id int64) (*dbconfig.Run, error) {
	return s.runRepo.GetByID(ctx, id)
}

// GetRunCount returns the count of runs with the specified name
func (s *RunService) GetRunCount(ctx context.Context, name string) (int64, error) {
	return s.runRepo.GetCount(ctx, name)
}

// GetActiveRunsByServiceName returns all active runs for a specific service
func (s *RunService) GetActiveRunsByServiceName(ctx context.Context, serviceName string) ([]*dbconfig.Run, error) {
	return s.runRepo.GetActiveRunsByServiceName(ctx, serviceName)
}

// UpdateRunMetadata updates the metadata for a specific run
func (s *RunService) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}) error {
	// Get the current run to preserve other fields
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run by ID: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found with ID %d", runID)
	}

	// Update only the metadata field
	err = s.runRepo.Update(ctx, runID, run.ServiceName, run.ScopeName, run.Name,
		run.StartTimestampNs, run.EndTimestampNs, run.TraceID, run.ProcessorID, run.RunType, metadata)
	if err != nil {
		return err
	}

	// After successful update, publish a run.updated event
	if s.eventBus != nil {
		// Get the updated run to include in the event
		updatedRun, err := s.runRepo.GetByID(ctx, runID)
		if err != nil {
			log.Printf("Failed to get updated run for event publishing: %v", err)
		} else {
			runResponse := ToRunEntryResponse(updatedRun)
			s.eventBus.Publish(ctx, event.Event{
				Type:      EventTypeRunUpdated.ToEventType(),
				Timestamp: time.Now(),
				Schema:    "otel", // Use default schema for run events
				Data:      runResponse,
				IDs:       []string{fmt.Sprintf("%d", runID)},
			})
		}
	}

	return nil
}

// UpdateRunMetadataBatch updates metadata for multiple runs and publishes a batch event
func (s *RunService) UpdateRunMetadataBatch(ctx context.Context, updates map[int64]map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	var updatedRuns []RunEntryResponse
	var runIDs []string

	// Process each update
	for runID, metadata := range updates {
		// Get the current run to preserve other fields
		run, err := s.runRepo.GetByID(ctx, runID)
		if err != nil {
			log.Printf("Failed to get run by ID %d: %v", runID, err)
			continue
		}
		if run == nil {
			log.Printf("Run not found with ID %d", runID)
			continue
		}

		// Update only the metadata field
		err = s.runRepo.Update(ctx, runID, run.ServiceName, run.ScopeName, run.Name,
			run.StartTimestampNs, run.EndTimestampNs, run.TraceID, run.ProcessorID, run.RunType, metadata)
		if err != nil {
			log.Printf("Failed to update run %d: %v", runID, err)
			continue
		}

		// Get the updated run for the event
		updatedRun, err := s.runRepo.GetByID(ctx, runID)
		if err != nil {
			log.Printf("Failed to get updated run %d for event publishing: %v", runID, err)
			continue
		}

		runResponse := ToRunEntryResponse(updatedRun)
		updatedRuns = append(updatedRuns, runResponse)
		runIDs = append(runIDs, fmt.Sprintf("%d", runID))
	}

	// Publish batch event if we have updates and event bus is available
	if len(updatedRuns) > 0 && s.eventBus != nil {
		s.eventBus.Publish(ctx, event.Event{
			Type:      EventTypeRunUpdated.ToEventType(),
			Timestamp: time.Now(),
			Schema:    "",          // Runs don't have schema-specific routing like logs
			Data:      updatedRuns, // Use concrete slice, not map
			IDs:       runIDs,
		})
	}

	return nil
}

// GetRunsBySessionID returns all runs for a specific session ID
func (s *RunService) GetRunsBySessionID(ctx context.Context, sessionID string) ([]*dbconfig.Run, error) {
	return s.runRepo.GetBySessionID(ctx, sessionID)
}
