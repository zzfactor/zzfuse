package repository

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
)

const (
	ProfileStorageVersionCurrent int16 = 1
	ProfileEncodingLegacy              = "legacy-unversioned"
	ProfileEncodingOTLP                = "otlp-profiles-protobuf"
)

// ProfileSchemaVersion is the development wire version compiled into pdata, not
// a claim that earlier Collector releases with the same metadata are decodable.
const ProfileSchemaVersion = pprofileotlp.DevelopmentVersion

// ValidateProfiles verifies the current development schema's dictionary graph.
// Index zero is the required default entry in every dictionary table.
func ValidateProfiles(pd pprofile.Profiles) error {
	d := pd.Dictionary()
	if d.StringTable().Len() == 0 || d.AttributeTable().Len() == 0 || d.MappingTable().Len() == 0 || d.LocationTable().Len() == 0 || d.FunctionTable().Len() == 0 || d.LinkTable().Len() == 0 || d.StackTable().Len() == 0 {
		return fmt.Errorf("profile dictionary requires a zero entry in every table")
	}
	a, m, l, f, k, s := d.AttributeTable().At(0), d.MappingTable().At(0), d.LocationTable().At(0), d.FunctionTable().At(0), d.LinkTable().At(0), d.StackTable().At(0)
	if d.StringTable().At(0) != "" || a.KeyStrindex() != 0 || a.UnitStrindex() != 0 || a.Value().Type() != pcommon.ValueTypeEmpty || m.MemoryStart() != 0 || m.MemoryLimit() != 0 || m.FileOffset() != 0 || m.FilenameStrindex() != 0 || m.AttributeIndices().Len() != 0 || l.MappingIndex() != 0 || l.Address() != 0 || l.Lines().Len() != 0 || l.AttributeIndices().Len() != 0 || f.NameStrindex() != 0 || f.SystemNameStrindex() != 0 || f.FilenameStrindex() != 0 || f.StartLine() != 0 || !k.TraceID().IsEmpty() || !k.SpanID().IsEmpty() || s.LocationIndices().Len() != 0 {
		return fmt.Errorf("profile dictionary index zero must contain default values")
	}
	index := func(label string, v int32, length int) error {
		if v < 0 || int64(v) >= int64(length) {
			return fmt.Errorf("%s index %d outside [0,%d)", label, v, length)
		}
		return nil
	}
	strings := func(label string, v int32) error { return index(label, v, d.StringTable().Len()) }
	attributes := func(values pcommon.Int32Slice) error {
		keys := make(map[string]bool, values.Len())
		for i := 0; i < values.Len(); i++ {
			n := values.At(i)
			if err := index("attribute", n, d.AttributeTable().Len()); err != nil {
				return err
			}
			a := d.AttributeTable().At(int(n))
			if err := strings("attribute key", a.KeyStrindex()); err != nil {
				return err
			}
			key := d.StringTable().At(int(a.KeyStrindex()))
			if keys[key] {
				return fmt.Errorf("referenced attribute keys must be unique: %q", key)
			}
			keys[key] = true
		}
		return nil
	}
	for i := 0; i < d.AttributeTable().Len(); i++ {
		v := d.AttributeTable().At(i)
		if err := strings("attribute key", v.KeyStrindex()); err != nil {
			return err
		}
		if err := strings("attribute unit", v.UnitStrindex()); err != nil {
			return err
		}
	}
	for i := 0; i < d.MappingTable().Len(); i++ {
		v := d.MappingTable().At(i)
		if err := strings("mapping filename", v.FilenameStrindex()); err != nil {
			return err
		}
		if err := attributes(v.AttributeIndices()); err != nil {
			return err
		}
	}
	for i := 0; i < d.FunctionTable().Len(); i++ {
		v := d.FunctionTable().At(i)
		if i != 0 && v.NameStrindex() == 0 && v.SystemNameStrindex() == 0 && v.FilenameStrindex() == 0 {
			return fmt.Errorf("function requires name, system name, or filename")
		}
		for _, n := range []int32{v.NameStrindex(), v.SystemNameStrindex(), v.FilenameStrindex()} {
			if err := strings("function string", n); err != nil {
				return err
			}
		}
	}
	for i := 0; i < d.LocationTable().Len(); i++ {
		v := d.LocationTable().At(i)
		if err := index("location mapping", v.MappingIndex(), d.MappingTable().Len()); err != nil {
			return err
		}
		if err := attributes(v.AttributeIndices()); err != nil {
			return err
		}
		for j := 0; j < v.Lines().Len(); j++ {
			if err := index("line function", v.Lines().At(j).FunctionIndex(), d.FunctionTable().Len()); err != nil {
				return err
			}
		}
	}
	for i := 0; i < d.StackTable().Len(); i++ {
		v := d.StackTable().At(i).LocationIndices()
		for j := 0; j < v.Len(); j++ {
			if err := index("stack location", v.At(j), d.LocationTable().Len()); err != nil {
				return err
			}
		}
	}
	for i := 0; i < pd.ResourceProfiles().Len(); i++ {
		rp := pd.ResourceProfiles().At(i)
		for j := 0; j < rp.ScopeProfiles().Len(); j++ {
			sp := rp.ScopeProfiles().At(j)
			for n := 0; n < sp.Profiles().Len(); n++ {
				p := sp.Profiles().At(n)
				if (p.OriginalPayloadFormat() == "") != (p.OriginalPayload().Len() == 0) {
					return fmt.Errorf("original payload format and bytes must both be set or both absent")
				}
				if err := attributes(p.AttributeIndices()); err != nil {
					return err
				}
				for _, v := range []pprofile.ValueType{p.SampleType(), p.PeriodType()} {
					if err := strings("value type", v.TypeStrindex()); err != nil {
						return err
					}
					if err := strings("value unit", v.UnitStrindex()); err != nil {
						return err
					}
				}
				for x := 0; x < p.Samples().Len(); x++ {
					v := p.Samples().At(x)
					if err := index("sample stack", v.StackIndex(), d.StackTable().Len()); err != nil {
						return err
					}
					if err := index("sample link", v.LinkIndex(), d.LinkTable().Len()); err != nil {
						return err
					}
					if err := attributes(v.AttributeIndices()); err != nil {
						return err
					}
					nv, nt := v.Values().Len(), v.TimestampsUnixNano().Len()
					if nv == 0 && nt == 0 {
						return fmt.Errorf("sample requires values or timestamps")
					}
					if nv != 0 && nt != 0 && nv != nt {
						return fmt.Errorf("sample values and timestamps must have matching lengths")
					}
				}
			}
		}
	}
	return nil
}

