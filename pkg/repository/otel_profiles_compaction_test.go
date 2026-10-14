package repository

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

// Resolve graph identities into values rather than comparing wire indexes. This
// independently describes the complete reachable graph that a consumer sees.
func profileGraphSnapshot(pd pprofile.Profiles) interface{} {
	d := pd.Dictionary()
	rp := pd.ResourceProfiles().At(0)
	sp := rp.ScopeProfiles().At(0)
	p := sp.Profiles().At(0)
	stringAt := func(i int32) string { return d.StringTable().At(int(i)) }
	attrs := func(indices pcommon.Int32Slice) []interface{} {
		result := []interface{}{}
		for i := 0; i < indices.Len(); i++ {
			a := d.AttributeTable().At(int(indices.At(i)))
			result = append(result, map[string]interface{}{"key": stringAt(a.KeyStrindex()), "unit": stringAt(a.UnitStrindex()), "value": a.Value().AsRaw(), "value_type": a.Value().Type()})
		}
		return result
	}
	function := func(n int32) interface{} {
		f := d.FunctionTable().At(int(n))
		return []interface{}{stringAt(f.NameStrindex()), stringAt(f.SystemNameStrindex()), stringAt(f.FilenameStrindex()), f.StartLine()}
	}
	mapping := func(n int32) interface{} {
		m := d.MappingTable().At(int(n))
		return []interface{}{m.MemoryStart(), m.MemoryLimit(), m.FileOffset(), stringAt(m.FilenameStrindex()), attrs(m.AttributeIndices())}
	}
	location := func(n int32) interface{} {
		l := d.LocationTable().At(int(n))
		lines := []interface{}{}
		for i := 0; i < l.Lines().Len(); i++ {
			line := l.Lines().At(i)
			lines = append(lines, []interface{}{function(line.FunctionIndex()), line.Line(), line.Column()})
		}
		return []interface{}{mapping(l.MappingIndex()), l.Address(), lines, attrs(l.AttributeIndices())}
	}
	samples := []interface{}{}
	for i := 0; i < p.Samples().Len(); i++ {
		s := p.Samples().At(i)
		stack := d.StackTable().At(int(s.StackIndex()))
		frames := []interface{}{}
		for j := 0; j < stack.LocationIndices().Len(); j++ {
			frames = append(frames, location(stack.LocationIndices().At(j)))
		}
		link := d.LinkTable().At(int(s.LinkIndex()))
		samples = append(samples, []interface{}{frames, link.TraceID(), link.SpanID(), attrs(s.AttributeIndices()), s.Values().AsRaw(), s.TimestampsUnixNano().AsRaw()})
	}
	return map[string]interface{}{
		"resource": rp.Resource().Attributes().AsRaw(), "resource_dropped": rp.Resource().DroppedAttributesCount(), "resource_schema": rp.SchemaUrl(),
		"scope": sp.Scope().Attributes().AsRaw(), "scope_name": sp.Scope().Name(), "scope_version": sp.Scope().Version(), "scope_dropped": sp.Scope().DroppedAttributesCount(), "scope_schema": sp.SchemaUrl(),
		"profile_id": p.ProfileID(), "time": uint64(p.Time()), "duration": p.DurationNano(), "dropped": p.DroppedAttributesCount(), "original_format": p.OriginalPayloadFormat(), "original": p.OriginalPayload().AsRaw(),
		"sample_type": []string{stringAt(p.SampleType().TypeStrindex()), stringAt(p.SampleType().UnitStrindex())}, "period_type": []string{stringAt(p.PeriodType().TypeStrindex()), stringAt(p.PeriodType().UnitStrindex())}, "period": p.Period(), "attrs": attrs(p.AttributeIndices()), "samples": samples,
	}
}

func singleProfileWithContext(pd pprofile.Profiles, resource, scope, profile int) pprofile.Profiles {
	row := pprofile.NewProfiles()
	srcR := pd.ResourceProfiles().At(resource)
	srcS := srcR.ScopeProfiles().At(scope)
	r := row.ResourceProfiles().AppendEmpty()
	srcR.Resource().CopyTo(r.Resource())
	r.SetSchemaUrl(srcR.SchemaUrl())
	s := r.ScopeProfiles().AppendEmpty()
	srcS.Scope().CopyTo(s.Scope())
	s.SetSchemaUrl(srcS.SchemaUrl())
	srcS.Profiles().At(profile).CopyTo(s.Profiles().AppendEmpty())
	pd.Dictionary().CopyTo(row.Dictionary())
	return row
}

