//go:build ignore

// Reproduce from pkg/repository: go run -mod=readonly testdata/generate-profiles.go
package main

import (
	"bytes"
	"fmt"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"math"
	"os"
)

func main() {
	pd := pprofile.NewProfiles()
	d := pd.Dictionary()
	d.StringTable().FromRaw([]string{"", "cpu", "nanoseconds", "profile.owner", "thread.id", "count", "doWork", "worker.go", "libworker.so"})
	d.AttributeTable().AppendEmpty()
	a := d.AttributeTable().AppendEmpty()
	a.SetKeyStrindex(3)
	a.Value().SetStr("team-core")
	a.SetUnitStrindex(0)
	a = d.AttributeTable().AppendEmpty()
	a.SetKeyStrindex(4)
	a.Value().SetInt(42)
	a.SetUnitStrindex(5)
	d.FunctionTable().AppendEmpty()
	f := d.FunctionTable().AppendEmpty()
	f.SetNameStrindex(6)
	f.SetSystemNameStrindex(6)
	f.SetFilenameStrindex(7)
	f.SetStartLine(10)
	d.MappingTable().AppendEmpty()
	m := d.MappingTable().AppendEmpty()
	m.SetFilenameStrindex(8)
	m.SetMemoryStart(4096)
	m.SetMemoryLimit(8192)
	m.SetFileOffset(128)
	m.AttributeIndices().FromRaw([]int32{2})
	d.LocationTable().AppendEmpty()
	for i := 0; i < 2; i++ {
		l := d.LocationTable().AppendEmpty()
		l.SetMappingIndex(1)
		l.SetAddress(uint64(4096 + i*8))
		l.AttributeIndices().FromRaw([]int32{2})
		line := l.Lines().AppendEmpty()
		line.SetFunctionIndex(1)
		line.SetLine(int64(10 + i))
		line.SetColumn(2)
	}
	d.StackTable().AppendEmpty()
	d.StackTable().AppendEmpty().LocationIndices().FromRaw([]int32{1, 2})
	d.LinkTable().AppendEmpty()
	link := d.LinkTable().AppendEmpty()
	link.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	link.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8})
	rp := pd.ResourceProfiles().AppendEmpty()
	rp.SetSchemaUrl("https://opentelemetry.io/schemas/1.34.0")
	rp.Resource().Attributes().PutStr("service.name", "profile-review")
	rp.Resource().Attributes().PutStr("service.namespace", "modern-team")
	rp.Resource().Attributes().PutStr("service.instance.id", "modern-instance")
	rp.Resource().Attributes().PutEmptyMap("resource.nested").PutStr("zone", "east")
	rp.Resource().SetDroppedAttributesCount(3)
	sp := rp.ScopeProfiles().AppendEmpty()
	sp.SetSchemaUrl("https://opentelemetry.io/schemas/1.33.0")
	sp.Scope().SetName("modern.profiler")
	sp.Scope().SetVersion("0.163-fixture")
	sp.Scope().Attributes().PutStr("scope.note", "scope-attribute-string")
	sp.Scope().Attributes().PutBool("scope.enabled", true)
	sp.Scope().SetDroppedAttributesCount(2)
	for i := 0; i < 2; i++ {
		p := sp.Profiles().AppendEmpty()
		p.SetProfileID(pprofile.ProfileID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, byte(16 + i)})
		p.SetTime(pcommon.Timestamp(1777777777123456789 + uint64(i)))
		p.SetDurationNano(9000000000)
		p.SetDroppedAttributesCount(4)
		p.SampleType().SetTypeStrindex(1)
		p.SampleType().SetUnitStrindex(2)
		p.PeriodType().SetTypeStrindex(1)
		p.PeriodType().SetUnitStrindex(2)
		p.SetPeriod(37)
		p.AttributeIndices().FromRaw([]int32{1})
		p.SetOriginalPayloadFormat("jfr")
		p.OriginalPayload().FromRaw([]byte{0, 1, 127, 128, 255, 10, 13})
		s := p.Samples().AppendEmpty()
		s.SetStackIndex(1)
		s.SetLinkIndex(1)
		s.AttributeIndices().FromRaw([]int32{2})
		s.Values().FromRaw([]int64{7, 0, -5, 9007199254740993})
		s.TimestampsUnixNano().FromRaw([]uint64{1777777777123456789, 1777777777123456790, 1777777777123456791, 1777777777123456792})
	}
	// A separate resource/scope exercises unsigned source values beyond bigint.
	er := pd.ResourceProfiles().AppendEmpty()
	rp.Resource().CopyTo(er.Resource())
	er.SetSchemaUrl("https://opentelemetry.io/schemas/1.35.0")
	er.Resource().Attributes().PutStr("resource.variant", "extreme")
	er.Resource().SetDroppedAttributesCount(math.MaxUint32)
	es := er.ScopeProfiles().AppendEmpty()
	sp.Scope().CopyTo(es.Scope())
	es.SetSchemaUrl("https://opentelemetry.io/schemas/1.35.0")
	es.Scope().SetDroppedAttributesCount(math.MaxUint32)
	ep := es.Profiles().AppendEmpty()
	sp.Profiles().At(0).CopyTo(ep)
	ep.SetProfileID(pprofile.ProfileID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 99})
	ep.SetTime(pcommon.Timestamp(math.MaxUint64))
	ep.SetDurationNano(math.MaxUint64)
	ep.SetDroppedAttributesCount(math.MaxUint32)
	originalDictLen := d.StringTable().Len()
	allCopy := pprofile.NewProfiles()
	pd.CopyTo(allCopy)
	allPB, err := (&pprofile.ProtoMarshaler{}).MarshalProfiles(allCopy)
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("testdata/current-profiles.pb", allPB, 0600); err != nil {
		panic(err)
	}
	allJSON, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(allCopy)
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("testdata/current-profiles.json", allJSON, 0644); err != nil {
		panic(err)
	}
	for i := 0; i < 2; i++ {
		row := pprofile.NewProfiles()
		dr := row.ResourceProfiles().AppendEmpty()
		rp.Resource().CopyTo(dr.Resource())
		dr.SetSchemaUrl(rp.SchemaUrl())
		ds := dr.ScopeProfiles().AppendEmpty()
		sp.Scope().CopyTo(ds.Scope())
		ds.SetSchemaUrl(sp.SchemaUrl())
		sp.Profiles().At(i).CopyTo(ds.Profiles().AppendEmpty())
		d.CopyTo(row.Dictionary())
		wire, err := (&pprofile.ProtoMarshaler{}).MarshalProfiles(row)
		if err != nil {
			panic(err)
		}
		decoded, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(wire)
		if err != nil {
			panic(err)
		}
		p := decoded.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
		if decoded.ProfileCount() != 1 || p.ProfileID() != sp.Profiles().At(i).ProfileID() || p.Time() != sp.Profiles().At(i).Time() || p.DurationNano() != 9000000000 || p.Samples().At(0).Values().At(3) != 9007199254740993 || !bytes.Equal(p.OriginalPayload().AsRaw(), []byte{0, 1, 127, 128, 255, 10, 13}) {
			panic("roundtrip lost profile metadata")
		}
		if val, ok := decoded.ResourceProfiles().At(0).Resource().Attributes().Get("service.name"); !ok || val.Str() != "profile-review" {
			panic("resource attr lost")
		}
		if val, ok := decoded.ResourceProfiles().At(0).ScopeProfiles().At(0).Scope().Attributes().Get("scope.note"); !ok || val.Str() != "scope-attribute-string" {
			panic("scope attr lost")
		}
		view, err := (&pprofile.JSONMarshaler{}).MarshalProfiles(decoded)
		if err != nil {
			panic(err)
		}
		os.WriteFile(fmt.Sprintf("testdata/current-profile-row%d.pb", i), wire, 0600)
		os.WriteFile(fmt.Sprintf("testdata/current-profile-row%d.json", i), view, 0600)
	}
	if d.StringTable().Len() != originalDictLen {
		panic("source dictionary mutated")
	}
	if val, ok := rp.Resource().Attributes().Get("service.name"); !ok || val.Str() != "profile-review" {
		panic("source resource mutated")
	}
	fmt.Println("PASS: 0.163 fixtures, row isolation, full dictionary, resource/scope attrs and schemas, distinct16byteIDs, exact nanoseconds, multi-value samples including >2^53, original binarypayload, input mutation guard")
}