// DecodeStoredProfile only decodes an explicitly identified, supported encoding.
// Unversioned bytes remain opaque because historical development schemas changed
// wire types; passing them to the current unmarshaler is not a migration.
func DecodeStoredProfile(e ProfileEntry) (pprofile.Profiles, error) {
	if e.ProfileStorageVersion != ProfileStorageVersionCurrent || e.ProfileEncoding != ProfileEncodingOTLP || e.ProfileSchemaVersion == nil || *e.ProfileSchemaVersion != ProfileSchemaVersion {
		return pprofile.Profiles{}, fmt.Errorf("unsupported profile storage encoding or schema version")
	}
	if len(e.ProfileBytes) == 0 {
		return pprofile.Profiles{}, fmt.Errorf("profile payload is empty")
	}
	pd, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(e.ProfileBytes)
	if err != nil {
		return pprofile.Profiles{}, fmt.Errorf("decode profile protobuf: %w", err)
	}
	if pd.ProfileCount() != 1 || pd.ResourceProfiles().Len() != 1 || pd.ResourceProfiles().At(0).ScopeProfiles().Len() != 1 {
		return pprofile.Profiles{}, fmt.Errorf("stored profile payload must contain exactly one resource, scope, and profile")
	}
	if err := ValidateProfiles(pd); err != nil {
		return pprofile.Profiles{}, err
	}
	return pd, nil
}

