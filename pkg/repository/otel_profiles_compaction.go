package repository

import (
	"fmt"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

// IsolatedProfile contains one profile with its own reachable dictionary/context.
// ResourceIndex identifies the original resource for schema routing.
type IsolatedProfile struct {
	ResourceIndex int
	Data          pprofile.Profiles
}

// CompactProfiles builds independent rows without mutating the caller. It drops
// unreferenced dictionary entries and foreign resource/scope context, and remaps
// every surviving graph reference. Typed nested attribute strings are resolved
// before copying and are re-interned by pdata only into each destination row.
func CompactProfiles(pd pprofile.Profiles) ([]IsolatedProfile, error) {
	if pd.ProfileCount() == 0 {
		return nil, nil
	}
	// A preceding pdata marshaler may have left inline attributes as string refs.
	// Normalize a copy through the current wire codec, never through a legacy codec.
	clone := pprofile.NewProfiles()
	pd.CopyTo(clone)
	raw, err := (&pprofile.ProtoMarshaler{}).MarshalProfiles(clone)
	if err != nil {
		return nil, fmt.Errorf("normalize profile strings: %w", err)
	}
	source, err := (&pprofile.ProtoUnmarshaler{}).UnmarshalProfiles(raw)
	if err != nil {
		return nil, fmt.Errorf("resolve profile strings: %w", err)
	}
	if err := ValidateProfiles(source); err != nil {
		return nil, err
	}
	rows := make([]IsolatedProfile, 0, source.ProfileCount())
	for i := 0; i < source.ResourceProfiles().Len(); i++ {
		rp := source.ResourceProfiles().At(i)
		for j := 0; j < rp.ScopeProfiles().Len(); j++ {
			sp := rp.ScopeProfiles().At(j)
			for k := 0; k < sp.Profiles().Len(); k++ {
				row := compactValidatedProfile(source.Dictionary(), rp, sp, sp.Profiles().At(k))
				if err := ValidateProfiles(row); err != nil {
					return nil, fmt.Errorf("validate compacted profile: %w", err)
				}
				rows = append(rows, IsolatedProfile{ResourceIndex: i, Data: row})
			}
		}
	}
	return rows, nil
}

type profileIndexSet struct {
	needed []bool
	remap  []int32
}

func newProfileIndexSet(length int) profileIndexSet {
	s := profileIndexSet{needed: make([]bool, length), remap: make([]int32, length)}
	s.needed[0] = true
	return s
}
func (s profileIndexSet) mark(index int32) bool {
	if s.needed[index] {
		return false
	}
	s.needed[index] = true
	return true
}
func (s profileIndexSet) assign() {
	next := int32(0)
	for i, needed := range s.needed {
		if needed {
			s.remap[i] = next
			next++
		}
	}
}
func (s profileIndexSet) rewrite(values pcommon.Int32Slice) {
	for i := 0; i < values.Len(); i++ {
		values.SetAt(i, s.remap[values.At(i)])
	}
}

func compactValidatedProfile(src pprofile.ProfilesDictionary, rp pprofile.ResourceProfiles, sp pprofile.ScopeProfiles, p pprofile.Profile) pprofile.Profiles {
	strings := newProfileIndexSet(src.StringTable().Len())
	attrs := newProfileIndexSet(src.AttributeTable().Len())
	mappings := newProfileIndexSet(src.MappingTable().Len())
	functions := newProfileIndexSet(src.FunctionTable().Len())
	locations := newProfileIndexSet(src.LocationTable().Len())
	stacks := newProfileIndexSet(src.StackTable().Len())
	links := newProfileIndexSet(src.LinkTable().Len())
	markAttrs := func(indices pcommon.Int32Slice) {
		for i := 0; i < indices.Len(); i++ {
			n := indices.At(i)
			if attrs.mark(n) {
				a := src.AttributeTable().At(int(n))
				strings.mark(a.KeyStrindex())
				strings.mark(a.UnitStrindex())
			}
		}
	}
	markMapping := func(n int32) {
		if mappings.mark(n) {
			m := src.MappingTable().At(int(n))
			strings.mark(m.FilenameStrindex())
			markAttrs(m.AttributeIndices())
		}
	}
	markFunction := func(n int32) {
		if functions.mark(n) {
			f := src.FunctionTable().At(int(n))
			strings.mark(f.NameStrindex())
			strings.mark(f.SystemNameStrindex())
			strings.mark(f.FilenameStrindex())
		}
	}
	markLocation := func(n int32) {
		if locations.mark(n) {
			l := src.LocationTable().At(int(n))
			markMapping(l.MappingIndex())
			markAttrs(l.AttributeIndices())
			for i := 0; i < l.Lines().Len(); i++ {
				markFunction(l.Lines().At(i).FunctionIndex())
			}
		}
	}
	markStack := func(n int32) {
		if stacks.mark(n) {
			v := src.StackTable().At(int(n)).LocationIndices()
			for i := 0; i < v.Len(); i++ {
				markLocation(v.At(i))
			}
		}
	}
	markAttrs(p.AttributeIndices())
	for _, v := range []pprofile.ValueType{p.SampleType(), p.PeriodType()} {
		strings.mark(v.TypeStrindex())
		strings.mark(v.UnitStrindex())
	}
	for i := 0; i < p.Samples().Len(); i++ {
		sample := p.Samples().At(i)
		markStack(sample.StackIndex())
		links.mark(sample.LinkIndex())
		markAttrs(sample.AttributeIndices())
	}
	for _, set := range []profileIndexSet{strings, attrs, mappings, functions, locations, stacks, links} {
		set.assign()
	}
	row := pprofile.NewProfiles()
	dst := row.Dictionary()
	for i, needed := range strings.needed {
		if needed {
			dst.StringTable().Append(src.StringTable().At(i))
		}
	}
	for i, needed := range attrs.needed {
		if needed {
			v := dst.AttributeTable().AppendEmpty()
			src.AttributeTable().At(i).CopyTo(v)
			v.SetKeyStrindex(strings.remap[v.KeyStrindex()])
			v.SetUnitStrindex(strings.remap[v.UnitStrindex()])
		}
	}
	for i, needed := range mappings.needed {
		if needed {
			v := dst.MappingTable().AppendEmpty()
			src.MappingTable().At(i).CopyTo(v)
			v.SetFilenameStrindex(strings.remap[v.FilenameStrindex()])
			attrs.rewrite(v.AttributeIndices())
		}
	}
	for i, needed := range functions.needed {
		if needed {
			v := dst.FunctionTable().AppendEmpty()
			src.FunctionTable().At(i).CopyTo(v)
			v.SetNameStrindex(strings.remap[v.NameStrindex()])
			v.SetSystemNameStrindex(strings.remap[v.SystemNameStrindex()])
			v.SetFilenameStrindex(strings.remap[v.FilenameStrindex()])
		}
	}
	for i, needed := range locations.needed {
		if needed {
			v := dst.LocationTable().AppendEmpty()
			src.LocationTable().At(i).CopyTo(v)
			v.SetMappingIndex(mappings.remap[v.MappingIndex()])
			attrs.rewrite(v.AttributeIndices())
			for j := 0; j < v.Lines().Len(); j++ {
				line := v.Lines().At(j)
				line.SetFunctionIndex(functions.remap[line.FunctionIndex()])
			}
		}
	}
	for i, needed := range stacks.needed {
		if needed {
			v := dst.StackTable().AppendEmpty()
			src.StackTable().At(i).CopyTo(v)
			locations.rewrite(v.LocationIndices())
		}
	}
	for i, needed := range links.needed {
		if needed {
			src.LinkTable().At(i).CopyTo(dst.LinkTable().AppendEmpty())
		}
	}
	dr := row.ResourceProfiles().AppendEmpty()
	rp.Resource().CopyTo(dr.Resource())
	dr.SetSchemaUrl(rp.SchemaUrl())
	ds := dr.ScopeProfiles().AppendEmpty()
	sp.Scope().CopyTo(ds.Scope())
	ds.SetSchemaUrl(sp.SchemaUrl())
	dp := ds.Profiles().AppendEmpty()
	p.CopyTo(dp)
	attrs.rewrite(dp.AttributeIndices())
	for _, v := range []pprofile.ValueType{dp.SampleType(), dp.PeriodType()} {
		v.SetTypeStrindex(strings.remap[v.TypeStrindex()])
		v.SetUnitStrindex(strings.remap[v.UnitStrindex()])
	}
	for i := 0; i < dp.Samples().Len(); i++ {
		s := dp.Samples().At(i)
		s.SetStackIndex(stacks.remap[s.StackIndex()])
		s.SetLinkIndex(links.remap[s.LinkIndex()])
		attrs.rewrite(s.AttributeIndices())
	}
	return row
}
