package repository

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

func profileFixture(t *testing.T, name string) pprofile.Profiles {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	return pd
}
func TestProfileStorageRoundTrip(t *testing.T) {
	pd := profileFixture(t, "current-profile-row0.pb")
	initialAttributes := pd.ResourceProfiles().At(0).Resource().Attributes().AsRaw()
	initialScope := pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Scope().Attributes().AsRaw()
	before := pprofile.NewProfiles()
	pd.CopyTo(before)
	entry, err := ProfileEntryFromProfiles(pd, time.Unix(1700000000, 123).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if *entry.OtelProfileID != "0102030405060708090a0b0c0d0e0f10" || *entry.ProfileTimeUnixNano != "1777777777123456789" || *entry.DurationNano != "9000000000" || entry.TimestampNS != 1777777777123456789 || entry.DurationNS != 9000000000 {
		t.Fatalf("source identity/timing lost: %+v", entry)
	}
	decoded, err := DecodeStoredProfile(entry)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(before)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, actual) {
		t.Fatalf("complete dictionary/context/sample roundtrip changed\nexpected %s\nactual %s", expected, actual)
	}
	response := toProfileEntryResponse(entry)
	responseJSON, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if response.ProfileDecodeStatus != "decoded" || *response.ProfileDroppedAttributesCount != 4 || *response.ResourceDroppedAttributesCount != 3 || *response.ScopeDroppedAttributesCount != 2 || *response.DroppedAttrCount != 4 {
		t.Fatalf("incorrect decoded dropped counts: %+v", response)
	}
	for _, v := range []string{`"9007199254740993"`, `"-5"`, `"0"`, `"1777777777123456789"`, `"originalPayload":"AAF/gP8KDQ=="`, `"stackTable"`, `"dictionary"`} {
		if !bytes.Contains(responseJSON, []byte(v)) {
			t.Fatalf("API JSON lost %s: %s", v, responseJSON)
		}
	}
	raw, err := base64.StdEncoding.DecodeString(response.ProfileBytes)
	if err != nil || !bytes.Equal(raw, entry.ProfileBytes) {
		t.Fatal("raw bytes changed in response")
	}
	// Marshal the still-mutable caller data only after conversion, proving the
	// exporter codec did not intern away resource/scope string attributes.
	originalResource := pd.ResourceProfiles().At(0).Resource().Attributes().AsRaw()
	if !reflect.DeepEqual(originalResource, initialAttributes) || !reflect.DeepEqual(pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Scope().Attributes().AsRaw(), initialScope) {
		t.Fatal("source resource attributes mutated")
	}
}

func TestProfileUnsignedTimingAndDroppedCounts(t *testing.T) {
	pd := profileFixture(t, "current-profile-row0.pb")
	p := pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
	p.SetTime(pcommon.Timestamp(math.MaxUint64))
	p.SetDurationNano(math.MaxUint64)
	p.SetDroppedAttributesCount(math.MaxUint32)
	pd.ResourceProfiles().At(0).Resource().SetDroppedAttributesCount(math.MaxUint32)
	pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Scope().SetDroppedAttributesCount(math.MaxUint32)
	ingestion := time.Unix(1778888888, 4321).UTC()
	entry, err := ProfileEntryFromProfiles(pd, ingestion)
	if err != nil {
		t.Fatal(err)
	}
	if entry.TimestampNS != ingestion.UnixNano() || entry.DurationNS != 0 || *entry.ProfileTimeUnixNano != "18446744073709551615" || *entry.DurationNano != "18446744073709551615" || entry.ScopeDroppedAttrCount != nil {
		t.Fatalf("unsigned values overflowed summaries: %+v", entry)
	}
	response := toProfileEntryResponse(entry)
	if response.ProfileDecodeStatus != "decoded" || *response.ScopeDroppedAttributesCount != math.MaxUint32 || *response.ResourceDroppedAttributesCount != math.MaxUint32 || *response.ProfileDroppedAttributesCount != math.MaxUint32 || response.DroppedAttrCount != nil {
		t.Fatal("uint32 dropped count lost")
	}
	p.SetTime(0)
	entry, err = ProfileEntryFromProfiles(pd, ingestion)
	if err != nil || entry.TimestampNS != ingestion.UnixNano() || *entry.ProfileTimeUnixNano != "0" {
		t.Fatal("zero source time not retained separately from index")
	}
}

func TestProfileLegacyOpaqueBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/profiles-0.135/legacy-0.135-profile-row.pb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw); err == nil {
		t.Fatal("fixture no longer proves development schema incompatibility")
	}
	entry := ProfileEntry{ID: 72, ProfileBytes: raw, ProfileType: "cpu", DurationNS: 9000000000, Attributes: []byte(`{"legacy":true}`), Timestamp: time.Unix(1777777777, 123456789), TimestampNS: 1777777777123456789}
	response := toProfileEntryResponse(entry)
	if response.ProfileEncoding != ProfileEncodingLegacy || response.ProfileDecodeStatus != "legacy-opaque" || response.ProfileData != nil || response.DurationNS != 9000000000 || response.ProfileID != 72 {
		t.Fatalf("legacy row was reinterpreted: %+v", response)
	}
	view, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var saved ProfileEntryResponse
	if err = json.Unmarshal(view, &saved); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(saved.ProfileBytes)
	if err != nil || !bytes.Equal(decoded, raw) {
		t.Fatal("legacy binary corrupted through JSON response")
	}
	// Damaged or unsupported versioned rows remain retrievable without discarding bytes.
	version := ProfileSchemaVersion
	entry.ProfileStorageVersion = 1
	entry.ProfileEncoding = ProfileEncodingOTLP
	entry.ProfileSchemaVersion = &version
	response = toProfileEntryResponse(entry)
	if response.ProfileDecodeStatus != "error" || response.ProfileDecodeError == "" || response.ProfileData != nil || response.ProfileBytes != base64.StdEncoding.EncodeToString(raw) {
		t.Fatal("invalid modern declaration silently decoded or lost bytes")
	}
}

func TestProfileDictionaryValidation(t *testing.T) {
	cases := map[string]func(pprofile.Profiles){
		"sample stack negative": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples().At(0).SetStackIndex(-1)
		},
		"sample link upper bound": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples().At(0).SetLinkIndex(int32(pd.Dictionary().LinkTable().Len()))
		},
		"sample attribute": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples().At(0).AttributeIndices().FromRaw([]int32{-1})
		},
		"profile attribute": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).AttributeIndices().FromRaw([]int32{-1})
		},
		"sample type": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).SampleType().SetTypeStrindex(-1)
		},
		"period unit": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).PeriodType().SetUnitStrindex(-1)
		},
		"stack location":   func(pd pprofile.Profiles) { pd.Dictionary().StackTable().At(1).LocationIndices().FromRaw([]int32{-1}) },
		"location mapping": func(pd pprofile.Profiles) { pd.Dictionary().LocationTable().At(1).SetMappingIndex(-1) },
		"location attribute": func(pd pprofile.Profiles) {
			pd.Dictionary().LocationTable().At(1).AttributeIndices().FromRaw([]int32{-1})
		},
		"line function":    func(pd pprofile.Profiles) { pd.Dictionary().LocationTable().At(1).Lines().At(0).SetFunctionIndex(-1) },
		"mapping filename": func(pd pprofile.Profiles) { pd.Dictionary().MappingTable().At(1).SetFilenameStrindex(-1) },
		"mapping attribute": func(pd pprofile.Profiles) {
			pd.Dictionary().MappingTable().At(1).AttributeIndices().FromRaw([]int32{-1})
		},
		"function name":        func(pd pprofile.Profiles) { pd.Dictionary().FunctionTable().At(1).SetNameStrindex(-1) },
		"function system name": func(pd pprofile.Profiles) { pd.Dictionary().FunctionTable().At(1).SetSystemNameStrindex(-1) },
		"function filename":    func(pd pprofile.Profiles) { pd.Dictionary().FunctionTable().At(1).SetFilenameStrindex(-1) },
		"attribute key":        func(pd pprofile.Profiles) { pd.Dictionary().AttributeTable().At(1).SetKeyStrindex(-1) },
		"attribute unit":       func(pd pprofile.Profiles) { pd.Dictionary().AttributeTable().At(1).SetUnitStrindex(-1) },
		"zero sentinel":        func(pd pprofile.Profiles) { pd.Dictionary().StringTable().SetAt(0, "invalid") },
		"sample lengths": func(pd pprofile.Profiles) {
			pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples().At(0).Values().FromRaw([]int64{1})
		},
		"empty sample": func(pd pprofile.Profiles) {
			s := pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples().At(0)
			s.Values().FromRaw(nil)
			s.TimestampsUnixNano().FromRaw(nil)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			pd := profileFixture(t, "current-profile-row0.pb")
			mutate(pd)
			if err := ValidateProfiles(pd); err == nil {
				t.Fatal("invalid profile accepted")
			}
			if _, err := ProfileEntryFromProfiles(pd, time.Now()); err == nil {
				t.Fatal("invalid profile serialized")
			}
		})
	}
	if err := ValidateProfiles(pprofile.NewProfiles()); err == nil {
		t.Fatal("missing zero tables accepted")
	}
}

func TestProfileSummaryKeepsLargeAttributeNumbers(t *testing.T) {
	response := toProfileEntryResponse(ProfileEntry{Attributes: []byte(`{"integer":9007199254740993}`)})
	raw, err := json.Marshal(response.Attributes)
	if err != nil || !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("integer attribute rounded")
	}
}

