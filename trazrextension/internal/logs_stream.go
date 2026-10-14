package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// StreamLogs streams logs with preload and dynamic filter updates
func StreamLogs(repo repository.LogStreamRepository, querySvc repository.LogQueryService, conn *websocket.Conn, r *http.Request) {
	// Use common query parameter parsing
	commonParams := ParseCommonQueryParams(r, 1000)
	schema, allowed := authorizedStreamSchema(r)
	if !allowed {
		return
	}

	qv := r.URL.Query()
	// Use snake_case parameter names to match OpenAPI spec
	serviceIn := qv["service_in"]
	severityMin := atoiPtr(qv.Get("severity_min"))
	severityMax := atoiPtr(qv.Get("severity_max"))
	severityTextIn := qv["severity_text_in"]
	severityGroupIn := upperSlice(qv["severity_group_in"])
	q := qv.Get("q")
	qMode := strings.ToLower(qv.Get("q_mode"))
	qFlags := qv["q_flags"]
	// deepObject attr filters via exported helpers
	attrEq := ExtractDeepObjectStringMap(qv, "attr.eq[")
	attrIn := ExtractDeepObjectCSVMap(qv, "attr.in[")
	attrExists := ExtractDeepObjectBoolMap(qv, "attr.exists[")
	resAttrEq := ExtractDeepObjectStringMap(qv, "res_attr.eq[")
	resAttrIn := ExtractDeepObjectCSVMap(qv, "res_attr.in[")
	resAttrExists := ExtractDeepObjectBoolMap(qv, "res_attr.exists[")

	waitForEstablished := parseBool(qv.Get("wait_for_established"))

	ctx := r.Context()

	fmt.Printf("[StreamLogs] %s Starting WebSocket stream\n", time.Now().Format("2006-01-02 15:04:05"))

	logCh, cancel, establishedC, err := repo.SubscribeLogs(ctx, repository.LogFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
	if err != nil {
		fmt.Printf("[StreamLogs] %s SubscribeLogs error: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}
	defer cancel()

	if waitForEstablished {
		<-establishedC
	}

	// Preload if cursor or time parameters are provided
	if commonParams.Cursor != "" || commonParams.Start != nil {
		var preQ repository.LogQuery

		if commonParams.Cursor != "" {
			// Cursor-based preload - ignore time parameters when cursor is present
			preQ = repository.LogQuery{
				SeverityMin:    severityMin,
				SeverityMax:    severityMax,
				SeverityGroups: severityGroupIn,
				SeverityTextIn: severityTextIn,
			}
			// Set embedded CommonBaseFilters fields
			preQ.CommonBaseFilters.Cursor = commonParams.Cursor
			preQ.CommonBaseFilters.ServiceNames = serviceIn
			preQ.CommonBaseFilters.Schema = commonParams.Schema
			preQ.CommonBaseFilters.Direction = commonParams.Direction
			preQ.CommonBaseFilters.Limit = commonParams.Limit
			// Set embedded CommonAdvancedFilters fields
			preQ.CommonAdvancedFilters.Q = q
			preQ.CommonAdvancedFilters.QMode = qMode
			preQ.CommonAdvancedFilters.QFlags = qFlags
			preQ.CommonAdvancedFilters.AttrEq = attrEq
			preQ.CommonAdvancedFilters.AttrIn = attrIn
			preQ.CommonAdvancedFilters.AttrExists = attrExists
			preQ.CommonAdvancedFilters.ResAttrEq = resAttrEq
			preQ.CommonAdvancedFilters.ResAttrIn = resAttrIn
			preQ.CommonAdvancedFilters.ResAttrExists = resAttrExists
		} else {
			// Time-based preload (existing behavior)
			preQ = repository.LogQuery{
				TimeRange:      repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
				SeverityMin:    severityMin,
				SeverityMax:    severityMax,
				SeverityGroups: severityGroupIn,
				SeverityTextIn: severityTextIn,
			}
			// Set embedded CommonBaseFilters fields
			preQ.CommonBaseFilters.ServiceNames = serviceIn
			preQ.CommonBaseFilters.Schema = commonParams.Schema
			preQ.CommonBaseFilters.Direction = commonParams.Direction
			preQ.CommonBaseFilters.Limit = commonParams.Limit
			// Set embedded CommonAdvancedFilters fields
			preQ.CommonAdvancedFilters.Q = q
			preQ.CommonAdvancedFilters.QMode = qMode
			preQ.CommonAdvancedFilters.QFlags = qFlags
			preQ.CommonAdvancedFilters.AttrEq = attrEq
			preQ.CommonAdvancedFilters.AttrIn = attrIn
			preQ.CommonAdvancedFilters.AttrExists = attrExists
			preQ.CommonAdvancedFilters.ResAttrEq = resAttrEq
			preQ.CommonAdvancedFilters.ResAttrIn = resAttrIn
			preQ.CommonAdvancedFilters.ResAttrExists = resAttrExists
		}

		if res, err := querySvc.QueryLogs(ctx, preQ); err == nil {
			consecutiveWriteErrors := 0
			for _, e := range res.Entries {
				if err := wsWriteWithTimeout(conn, e, &consecutiveWriteErrors); err != nil {
					return
				}
			}
		}
		// Optional ready marker to signal preload complete
		_ = SafeWriteJSON(conn, map[string]string{"type": "ready"})
	}

	fmt.Printf("[StreamLogs] WebSocket connection established\n")
	defer func() {
		_ = SafeWriteControl(conn, websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "log stream ended"), time.Now().Add(time.Second))
		conn.Close()
	}()

	// Build initial group ranges and regex
	grpRanges := buildSeverityGroupRanges(severityGroupIn)
	var compiled *regexp.Regexp
	if qMode == "regex" && strings.TrimSpace(q) != "" {
		if re := compileRegex(q, qFlags); re != nil {
			compiled = re
		}
	}

	state := &filterState{
		serviceIn:       serviceIn,
		severityMin:     severityMin,
		severityMax:     severityMax,
		severityTextIn:  severityTextIn,
		severityGroups:  severityGroupIn,
		groupRanges:     grpRanges,
		q:               q,
		qMode:           qMode,
		qFlags:          qFlags,
		compiledPattern: compiled,
		attrEq:          attrEq,
		attrIn:          attrIn,
		attrExists:      attrExists,
		resAttrEq:       resAttrEq,
		resAttrIn:       resAttrIn,
		resAttrExists:   resAttrExists,
	}

	// Reader goroutine for in-band set_filter
	type clientMsg struct {
		Type   string                 `json:"type"`
		Filter map[string]interface{} `json:"filter"`
	}
	ack := func() { _ = SafeWriteJSON(conn, map[string]string{"type": "ack"}) }
	sendErr := func(msg string) { _ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": msg}) }
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m clientMsg
			if err := json.Unmarshal(data, &m); err != nil {
				sendErr("invalid message")
				continue
			}
			switch m.Type {
			case "set_filter":
				// Handle snake_case parameter names from OpenAPI spec
				if v, ok := m.Filter["service_in"].([]interface{}); ok {
					state.serviceIn = toStringSlice(v)
				}
				if v, ok := m.Filter["severity_min"].(float64); ok {
					i := int(v)
					state.severityMin = &i
				}
				if v, ok := m.Filter["severity_max"].(float64); ok {
					i := int(v)
					state.severityMax = &i
				}
				if v, ok := m.Filter["severity_text_in"].([]interface{}); ok {
					state.severityTextIn = toStringSlice(v)
				}
				if v, ok := m.Filter["severity_group_in"].([]interface{}); ok {
					state.severityGroups = upperSlice(toStringSlice(v))
					state.groupRanges = buildSeverityGroupRanges(state.severityGroups)
				}
				if v, ok := m.Filter["q"].(string); ok {
					state.q = v
				}
				if v, ok := m.Filter["q_mode"].(string); ok {
					state.qMode = strings.ToLower(v)
				}
				if v, ok := m.Filter["q_flags"].([]interface{}); ok {
					state.qFlags = toStringSlice(v)
				}
				// recompile regex if needed
				state.compiledPattern = nil
				if state.qMode == "regex" && strings.TrimSpace(state.q) != "" {
					if re := compileRegex(state.q, state.qFlags); re != nil {
						state.compiledPattern = re
					}
				}
				if v, ok := m.Filter["attr.eq"].(map[string]interface{}); ok {
					state.attrEq = toStringMap(v)
				}
				if v, ok := m.Filter["attr.in"].(map[string]interface{}); ok {
					state.attrIn = toCSVMap(v)
				}
				if v, ok := m.Filter["attr.exists"].(map[string]interface{}); ok {
					state.attrExists = toBoolMap(v)
				}
				if v, ok := m.Filter["res_attr.eq"].(map[string]interface{}); ok {
					state.resAttrEq = toStringMap(v)
				}
				if v, ok := m.Filter["res_attr.in"].(map[string]interface{}); ok {
					state.resAttrIn = toCSVMap(v)
				}
				if v, ok := m.Filter["res_attr.exists"].(map[string]interface{}); ok {
					state.resAttrExists = toBoolMap(v)
				}
				ack()
			case "keepalive", "ping":
				// Ignore client keepalive or ping messages to avoid errors.
				// Optionally, we could ack: _ = conn.WriteJSON(map[string]string{"type":"ack"})
				continue
			default:
				sendErr("unknown message type")
			}
		}
	}()

	consecutiveWriteErrors := 0
	droppedSinceNotice := 0
	for entry := range logCh {
		if !matchLogStream(entry, state) {
			continue
		}
		prev := consecutiveWriteErrors
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return
		}
		if consecutiveWriteErrors > prev {
			droppedSinceNotice += (consecutiveWriteErrors - prev)
			if droppedSinceNotice >= 10 {
				_ = SafeWriteJSON(conn, map[string]interface{}{"type": "dropped", "count": droppedSinceNotice})
				droppedSinceNotice = 0
			}
		}
	}
}

