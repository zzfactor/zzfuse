package repository

import (
	"testing"
)

// TestAllAttributesTemplate tests the default template that includes all attributes
func TestAllAttributesTemplate(t *testing.T) {
	template := NewAllAttributesTemplate()

	// Test template metadata
	if template.GetTemplateName() != "all_attributes" {
		t.Errorf("Expected template name 'all_attributes', got '%s'", template.GetTemplateName())
	}

	// Test that all attributes are included
	testCases := []struct {
		key      string
		expected bool
	}{
		{"db.system", true},
		{"http.method", true},
		{"internal.debug", true},
		{"user.id", true},
		{"", true}, // Empty key should also be included
	}

	for _, tc := range testCases {
		result := template.ShouldIncludeSpanAttribute(tc.key)
		if result != tc.expected {
			t.Errorf("For key '%s', expected %v, got %v", tc.key, tc.expected, result)
		}
	}

	// Test signal attributes
	signalKeys := []string{"log.severity", "metric.type", "profile.cpu"}
	for _, key := range signalKeys {
		if !template.ShouldIncludeLogAttribute(key) {
			t.Errorf("Log attribute '%s' should be included", key)
		}
		if !template.ShouldIncludeMetricAttribute(key) {
			t.Errorf("Metric attribute '%s' should be included", key)
		}
		if !template.ShouldIncludeProfileAttribute(key) {
			t.Errorf("Profile attribute '%s' should be included", key)
		}
	}
}

// TestTemplateModeTransformerCreation tests that the template transformer can be created
func TestTemplateModeTransformerCreation(t *testing.T) {
	transformer := NewFullSpanTransformer()

	if transformer == nil {
		t.Fatal("Transformer should not be nil")
	}

	if transformer.GetMode() != "full_span" {
		t.Errorf("Expected mode 'full_span', got '%s'", transformer.GetMode())
	}
}

// TestTransformerFactoryWithTemplateMode tests that the factory can create the template transformer
func TestTransformerFactoryWithTemplateMode(t *testing.T) {
	factory := NewTransformerFactory()

	// Test the template mode
	transformer, err := factory.CreateTransformer("template")
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	if transformer == nil {
		t.Fatal("Transformer should not be nil")
	}

	if transformer.GetMode() != "full_span" {
		t.Errorf("Expected mode 'full_span', got '%s'", transformer.GetMode())
	}

	// Test that it's the correct type
	_, ok := transformer.(*FullSpanTransformer)
	if !ok {
		t.Error("Transformer should be of type FullSpanTransformer")
	}
}

// TestTemplateInterfaceCompliance tests that the template implements the interface correctly
func TestTemplateInterfaceCompliance(t *testing.T) {
	var template AttributeTemplate = NewAllAttributesTemplate()

	// Test that all interface methods work
	if template.GetTemplateName() == "" {
		t.Error("Template name should not be empty")
	}

	if template.GetDescription() == "" {
		t.Error("Template description should not be empty")
	}

	// Test attribute filtering methods
	if !template.ShouldIncludeSpanAttribute("test") {
		t.Error("AllAttributesTemplate should include all span attributes")
	}

	if !template.ShouldIncludeLogAttribute("test") {
		t.Error("AllAttributesTemplate should include all log attributes")
	}

	if !template.ShouldIncludeMetricAttribute("test") {
		t.Error("AllAttributesTemplate should include all metric attributes")
	}

	if !template.ShouldIncludeProfileAttribute("test") {
		t.Error("AllAttributesTemplate should include all profile attributes")
	}
}
