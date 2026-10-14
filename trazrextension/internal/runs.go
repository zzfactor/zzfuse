package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// RunEntryDTO represents a run entry in API requests
type RunEntryDTO struct {
	ServiceName      string                 `json:"service_name"`
	ScopeName        *string                `json:"scope_name,omitempty"`
	Name             string                 `json:"name"`
	StartTimestampNs int64                  `json:"start_timestamp_ns"`
	EndTimestampNs   *int64                 `json:"end_timestamp_ns,omitempty"`
	TraceID          *string                `json:"trace_id,omitempty"`
	ProcessorID      *int64                 `json:"processor_id,omitempty"`
	RunType          *int16                 `json:"run_type,omitempty"`
	SessionID        *string                `json:"session_id,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// StreamRunsWSWithPreload is a standardized alias that forwards to StreamFilteredRunsWSWithPreload.
// Prefer using this name in new call sites; the legacy name remains for compatibility.
func StreamRunsWSWithPreload(repo repository.RunStreamRepository, querySvc repository.RunQueryService, conn *websocket.Conn, r *http.Request) {
	StreamFilteredRunsWSWithPreload(repo, querySvc, conn, r)
}

// ToRunEntry converts RunEntryDTO to repository.RunEntry
func (dto RunEntryDTO) ToRunEntry() (repository.RunEntry, error) {
	return repository.RunEntry{
		ServiceName:      dto.ServiceName,
		ScopeName:        dto.ScopeName,
		Name:             dto.Name,
		StartTimestampNs: dto.StartTimestampNs,
		EndTimestampNs:   dto.EndTimestampNs,
		TraceID:          dto.TraceID,
		ProcessorID:      dto.ProcessorID,
		RunType:          dto.RunType,
		SessionID:        dto.SessionID,
		Metadata:         dto.Metadata,
	}, nil
}

// NewRunQueryV1Handler returns an http.HandlerFunc for GET /api/v1/runs
func NewRunQueryV1Handler(service repository.RunQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Collect all validation errors
		validationErrors := &ValidationErrors{}

		// Parse common query parameters using DRY helper
		commonParams := ParseCommonQueryParams(r, 100)
		if !requireSchemaAccess(w, r, commonParams.Schema) {
			return
		}

		// Extract runs-specific parameters
		name := r.URL.Query().Get("name")
		sessionID := r.URL.Query().Get("session_id")
		runTypeStr := r.URL.Query().Get("run_type")
		activeOnlyStr := r.URL.Query().Get("active_only")

		// Use common parameters
		validationErrors = commonParams.ValidationErrors
		start := commonParams.Start
		end := commonParams.End
		serviceName := commonParams.ServiceName
		limit := commonParams.Limit
		direction := commonParams.Direction
		cursor := commonParams.Cursor
		schema := commonParams.Schema

		// Parse and validate run type
		var runType *int16
		if runTypeStr != "" {
			if rt, err := strconv.ParseInt(runTypeStr, 10, 16); err == nil {
				rt16 := int16(rt)
				runType = &rt16
			} else {
				validationErrors.AddError("run_type", "run_type must be a valid number")
			}
		}

		activeOnly := false
		if activeOnlyStr == "true" {
			activeOnly = true
		}

		// Check for validation errors before proceeding
		if validationErrors.HasErrors() {
			WriteValidationErrors(w, validationErrors)
			return
		}

		q := repository.RunQuery{
			TimeRange:   repository.TimeRange{Start: start, End: end},
			ServiceName: serviceName,
			Name:        name,
			SessionID:   sessionID,
			RunType:     runType,
			ActiveOnly:  activeOnly,
			Limit:       limit,
			Cursor:      cursor,
			Schema:      schema,
			Direction:   direction,
		}

		result, err := service.QueryRuns(ctx, q)
		if err != nil {
			sanitizedError := SanitizeDatabaseError(err)
			WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
			return
		}

		WriteJSONResponse(w, http.StatusOK, result)
	}
}

// NewRunInsertHandler returns an http.HandlerFunc for POST /api/v1/runs
func NewRunInsertHandler(runWriter repository.RunWriterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := r.URL.Query().Get("schema")
		if schema == "" {
			schema = DefaultSchema
		}
		if !requireSchemaAccess(w, r, schema) {
			return
		}

		var dtos []RunEntryDTO
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeInsertError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if err := json.Unmarshal(body, &dtos); err != nil {
			// Try single object
			var singleDTO RunEntryDTO
			if err2 := json.Unmarshal(body, &singleDTO); err2 != nil {
				writeInsertError(w, http.StatusBadRequest, "invalid run entry format")
				return
			}
			dtos = []RunEntryDTO{singleDTO}
		}
		if len(dtos) == 0 {
			writeInsertError(w, http.StatusBadRequest, "no run entries provided")
			return
		}
		var entries []repository.RunEntry
		for _, dto := range dtos {
			entry, err := dto.ToRunEntry()
			if err != nil {
				writeInsertError(w, http.StatusBadRequest, err.Error())
				return
			}
			entries = append(entries, entry)
		}
		ids, err := runWriter.InsertRuns(r.Context(), entries, schema)
		if err != nil {
			sanitizedError := SanitizeDatabaseError(err)
			writeInsertError(w, http.StatusInternalServerError, sanitizedError)
			return
		}

		// Convert int64 IDs to strings for consistency with other APIs
		idStrings := make([]string, len(ids))
		for i, id := range ids {
			idStrings[i] = strconv.FormatInt(id, 10)
		}
		writeInsertResult(w, idStrings)
	}
}

// NewRunUpdateHandler returns an http.HandlerFunc for PUT /api/v1/runs
func NewRunUpdateHandler(runWriter repository.RunWriterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := r.URL.Query().Get("schema")
		if schema == "" {
			schema = DefaultSchema
		}
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		var request struct {
			ID       int64                  `json:"id"`
			Metadata map[string]interface{} `json:"metadata"`
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "failed to read body")
			return
		}

		if err := json.Unmarshal(body, &request); err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "invalid request format")
			return
		}

		err = runWriter.UpdateRunMetadata(r.Context(), request.ID, request.Metadata, schema)
		if err != nil {
			sanitizedError := SanitizeDatabaseError(err)
			WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
			return
		}

		WriteJSONResponse(w, http.StatusOK, map[string]string{"Status": "updated"})
	}
}

// NewRunEndHandler returns an http.HandlerFunc for DELETE /api/v1/runs
func NewRunEndHandler(runWriter repository.RunWriterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := r.URL.Query().Get("schema")
		if schema == "" {
			schema = DefaultSchema
		}
		if !requireSchemaAccess(w, r, schema) {
			return
		}

		var request struct {
			Name           string `json:"name"`
			EndTimestampNs int64  `json:"end_timestamp_ns"`
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "failed to read body")
			return
		}

		if err := json.Unmarshal(body, &request); err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "invalid request format")
			return
		}

		err = runWriter.EndRun(r.Context(), request.Name, request.EndTimestampNs, schema)
		if err != nil {
			sanitizedError := SanitizeDatabaseError(err)
			WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
			return
		}

		WriteJSONResponse(w, http.StatusOK, map[string]string{"Status": "ended"})
	}
}

// StreamFilteredRunsWSWithPreload streams runs via WebSocket with optional preload window
func StreamFilteredRunsWSWithPreload(repo repository.RunStreamRepository, querySvc repository.RunQueryService, conn *websocket.Conn, r *http.Request) {
	// Use common query parameter parsing
	commonParams := ParseCommonQueryParams(r, 1000)
	schema, allowed := authorizedStreamSchema(r)
	if !allowed {
		return
	}

	qv := r.URL.Query()
	service := qv.Get("service")
	name := qv.Get("name")
	sessionID := qv.Get("session_id")
	runTypeStr := qv.Get("run_type")
	activeOnly := qv.Get("active_only") == "true"
	waitForEstablished := parseBool(qv.Get("wait_for_established"))

	ctx := r.Context()

	fmt.Printf("[StreamFilteredRunsWS] %s Starting WebSocket stream with filters: service=%s, name=%s, sessionID=%s, runType=%s, schema=%s, activeOnly=%v, waitForEstablished=%v\n",
		time.Now().Format(time.RFC3339), service, name, sessionID, runTypeStr, commonParams.Schema, activeOnly, waitForEstablished)

	// Parse run type filter
	var runType *int16
	if runTypeStr != "" {
		if rt, err := strconv.ParseInt(runTypeStr, 10, 16); err == nil {
			rt16 := int16(rt)
			runType = &rt16
		}
	}

	// Create filter
	filter := repository.RunFilter{
		Name:       name,
		SessionID:  sessionID,
		RunType:    runType,
		ActiveOnly: activeOnly,
	}
	filter.CommonBaseFilters.ServiceName = service
	filter.CommonBaseFilters.Schema = schema

	runCh, cancel, establishedC, err := repo.SubscribeRuns(ctx, filter)
	if err != nil {
		fmt.Printf("[StreamFilteredRunsWS] %s SubscribeRuns error: %v\n", time.Now().Format(time.RFC3339), err)
		return
	}
	defer cancel()

	if waitForEstablished {
		fmt.Printf("[StreamFilteredRunsWS] Waiting for subscription to be established...\n")
		<-establishedC
		fmt.Printf("[StreamFilteredRunsWS] Subscription established.\n")
	}

	fmt.Printf("[StreamFilteredRunsWS] WebSocket connection established\n")
	defer func() {
		fmt.Printf("[StreamFilteredRunsWS] Closing WebSocket connection\n")
		_ = SafeWriteControl(conn,
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "run stream ended"),
			time.Now().Add(time.Second),
		)
		conn.Close()
	}()

	// Ensure control frames (Ping/Pong) are processed during preload and live streaming
	startWSControlRead(ctx, conn)

	// Preload if cursor or time parameters are provided
	if commonParams.Cursor != "" || commonParams.Start != nil {
		var q repository.RunQuery

		if commonParams.Cursor != "" {
			// Cursor-based preload - ignore time parameters when cursor is present
			q = repository.RunQuery{
				Cursor:      commonParams.Cursor,
				ServiceName: service,
				Name:        name,
				SessionID:   sessionID,
				RunType:     runType,
				ActiveOnly:  activeOnly,
				Limit:       commonParams.Limit,
				Schema:      commonParams.Schema,
				Direction:   commonParams.Direction, // Respect user's direction preference
			}
		} else {
			// Time-based preload (existing behavior)
			q = repository.RunQuery{
				TimeRange:   repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
				ServiceName: service,
				Name:        name,
				SessionID:   sessionID,
				RunType:     runType,
				ActiveOnly:  activeOnly,
				Limit:       commonParams.Limit,
				Schema:      commonParams.Schema,
				Direction:   commonParams.Direction, // Respect user's direction preference
			}
		}
		if res, err := querySvc.QueryRuns(ctx, q); err == nil {
			// Send results in query order (respects user's direction preference)
			consecutiveWriteErrors := 0
			for _, e := range res.Entries {
				msg := map[string]interface{}{"type": "run_upsert", "run": e}
				if e.EndTimestampNs != nil {
					msg["type"] = "run_ended"
				}
				if err := wsWriteWithTimeout(conn, msg, &consecutiveWriteErrors); err != nil {
					return
				}
			}
			_ = SafeWriteJSON(conn, map[string]string{"type": "ready"})
		}
	}

	consecutiveWriteErrors := 0
	droppedSinceNotice := 0
	msgCount := 0
	fmt.Printf("[StreamFilteredRunsWS] Starting to listen for run entries...\n")

	for entry := range runCh {
		fmt.Printf("[StreamFilteredRunsWS] Received run entry: service=%s, name=%s, sessionID=%v\n",
			entry.ServiceName, entry.Name, entry.SessionID)

		msg := map[string]interface{}{"type": "run_upsert", "run": entry}
		if entry.EndTimestampNs != nil {
			msg["type"] = "run_ended"
		}
		prev := consecutiveWriteErrors
		if err := wsWriteWithTimeout(conn, msg, &consecutiveWriteErrors); err != nil {
			fmt.Printf("[StreamFilteredRunsWS] Error writing to WebSocket: %v\n", err)
			return
		}
		if consecutiveWriteErrors > prev {
			droppedSinceNotice++
			if droppedSinceNotice >= 10 { // send periodic notice
				_ = SafeWriteJSON(conn, map[string]interface{}{"type": "dropped", "count": droppedSinceNotice})
				droppedSinceNotice = 0
			}
		}
		msgCount++
		fmt.Printf("[StreamFilteredRunsWS] Sent run entry to client (total sent: %d)\n", msgCount)
	}
	fmt.Printf("[StreamFilteredRunsWS] Run channel closed, sent %d messages\n", msgCount)
}

func NewRunGetByIDHandler(service repository.RunQueryService) http.HandlerFunc {
	return CreateGetByIDHandler("/api/v1/runs/", service.GetRunByID)
}
