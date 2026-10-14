-- Fix the template metadata for spans in the database
-- This script updates the template metadata to use the correct fields instead of content template fields

UPDATE otel.config_processor_template 
SET template_metadata = jsonb_set(
    template_metadata, 
    '{spans}', 
    '["name", "service_name", "status_code", "duration", "trace_id", "span_id", "attributes.region", "resource_attributes.trazr_random_seed"]'::jsonb
)
WHERE id = 1;

-- Verify the update
SELECT id, name, template_metadata->'spans' as spans_metadata 
FROM otel.config_processor_template 
WHERE id = 1; 