func TestProfileNonfiniteAndNestedAttributeSummaries(t *testing.T) {
	pd := profileFixture(t, "current-profile-row0.pb")
	rp := pd.ResourceProfiles().At(0)
	sp := rp.ScopeProfiles().At(0)
	rp.Resource().Attributes().PutDouble("not-a-number", math.NaN())
	rp.Resource().Attributes().PutEmptyMap("nested").PutEmptySlice("values").AppendEmpty().SetDouble(math.Inf(1))
	sp.Scope().Attributes().PutDouble("negative-infinity", math.Inf(-1))
	a := pd.Dictionary().AttributeTable().At(1)
	a.Value().SetEmptyMap().PutEmptySlice("values").AppendEmpty().SetDouble(math.NaN())
	entry, err := ProfileEntryFromProfiles(pd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{entry.Attributes, entry.ResourceAttributes, entry.ScopeAttributes} {
		if !json.Valid(raw) {
			t.Fatal("nonfinite summary invalid JSON")
		}
	}
	if !bytes.Contains(entry.Attributes, []byte(`"NaN"`)) || !bytes.Contains(entry.ResourceAttributes, []byte(`"Infinity"`)) || !bytes.Contains(entry.ScopeAttributes, []byte(`"-Infinity"`)) {
		t.Fatal("nonfinite summary values discarded")
	}
	decoded, err := DecodeStoredProfile(entry)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := decoded.ResourceProfiles().At(0).Resource().Attributes().Get("not-a-number")
	if !math.IsNaN(value.Double()) {
		t.Fatal("wire nonfinite double lost")
	}
	if response := toProfileEntryResponse(entry); response.ProfileDecodeStatus != "decoded" {
		t.Fatal("OTLP JSON could not expose nonfinite payload", response.ProfileDecodeError)
	}
}
func TestProfileAttributeKeyUniquenessAndPayloadPairing(t *testing.T) {
	for _, name := range []string{"duplicate key", "payload without format", "format without payload", "empty function"} {
		t.Run(name, func(t *testing.T) {
			pd := profileFixture(t, "current-profile-row0.pb")
			p := pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
			switch name {
			case "duplicate key":
				d := pd.Dictionary()
				key := d.StringTable().At(int(d.AttributeTable().At(1).KeyStrindex()))
				d.StringTable().Append(key)
				a := d.AttributeTable().AppendEmpty()
				a.SetKeyStrindex(int32(d.StringTable().Len() - 1))
				a.Value().SetStr("different value")
				p.AttributeIndices().FromRaw([]int32{1, int32(d.AttributeTable().Len() - 1)})
			case "payload without format":
				p.SetOriginalPayloadFormat("")
			case "format without payload":
				p.OriginalPayload().FromRaw(nil)
			case "empty function":
				f := pd.Dictionary().FunctionTable().At(1)
				f.SetNameStrindex(0)
				f.SetSystemNameStrindex(0)
				f.SetFilenameStrindex(0)
			}
			if err := ValidateProfiles(pd); err == nil {
				t.Fatal("malformed profile accepted")
			}
		})
	}
}
func TestProfileTimestampOnlyAndValueOnlySamples(t *testing.T) {
	pd := profileFixture(t, "current-profile-row0.pb")
	p := pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
	p.Samples().At(0).TimestampsUnixNano().FromRaw(nil)
	s := p.Samples().AppendEmpty()
	s.SetStackIndex(1)
	s.TimestampsUnixNano().FromRaw([]uint64{math.MaxUint64})
	entry, err := ProfileEntryFromProfiles(pd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStoredProfile(entry)
	if err != nil {
		t.Fatal(err)
	}
	samples := decoded.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples()
	if samples.At(0).Values().Len() != 4 || samples.At(0).TimestampsUnixNano().Len() != 0 || samples.At(1).Values().Len() != 0 || samples.At(1).TimestampsUnixNano().At(0) != math.MaxUint64 {
		t.Fatal("one-sided sample shapes changed")
	}
}

func TestProfileInconsistentStoredDeclarationRetainsBytes(t *testing.T) {
	version := ProfileSchemaVersion
	for _, e := range []ProfileEntry{
		{ProfileStorageVersion: 0, ProfileEncoding: ProfileEncodingOTLP, ProfileSchemaVersion: &version, ProfileBytes: []byte(`{"legacy":true}`)},
		{ProfileStorageVersion: 0, ProfileEncoding: "unknown", ProfileBytes: []byte(`{"legacy":true}`)},
		{ProfileStorageVersion: 1, ProfileEncoding: ProfileEncodingOTLP, ProfileSchemaVersion: &version, ProfileBytes: []byte(`{"legacy":true}`)},
	} {
		response := toProfileEntryResponse(e)
		if response.ProfileDecodeStatus != "error" || response.ProfileDecodeError == "" || response.ProfileData != nil || response.ProfileBytes != base64.StdEncoding.EncodeToString(e.ProfileBytes) {
			t.Fatal("inconsistent declaration silently treated as legacy JSON")
		}
	}
}