func matchLogStream(entry repository.LogEntryResponse, st *filterState) bool {
	if len(st.serviceIn) > 0 {
		ok := false
		for _, s := range st.serviceIn {
			if entry.ServiceName == s {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if st.severityMin != nil && int(entry.SeverityNumber) < *st.severityMin {
		return false
	}
	if st.severityMax != nil && int(entry.SeverityNumber) > *st.severityMax {
		return false
	}
	if len(st.groupRanges) > 0 {
		ok := false
		for _, rg := range st.groupRanges {
			if int(entry.SeverityNumber) >= rg[0] && int(entry.SeverityNumber) <= rg[1] {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(st.severityTextIn) > 0 {
		ok := false
		et := strings.ToLower(entry.SeverityText)
		for _, s := range st.severityTextIn {
			if et == strings.ToLower(s) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if strings.TrimSpace(st.q) != "" {
		if st.qMode == "regex" {
			if st.compiledPattern == nil || !st.compiledPattern.MatchString(entry.Body) {
				return false
			}
		} else {
			if !strings.Contains(strings.ToLower(entry.Body), strings.ToLower(st.q)) {
				return false
			}
		}
	}
	attrMap, _ := entry.Attributes.(map[string]interface{})
	resMap, _ := entry.ResourceAttributes.(map[string]interface{})
	for k, v := range st.attrEq {
		if !equalAttr(attrMap[k], v) {
			return false
		}
	}
	for k, v := range st.resAttrEq {
		if !equalAttr(resMap[k], v) {
			return false
		}
	}
	for k, list := range st.attrIn {
		if !inList(attrMap[k], list) {
			return false
		}
	}
	for k, list := range st.resAttrIn {
		if !inList(resMap[k], list) {
			return false
		}
	}
	for k, want := range st.attrExists {
		_, ok := attrMap[k]
		if want && !ok {
			return false
		}
		if !want && ok {
			return false
		}
	}
	for k, want := range st.resAttrExists {
		_, ok := resMap[k]
		if want && !ok {
			return false
		}
		if !want && ok {
			return false
		}
	}
	return true
}