func TestProfileCompactionLosslessAndPrivate(t *testing.T) {
	pd := profileFixture(t, "current-profiles.pb")
	// Empty foreign context is still interned into the source dictionary, while
	// valid orphan entries are never referenced by any profile graph.
	foreign := pd.ResourceProfiles().AppendEmpty()
	foreign.Resource().Attributes().PutEmptyMap("foreign.secret.key").PutEmptySlice("list").AppendEmpty().SetStr("foreign-empty-secret")
	foreign.ScopeProfiles().AppendEmpty().Scope().Attributes().PutStr("foreign.scope.secret", "foreign-scope-value")
	d := pd.Dictionary()
	orphanIndex := int32(d.StringTable().Len())
	d.StringTable().Append("orphan-secret")
	a := d.AttributeTable().AppendEmpty()
	a.SetKeyStrindex(orphanIndex)
	a.Value().SetStr("orphan-attribute-value")
	a.SetUnitStrindex(orphanIndex)
	f := d.FunctionTable().AppendEmpty()
	f.SetNameStrindex(orphanIndex)
	m := d.MappingTable().AppendEmpty()
	m.SetFilenameStrindex(orphanIndex)
	m.AttributeIndices().FromRaw([]int32{int32(d.AttributeTable().Len() - 1)})
	l := d.LocationTable().AppendEmpty()
	l.SetMappingIndex(int32(d.MappingTable().Len() - 1))
	l.Lines().AppendEmpty().SetFunctionIndex(int32(d.FunctionTable().Len() - 1))
	d.StackTable().AppendEmpty().LocationIndices().FromRaw([]int32{int32(d.LocationTable().Len() - 1)})
	d.LinkTable().AppendEmpty().SetTraceID(pcommon.TraceID{99})
	// Add nested typed strings/integers/bytes that must be re-interned locally.
	pd.ResourceProfiles().At(0).Resource().Attributes().PutEmptyMap("nested.new").PutEmptySlice("items").AppendEmpty().SetStr("tenant-local-string")
	pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Scope().Attributes().PutInt("large.integer", 9007199254740993)
	snapshot := pprofile.NewProfiles()
	pd.CopyTo(snapshot)
	rows, err := CompactProfiles(pd)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatal("profiles discarded or empty resource manufactured rows")
	}
	expectedRow := 0
	for i := 0; i < pd.ResourceProfiles().Len(); i++ {
		rp := pd.ResourceProfiles().At(i)
		for j := 0; j < rp.ScopeProfiles().Len(); j++ {
			sp := rp.ScopeProfiles().At(j)
			for k := 0; k < sp.Profiles().Len(); k++ {
				row := rows[expectedRow]
				if row.ResourceIndex != i {
					t.Fatal("routing resource index changed")
				}
				expected := profileGraphSnapshot(singleProfileWithContext(snapshot, i, j, k))
				actual := profileGraphSnapshot(row.Data)
				if !reflect.DeepEqual(expected, actual) {
					t.Fatalf("reachable semantics changed: expected=%+v actual=%+v", expected, actual)
				}
				entry, err := ProfileEntryFromProfiles(row.Data, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				stored, err := DecodeStoredProfile(entry)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(expected, profileGraphSnapshot(stored)) {
					t.Fatal("serialized compacted row changed graph/context")
				}
				raw, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(stored)
				if err != nil {
					t.Fatal(err)
				}
				for _, secret := range []string{"foreign-empty-secret", "foreign.scope.secret", "foreign-scope-value", "orphan-secret", "orphan-attribute-value"} {
					if bytes.Contains(raw, []byte(secret)) {
						t.Fatalf("unreferenced foreign data leaked: %s", secret)
					}
				}
				expectedRow++
			}
		}
	}
	// Serialize copies to compare full input, because pdata's marshaler mutates it.
	after := pprofile.NewProfiles()
	pd.CopyTo(after)
	beforeRaw, _ := (&pprofile.JSONMarshaler{}).MarshalProfiles(snapshot)
	afterRaw, _ := (&pprofile.JSONMarshaler{}).MarshalProfiles(after)
	if !bytes.Equal(beforeRaw, afterRaw) {
		t.Fatal("compaction changed caller data")
	}
	if !json.Valid(beforeRaw) {
		t.Fatal("test snapshot invalid")
	}
}

