package repository

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"time"

	"database/sql"

	"github.com/medxops/trazr-otel/pkg/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// API response models
type ProfileEntryResponse struct {
	ProfileBytes                   string      `json:"profile_bytes"`
	ProfileStorageVersion          int16       `json:"profile_storage_version"`
	ProfileEncoding                string      `json:"profile_encoding"`
	ProfileSchemaVersion           *string     `json:"profile_schema_version,omitempty"`
	OtelProfileID                  *string     `json:"otel_profile_id,omitempty"`
	ProfileTimeUnixNano            *string     `json:"profile_time_unix_nano,omitempty"`
	DurationNano                   *string     `json:"duration_nano,omitempty"`
	DurationNS                     int64       `json:"duration_ns"`
	SampleType                     *string     `json:"sample_type,omitempty"`
	SampleUnit                     *string     `json:"sample_unit,omitempty"`
	ProfileDecodeStatus            string      `json:"profile_decode_status"`
	ProfileDecodeError             string      `json:"profile_decode_error,omitempty"`
	ServiceNamespace               *string     `json:"service_namespace,omitempty"`
	ServiceInstanceID              *string     `json:"service_instance_id,omitempty"`
	ProfileDroppedAttributesCount  *uint32     `json:"profile_dropped_attributes_count,omitempty"`
	ResourceDroppedAttributesCount *uint32     `json:"resource_dropped_attributes_count,omitempty"`
	ScopeDroppedAttributesCount    *uint32     `json:"scope_dropped_attributes_count,omitempty"`
	ProfileID                      int64       `json:"profile_id"`
	TimestampNS                    int64       `json:"timestamp_ns"`
	Timestamp                      string      `json:"timestamp"`
	ServiceName                    *string     `json:"service_name,omitempty"`
	ProfileType                    *string     `json:"profile_type,omitempty"`
	ProfileData                    interface{} `json:"profile_data,omitempty"`
	ResourceAttributes             interface{} `json:"resource_attributes,omitempty"`
	ResourceSchemaURL              *string     `json:"resource_schema_url,omitempty"`
	ScopeName                      *string     `json:"scope_name,omitempty"`
	ScopeVersion                   *string     `json:"scope_version,omitempty"`
	ScopeAttributes                interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount          *int32      `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL                 *string     `json:"scope_schema_url,omitempty"`
	Attributes                     interface{} `json:"attributes,omitempty"`
	DroppedAttrCount               *int32      `json:"dropped_attr_count,omitempty"`
	Metadata                       interface{} `json:"metadata,omitempty"`
}

type ProfileQueryResult struct {
	Entries    []ProfileEntryResponse `json:"entries"`
	NextCursor string                 `json:"next_cursor"`
	HasMore    bool                   `json:"has_more"`
	PrevCursor string                 `json:"prev_cursor,omitempty"`
	HasPrev    bool                   `json:"has_prev,omitempty"`
}

// Service interfaces
type ProfileQueryService interface {
	QueryProfiles(ctx context.Context, q ProfileQuery) (ProfileQueryResult, error)
	StreamProfiles(ctx context.Context, q ProfileQuery) (<-chan ProfileEntry, error)
}

type ProfileWriterService interface {
	InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error)
}

// Service implementations
type profileQueryService struct {
	repo ProfileRepository
}

func NewProfileQueryService(repo ProfileRepository) ProfileQueryService {
	return &profileQueryService{repo: repo}
}

func (s *profileQueryService) QueryProfiles(ctx context.Context, q ProfileQuery) (ProfileQueryResult, error) {
	entries, nextCursor, hasMore, err := s.repo.QueryProfiles(ctx, q)
	if err != nil {
		return ProfileQueryResult{}, err
	}
	respEntries := make([]ProfileEntryResponse, 0, len(entries))
	for _, e := range entries {
		respEntries = append(respEntries, toProfileEntryResponse(e))
	}
	return ProfileQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func (s *profileQueryService) StreamProfiles(ctx context.Context, q ProfileQuery) (<-chan ProfileEntry, error) {
	return nil, nil // Not implemented
}

type profileWriterService struct {
	repo         ProfileRepository
	eventBus     event.EventBusInterface
	schemaRouter DatabaseRoutingSchemaInterface
}

func NewProfileWriterService(repo ProfileRepository) ProfileWriterService {
	return &profileWriterService{repo: repo}
}

// NewProfileWriterServiceWithEventBus creates a ProfileWriterService with event bus support
func NewProfileWriterServiceWithEventBus(repo ProfileRepository, eventBus event.EventBusInterface, schemaRouter DatabaseRoutingSchemaInterface) ProfileWriterService {
	return &profileWriterService{repo: repo, eventBus: eventBus, schemaRouter: schemaRouter}
}

// extractServiceNamespaceFromProfile extracts the service.namespace from ResourceAttributes JSONB
func (s *profileWriterService) extractServiceNamespaceFromProfile(resourceAttributes []byte) string {
	if len(resourceAttributes) == 0 {
		return ""
	}

	var attrs map[string]interface{}
	if err := json.Unmarshal(resourceAttributes, &attrs); err != nil {
		return ""
	}

	if namespace, ok := attrs["service.namespace"]; ok {
		if ns, ok := namespace.(string); ok {
			return ns
		}
	}
	return ""
}

func (s *profileWriterService) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
	for i, e := range entries {
		if err := ValidateProfileEntry(e); err != nil {
			return nil, fmt.Errorf("profile %d: %w", i, err)
		}
	}
	// Apply schema routing if schema is empty and entries exist
	targetSchema := schema
	if schema == "" && len(entries) > 0 && s.schemaRouter != nil {
		// Use first entry for routing (assumes batch consistency)
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := s.extractServiceNamespaceFromProfile(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		} else {
			return nil, fmt.Errorf("no target schema found for service %s in namespace %s", serviceName, serviceNamespace)
		}
	}

	tracer := otel.Tracer("trazr-otel.repository.profiles")
	ctx, span := tracer.Start(ctx, "InsertProfiles", trace.WithAttributes(
		attribute.String("db.schema.target", targetSchema),
		attribute.Int("profiles.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.profiles"),
		zap.String("method", "InsertProfiles"),
		zap.String("schema.target", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertProfiles started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertProfiles(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertProfiles failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish profile.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]ProfileEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toProfileEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventProfileInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish profile.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertProfiles completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

// Mapping and helper functions
func toProfileEntryResponse(e ProfileEntry) ProfileEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	response := ProfileEntryResponse{
		ProfileBytes: base64.StdEncoding.EncodeToString(e.ProfileBytes), ProfileStorageVersion: e.ProfileStorageVersion, ProfileEncoding: e.ProfileEncoding, ProfileSchemaVersion: e.ProfileSchemaVersion,
		OtelProfileID: e.OtelProfileID, ProfileTimeUnixNano: e.ProfileTimeUnixNano, DurationNano: e.DurationNano, DurationNS: e.DurationNS, SampleType: e.SampleType, SampleUnit: e.SampleUnit,
		ServiceNamespace: e.ServiceNamespace, ServiceInstanceID: e.ServiceInstanceID,
		ProfileDecodeStatus: "legacy-opaque",
		ProfileID:           e.ID,
		TimestampNS:         e.TimestampNS,
		Timestamp:           e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ServiceName:         &e.ServiceName,
		ProfileType:         &e.ProfileType,

		ResourceAttributes:    unmarshalProfileJSON(e.ResourceAttributes),
		ResourceSchemaURL:     e.ResourceSchemaURL,
		ScopeName:             e.ScopeName,
		ScopeVersion:          e.ScopeVersion,
		ScopeAttributes:       unmarshalProfileJSON(e.ScopeAttributes),
		ScopeDroppedAttrCount: e.ScopeDroppedAttrCount,
		ScopeSchemaURL:        e.ScopeSchemaURL,
		Attributes:            unmarshalProfileJSON(e.Attributes),

		Metadata: metadata,
	}
	if response.ProfileEncoding == "" {
		response.ProfileEncoding = ProfileEncodingLegacy
	}
	if !isLegacyProfileEntry(e) {
		pd, err := DecodeStoredProfile(e)
		if err != nil {
			response.ProfileDecodeStatus = "error"
			response.ProfileDecodeError = err.Error()
			return response
		}
		raw, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(pd)
		if err != nil {
			response.ProfileDecodeStatus = "error"
			response.ProfileDecodeError = err.Error()
			return response
		}
		response.ProfileData = json.RawMessage(raw)
		response.ProfileDecodeStatus = "decoded"
		rp := pd.ResourceProfiles().At(0)
		sp := rp.ScopeProfiles().At(0)
		p := sp.Profiles().At(0)
		pc, rc, sc := p.DroppedAttributesCount(), rp.Resource().DroppedAttributesCount(), sp.Scope().DroppedAttributesCount()
		response.ProfileDroppedAttributesCount = &pc
		response.ResourceDroppedAttributesCount = &rc
		response.ScopeDroppedAttributesCount = &sc
		if pc <= 2147483647 {
			v := int32(pc)
			response.DroppedAttrCount = &v
		}
	} else if json.Valid(e.ProfileBytes) {
		// Some historical callers wrote JSON. Preserve that JSON without applying
		// current protobuf semantics; binary legacy rows remain base64 only.
		response.ProfileData = json.RawMessage(e.ProfileBytes)
		response.ProfileDecodeStatus = "legacy-json"
	}
	return response
}

func NewProfileRepository(db *sql.DB) ProfileRepository {
	return &profileRepository{db: db}
}

func NewProfileQueryServiceWithUnifiedClient(db OtelDatabaseClient) ProfileQueryService {
	return &profileQueryService{repo: db}
}

func NewProfileWriterServiceWithUnifiedClient(db OtelDatabaseClient) ProfileWriterService {
	return &profileWriterService{repo: db}
}

// NewProfileWriterServiceWithUnifiedClientAndEventBus creates a ProfileWriterService with unified client and event bus
func NewProfileWriterServiceWithUnifiedClientAndEventBus(db OtelDatabaseClient, eventBus event.EventBusInterface, schemaRouter DatabaseRoutingSchemaInterface) ProfileWriterService {
	return &profileWriterService{repo: db, eventBus: eventBus, schemaRouter: schemaRouter}
}

// Preserve large integer attribute values rather than rounding through float64.
func unmarshalProfileJSON(raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return nil
	}
	return v
}
