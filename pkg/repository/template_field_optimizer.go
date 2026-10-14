package repository

import (
	"fmt"

	templatepkg "github.com/medxops/trazr-otel/pkg/repository/template"
)

// TemplateFieldOptimizer combines content and metadata templates to generate optimized field lists
type TemplateFieldOptimizer struct {
	extractor    *TemplateFieldExtractor
	fieldMapping *templatepkg.FieldMapping
	sqlBuilder   *templatepkg.SQLBuilder
}

// NewTemplateFieldOptimizer creates a new template field optimizer
func NewTemplateFieldOptimizer() *TemplateFieldOptimizer {
	fieldMapping := templatepkg.NewFieldMapping()
	return &TemplateFieldOptimizer{
		extractor:    NewTemplateFieldExtractor(),
		fieldMapping: fieldMapping,
		sqlBuilder:   templatepkg.NewSQLBuilder(fieldMapping),
	}
}

// OptimizedFieldList represents the optimized field configuration for database queries
type OptimizedFieldList = templatepkg.OptimizedFieldList

// OptimizeTemplates combines content and metadata templates to create optimized field lists
func (o *TemplateFieldOptimizer) OptimizeTemplates(
	contentTemplates map[string]string,
	metadataTemplates map[string][]string,
) (map[string]*OptimizedFieldList, error) {

	optimizedLists := make(map[string]*OptimizedFieldList)

	// Process each signal type
	for signalType, contentTemplate := range contentTemplates {
		// Step 1: Extract fields from content template
		contentFields, err := o.extractor.ExtractFieldsFromTemplate(contentTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to extract fields from content template for %s: %w", signalType, err)
		}

		// Step 2: Get metadata fields for this signal type
		metadataFields := metadataTemplates[signalType]
		if metadataFields == nil {
			metadataFields = []string{} // Empty slice if no metadata
		}

		// Step 3: Combine and optimize fields
		optimizedList := o.optimizeFieldList(signalType, contentFields, metadataFields)
		optimizedLists[signalType] = optimizedList
	}

	return optimizedLists, nil
}

// optimizeFieldList performs the three-pass optimization process
func (o *TemplateFieldOptimizer) optimizeFieldList(
	signalType string,
	contentFields []string,
	metadataFields []string,
) *OptimizedFieldList {

	// First pass: Extract all field references from both templates
	allFields := make(map[string]bool)

	// Add content template fields
	for _, field := range contentFields {
		allFields[field] = true
	}

	// Add metadata template fields
	for _, field := range metadataFields {
		allFields[field] = true
	}

	// Second pass: Analyze patterns and optimize
	staticFields := make(map[string]bool)
	jsonbFields := make(map[string]map[string]bool)
	fullJSONBFields := make(map[string]bool)

	for field := range allFields {
		if o.fieldMapping.IsJSONBField(field) {
			// Handle JSONB fields
			baseField, specificKey := o.fieldMapping.ParseJSONBField(field)
			if specificKey != "" {
				// Specific field like "attributes.region"
				if jsonbFields[baseField] == nil {
					jsonbFields[baseField] = make(map[string]bool)
				}
				jsonbFields[baseField][specificKey] = true
				// ✅ CRITICAL FIX: Also include the full base field for metadata extraction
				fullJSONBFields[baseField] = true
			} else {
				// Full field like "attributes"
				fullJSONBFields[baseField] = true
			}
		} else {
			// Static field
			staticFields[field] = true
		}
	}

	// Third pass: Generate optimized field list
	optimizedList := &OptimizedFieldList{
		SignalType:      signalType,
		StaticFields:    o.fieldMapping.MapKeysToSlice(staticFields),
		JSONBFields:     o.fieldMapping.ConvertJSONBMap(jsonbFields),
		FullJSONBFields: o.fieldMapping.MapKeysToSlice(fullJSONBFields),
	}

	// Handle conflicts: if we have both full JSONB field and specific keys, prefer full field
	for baseField := range fullJSONBFields {
		delete(optimizedList.JSONBFields, baseField)
	}

	return optimizedList
}

// GenerateSQLSelect generates the SQL SELECT clause for the optimized field list
func (o *TemplateFieldOptimizer) GenerateSQLSelect(optimizedList *OptimizedFieldList) string {
	return o.sqlBuilder.BuildSelectClause(optimizedList)
}

// GetRequiredFields returns all required fields for database query
func (o *TemplateFieldOptimizer) GetRequiredFields(optimizedList *OptimizedFieldList) []string {
	return o.fieldMapping.GetRequiredFields(optimizedList)
}