func TestProfileCompactionReindexesEveryTable(t *testing.T) {
	pd := profileFixture(t, "current-profile-row0.pb")
	src := pprofile.NewProfilesDictionary()
	pd.Dictionary().CopyTo(src)
	dst := pd.Dictionary()
	shift := func(n int32) int32 {
		if n == 0 {
			return 0
		}
		return n + 1
	}
	// Interleave an orphan before every real entry, forcing each table reference
	// to shift back on compaction rather than passing because indexes coincide.
	dst.StringTable().FromRaw(nil)
	dst.StringTable().Append("")
	dst.StringTable().Append("interleaved-orphan")
	for i := 1; i < src.StringTable().Len(); i++ {
		dst.StringTable().Append(src.StringTable().At(i))
	}
	rewrite := func(v pcommon.Int32Slice) {
		for i := 0; i < v.Len(); i++ {
			v.SetAt(i, shift(v.At(i)))
		}
	}
	dst.AttributeTable().RemoveIf(func(pprofile.KeyValueAndUnit) bool { return true })
	dst.AttributeTable().AppendEmpty()
	dst.AttributeTable().AppendEmpty().SetKeyStrindex(1)
	for i := 1; i < src.AttributeTable().Len(); i++ {
		v := dst.AttributeTable().AppendEmpty()
		src.AttributeTable().At(i).CopyTo(v)
		v.SetKeyStrindex(shift(v.KeyStrindex()))
		v.SetUnitStrindex(shift(v.UnitStrindex()))
	}
	dst.FunctionTable().RemoveIf(func(pprofile.Function) bool { return true })
	dst.FunctionTable().AppendEmpty()
	dst.FunctionTable().AppendEmpty().SetNameStrindex(1)
	for i := 1; i < src.FunctionTable().Len(); i++ {
		v := dst.FunctionTable().AppendEmpty()
		src.FunctionTable().At(i).CopyTo(v)
		v.SetNameStrindex(shift(v.NameStrindex()))
		v.SetSystemNameStrindex(shift(v.SystemNameStrindex()))
		v.SetFilenameStrindex(shift(v.FilenameStrindex()))
	}
	dst.MappingTable().RemoveIf(func(pprofile.Mapping) bool { return true })
	dst.MappingTable().AppendEmpty()
	dst.MappingTable().AppendEmpty().SetFilenameStrindex(1)
	for i := 1; i < src.MappingTable().Len(); i++ {
		v := dst.MappingTable().AppendEmpty()
		src.MappingTable().At(i).CopyTo(v)
		v.SetFilenameStrindex(shift(v.FilenameStrindex()))
		rewrite(v.AttributeIndices())
	}
	dst.LocationTable().RemoveIf(func(pprofile.Location) bool { return true })
	dst.LocationTable().AppendEmpty()
	dst.LocationTable().AppendEmpty().SetAddress(99)
	for i := 1; i < src.LocationTable().Len(); i++ {
		v := dst.LocationTable().AppendEmpty()
		src.LocationTable().At(i).CopyTo(v)
		v.SetMappingIndex(shift(v.MappingIndex()))
		rewrite(v.AttributeIndices())
		for j := 0; j < v.Lines().Len(); j++ {
			l := v.Lines().At(j)
			l.SetFunctionIndex(shift(l.FunctionIndex()))
		}
	}
	dst.StackTable().RemoveIf(func(pprofile.Stack) bool { return true })
	dst.StackTable().AppendEmpty()
	dst.StackTable().AppendEmpty().LocationIndices().FromRaw([]int32{1})
	for i := 1; i < src.StackTable().Len(); i++ {
		v := dst.StackTable().AppendEmpty()
		src.StackTable().At(i).CopyTo(v)
		rewrite(v.LocationIndices())
	}
	dst.LinkTable().RemoveIf(func(pprofile.Link) bool { return true })
	dst.LinkTable().AppendEmpty()
	dst.LinkTable().AppendEmpty().SetTraceID(pcommon.TraceID{99})
	for i := 1; i < src.LinkTable().Len(); i++ {
		src.LinkTable().At(i).CopyTo(dst.LinkTable().AppendEmpty())
	}
	p := pd.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
	rewrite(p.AttributeIndices())
	for _, v := range []pprofile.ValueType{p.SampleType(), p.PeriodType()} {
		v.SetTypeStrindex(shift(v.TypeStrindex()))
		v.SetUnitStrindex(shift(v.UnitStrindex()))
	}
	for i := 0; i < p.Samples().Len(); i++ {
		s := p.Samples().At(i)
		s.SetStackIndex(shift(s.StackIndex()))
		s.SetLinkIndex(shift(s.LinkIndex()))
		rewrite(s.AttributeIndices())
	}
	expected := profileGraphSnapshot(pd)
	rows, err := CompactProfiles(pd)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !reflect.DeepEqual(expected, profileGraphSnapshot(rows[0].Data)) {
		t.Fatal("table reindexing changed graph")
	}
	compact := rows[0].Data.Dictionary()
	if compact.LinkTable().Len() != 2 || compact.StackTable().Len() != 2 || compact.LocationTable().Len() != 3 || compact.FunctionTable().Len() != 2 || compact.MappingTable().Len() != 2 || compact.AttributeTable().Len() != 3 {
		t.Fatal("unreferenced table entries retained")
	}
	if p.Samples().At(0).StackIndex() != 2 || rows[0].Data.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0).Samples().At(0).StackIndex() != 1 {
		t.Fatal("test did not force index remapping or source was mutated")
	}
}
