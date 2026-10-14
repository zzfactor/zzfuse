package util

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestFilterMetadata(t *testing.T) {
	meta := map[string]string{
		"resource_service.name": "svc1",
		"resource_env":          "prod",
		"attribute_user_id":     "42",
		"attribute_debug":       "true",
	}
	tests := []struct {
		name    string
		include []string
		exclude []string
		want    map[string]string
	}{
		{
			name:    "no include/exclude (all keys)",
			include: nil,
			exclude: nil,
			want:    meta,
		},
		{
			name:    "include only resource_",
			include: []string{"resource_"},
			exclude: nil,
			want: map[string]string{
				"resource_service.name": "svc1",
				"resource_env":          "prod",
			},
		},
		{
			name:    "exclude only attribute_",
			include: nil,
			exclude: []string{"attribute_"},
			want: map[string]string{
				"resource_service.name": "svc1",
				"resource_env":          "prod",
			},
		},
		{
			name:    "include resource_ and attribute_user, exclude attribute_debug",
			include: []string{"resource_", "attribute_user_id"},
			exclude: []string{"attribute_debug"},
			want: map[string]string{
				"resource_service.name": "svc1",
				"resource_env":          "prod",
				"attribute_user_id":     "42",
			},
		},
		{
			name:    "prefix matching include/exclude",
			include: []string{"attribute_"},
			exclude: []string{"attribute_debug"},
			want: map[string]string{
				"attribute_user_id": "42",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterMetadata(meta, tt.include, tt.exclude)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterMetadata() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractAndFlattenResourceAttributes(t *testing.T) {
	res := pcommon.NewResource()
	res.Attributes().PutStr("service.name", "svc1")
	res.Attributes().PutInt("instance", 7)
	got := ExtractAndFlattenResourceAttributes(res)
	want := map[string]interface{}{
		"resource_service.name": "svc1",
		"resource_instance":     int64(7),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractAndFlattenResourceAttributes() = %v, want %v", got, want)
	}
}

func TestExtractAndFlattenAttributes(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("foo", "bar")
	attrs.PutBool("debug", true)
	got := ExtractAndFlattenAttributes(attrs, "attribute_")
	want := map[string]interface{}{
		"attribute_foo":   "bar",
		"attribute_debug": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractAndFlattenAttributes() = %v, want %v", got, want)
	}
}