// ProfileEntryFromProfiles copies before marshaling: current pdata interners
// mutate mutable resources, scopes, attributes and the shared dictionary.
// The full dictionary and context are retained without reindexing references.
func ProfileEntryFromProfiles(pd pprofile.Profiles, ingestedAt time.Time) (ProfileEntry, error) {
	if pd.ProfileCount() != 1 || pd.ResourceProfiles().Len() != 1 || pd.ResourceProfiles().At(0).ScopeProfiles().Len() != 1 {
		return ProfileEntry{}, fmt.Errorf("profile row requires exactly one resource, scope, and profile")
	}
	if err := ValidateProfiles(pd); err != nil {
		return ProfileEntry{}, err
	}
	rp := pd.ResourceProfiles().At(0)
	sp := rp.ScopeProfiles().At(0)
	p := sp.Profiles().At(0)
	d := pd.Dictionary()
	strptr := func(v string) *string { return &v }
	attrMap := make(map[string]interface{})
	for i := 0; i < p.AttributeIndices().Len(); i++ {
		a := d.AttributeTable().At(int(p.AttributeIndices().At(i)))
		if a.KeyStrindex() != 0 {
			attrMap[d.StringTable().At(int(a.KeyStrindex()))] = a.Value().AsRaw()
		}
	}
	attrs, err := marshalProfileSummary(attrMap)
	if err != nil {
		return ProfileEntry{}, err
	}
	resAttrs, err := marshalProfileSummary(rp.Resource().Attributes().AsRaw())
	if err != nil {
		return ProfileEntry{}, err
	}
	scopeAttrs, err := marshalProfileSummary(sp.Scope().Attributes().AsRaw())
	if err != nil {
		return ProfileEntry{}, err
	}
	resourceString := func(key string) *string {
		if v, ok := rp.Resource().Attributes().Get(key); ok {
			return strptr(v.Str())
		}
		return nil
	}
	name := ""
	if v := resourceString("service.name"); v != nil {
		name = *v
	}
	timestamp := ingestedAt
	if p.Time() != 0 && uint64(p.Time()) <= math.MaxInt64 {
		timestamp = time.Unix(0, int64(p.Time())).UTC()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	if timestamp.Before(time.Unix(0, math.MinInt64)) || timestamp.After(time.Unix(0, math.MaxInt64)) {
		return ProfileEntry{}, fmt.Errorf("profile query-index timestamp outside signed nanosecond range")
	}
	duration := int64(0)
	if p.DurationNano() <= math.MaxInt64 {
		duration = int64(p.DurationNano())
	}
	scopeDropped := (*int32)(nil)
	if sp.Scope().DroppedAttributesCount() <= math.MaxInt32 {
		v := int32(sp.Scope().DroppedAttributesCount())
		scopeDropped = &v
	}
	clone := pprofile.NewProfiles()
	pd.CopyTo(clone)
	raw, err := (&pprofile.ProtoMarshaler{}).MarshalProfiles(clone)
	if err != nil {
		return ProfileEntry{}, fmt.Errorf("marshal profile protobuf: %w", err)
	}
	return ProfileEntry{
		TimestampNS: timestamp.UnixNano(), Timestamp: timestamp, ServiceName: name, ServiceNamespace: resourceString("service.namespace"), ServiceInstanceID: resourceString("service.instance.id"),
		ProfileType: d.StringTable().At(int(p.SampleType().TypeStrindex())), DurationNS: duration, SampleType: strptr(d.StringTable().At(int(p.SampleType().TypeStrindex()))), SampleUnit: strptr(d.StringTable().At(int(p.SampleType().UnitStrindex()))),
		ProfileBytes: raw, ProfileStorageVersion: ProfileStorageVersionCurrent, ProfileEncoding: ProfileEncodingOTLP, ProfileSchemaVersion: strptr(ProfileSchemaVersion), OtelProfileID: strptr(p.ProfileID().String()), ProfileTimeUnixNano: strptr(strconv.FormatUint(uint64(p.Time()), 10)), DurationNano: strptr(strconv.FormatUint(p.DurationNano(), 10)),
		Attributes: attrs, ResourceAttributes: resAttrs, ResourceSchemaURL: strptr(rp.SchemaUrl()), ScopeName: strptr(sp.Scope().Name()), ScopeVersion: strptr(sp.Scope().Version()), ScopeAttributes: scopeAttrs, ScopeDroppedAttrCount: scopeDropped, ScopeSchemaURL: strptr(sp.SchemaUrl()), Metadata: []byte("{}"),
	}, nil
}

// ValidateProfileEntry checks the format declaration before any database work.
func ValidateProfileEntry(e ProfileEntry) error {
	if isLegacyProfileEntry(e) {
		return nil
	}
	_, err := DecodeStoredProfile(e)
	return err
}

// OTLP allows non-finite doubles, while PostgreSQL JSONB does not. Use OTLP JSON's
// string spelling in searchable summaries; the original typed wire value remains
// available in profile_bytes/profile_data. Copy recursively without modifying pdata.
func marshalProfileSummary(v interface{}) ([]byte, error) { return json.Marshal(profileJSONSafe(v)) }
func profileJSONSafe(v interface{}) interface{} {
	switch value := v.(type) {
	case float64:
		if math.IsNaN(value) {
			return "NaN"
		}
		if math.IsInf(value, 1) {
			return "Infinity"
		}
		if math.IsInf(value, -1) {
			return "-Infinity"
		}
		return value
	case map[string]interface{}:
		copy := make(map[string]interface{}, len(value))
		for key, item := range value {
			copy[key] = profileJSONSafe(item)
		}
		return copy
	case []interface{}:
		copy := make([]interface{}, len(value))
		for i, item := range value {
			copy[i] = profileJSONSafe(item)
		}
		return copy
	default:
		return value
	}
}

func isLegacyProfileEntry(e ProfileEntry) bool {
	return e.ProfileStorageVersion == 0 && (e.ProfileEncoding == "" || e.ProfileEncoding == ProfileEncodingLegacy) && e.ProfileSchemaVersion == nil
}
