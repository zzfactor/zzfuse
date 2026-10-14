package internal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

func profileDTOFixture(t *testing.T) ProfileEntryDTO {
	t.Helper()
	raw, err := os.ReadFile("../../pkg/repository/testdata/current-profile-row0.pb")
	if err != nil {
		t.Fatal(err)
	}
	version := repository.ProfileSchemaVersion
	return ProfileEntryDTO{Timestamp: "2026-05-03T03:09:37.123456789Z", ProfileStorageVersion: 1, ProfileEncoding: repository.ProfileEncodingOTLP, ProfileSchemaVersion: &version, ProfileBytes: base64.StdEncoding.EncodeToString(raw)}
}
func TestProfileDTODerivesVersionedSummaries(t *testing.T) {
	dto := profileDTOFixture(t)
	dto.ServiceName = "forged"
	dto.ProfileType = "forged"
	dto.DurationNS = -7
	entry, err := dto.ToProfileEntry()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(dto.ProfileBytes)
	if entry.ProfileStorageVersion != 1 || entry.ProfileEncoding != repository.ProfileEncodingOTLP || *entry.OtelProfileID != "0102030405060708090a0b0c0d0e0f10" || *entry.ProfileTimeUnixNano != "1777777777123456789" || entry.ServiceName != "profile-review" || entry.ProfileType != "cpu" || entry.DurationNS != 9000000000 || !bytes.Equal(raw, entry.ProfileBytes) {
		t.Fatalf("versioned DTO ignored declaration or trusted conflicting summaries: %+v", entry)
	}
}
func TestProfileInsertRejectsInvalidVersionedPayloadBeforeWriter(t *testing.T) {
	cases := map[string]func(*ProfileEntryDTO){
		"invalid protobuf": func(d *ProfileEntryDTO) { d.ProfileBytes = base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe, 0}) },
		"empty protobuf":   func(d *ProfileEntryDTO) { d.ProfileBytes = "" },
		"old protobuf": func(d *ProfileEntryDTO) {
			raw, err := os.ReadFile("../../pkg/repository/testdata/profiles-0.135/legacy-0.135-profile-row.pb")
			if err != nil {
				t.Fatal(err)
			}
			d.ProfileBytes = base64.StdEncoding.EncodeToString(raw)
		},
		"unsupported storage":             func(d *ProfileEntryDTO) { d.ProfileStorageVersion = 2 },
		"unsupported schema":              func(d *ProfileEntryDTO) { v := "99"; d.ProfileSchemaVersion = &v },
		"missing schema":                  func(d *ProfileEntryDTO) { d.ProfileSchemaVersion = nil },
		"wrong encoding":                  func(d *ProfileEntryDTO) { d.ProfileEncoding = "json" },
		"modern encoding without version": func(d *ProfileEntryDTO) { d.ProfileStorageVersion = 0 },
		"multiple profiles": func(d *ProfileEntryDTO) {
			raw, err := os.ReadFile("../../pkg/repository/testdata/current-profiles.pb")
			if err != nil {
				t.Fatal(err)
			}
			d.ProfileBytes = base64.StdEncoding.EncodeToString(raw)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dto := profileDTOFixture(t)
			mutate(&dto)
			body, err := json.Marshal(dto)
			if err != nil {
				t.Fatal(err)
			}
			probe := &schemaProbe{}
			handler := NewProfileInsertHandler(&schemaProfileService{probe: probe})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles?schema=tenant_a", bytes.NewReader(body))
			req = req.WithContext(auth.WithUserContext(req.Context(), &auth.UserContext{Schemas: []string{"tenant_a"}}))
			response := httptest.NewRecorder()
			handler(response, req)
			if response.Code != http.StatusBadRequest || probe.calls != 0 {
				t.Fatalf("invalid payload got %d with %d writes: %s", response.Code, probe.calls, response.Body.String())
			}
		})
	}
}
func TestProfileInsertModernAndLegacyContracts(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "modern", true: "legacy"}[legacy], func(t *testing.T) {
			dto := profileDTOFixture(t)
			if legacy {
				dto.ProfileStorageVersion = 0
				dto.ProfileEncoding = ""
				dto.ProfileSchemaVersion = nil
				raw, _ := os.ReadFile("../../pkg/repository/testdata/profiles-0.135/legacy-0.135-profile-row.pb")
				dto.ProfileBytes = base64.StdEncoding.EncodeToString(raw)
			}
			body, _ := json.Marshal(dto)
			probe := &schemaProbe{}
			handler := NewProfileInsertHandler(&schemaProfileService{probe: probe})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles?schema=tenant_a", bytes.NewReader(body))
			req = req.WithContext(auth.WithUserContext(req.Context(), &auth.UserContext{Schemas: []string{"tenant_a"}}))
			response := httptest.NewRecorder()
			handler(response, req)
			if response.Code != http.StatusOK || probe.calls != 1 || response.Body.String() != "{\"ids\":[42],\"status\":\"inserted\"}\n" {
				t.Fatalf("insert contract changed: %d %s writes %d", response.Code, response.Body.String(), probe.calls)
			}
		})
	}
}
