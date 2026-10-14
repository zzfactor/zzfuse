# Template Categorization Examples

This document shows how to use the enhanced template functions for categorization and conditional logic in your embedding templates.

## 🎯 **Your Requested Template**

Here's exactly how to implement your log count range logic:

```go
// Your original request:
// service={{.ServiceName}} operation={{.SpanName}} status={{.Status}}{{if .StatusMessage}} error="{{.StatusMessage}}"{{end}}{{range $key, $value := .Attributes}} attr_{{$key}}="{{$value}}"{{end}}

// Corrected field names and enhanced with categorization:
`service={{.service_name}} operation={{.name}} status={{.status_code}}{{if .status_message}} error="{{.status_message}}"{{end}}{{if .log_count}} log_volume="{{.log_count | categorizeLogCount}}"{{end}}{{range $key, $value := .attributes}} attr_{{$key}}="{{$value}}"{{end}}`
```

## 📊 **Available Categorization Functions**

### 1. **Log Count Categorization**
```go
{{.log_count | categorizeLogCount}}
```
**Ranges:**
- 0-9: `"small"`
- 10-20: `"medium"`
- 21+: `"large"`

### 2. **Duration Categorization**
```go
{{.duration_ms | categorizeDuration}}
```
**Ranges:**
- 0-99ms: `"fast"`
- 100-1000ms: `"normal"`
- 1001ms+: `"slow"`

### 3. **Error Rate Categorization**
```go
{{.error_count | categorizeErrorRate .span_count}}
```
**Ranges:**
- 0-1%: `"low"`
- 1-5%: `"medium"`
- 5%+: `"high"`

### 4. **Span Count Categorization**
```go
{{.span_count | categorizeSpanCount}}
```
**Ranges:**
- 0-4: `"simple"`
- 5-20: `"medium"`
- 21+: `"complex"`

## 🔧 **Template Examples**

### **Basic Trace Template with Categorization**
```go
`service={{.service_name}} operation={{.name}} status={{.status_code}}{{if .status_message}} error="{{.status_message}}"{{end}}{{if .log_count}} log_volume="{{.log_count | categorizeLogCount}}"{{end}}{{if .duration_ms}} performance="{{.duration_ms | categorizeDuration}}"{{end}}{{range $key, $value := .attributes}} attr_{{$key}}="{{$value}}"{{end}}`
```

**Output Examples:**
```
service=user-service operation=login status=OK log_volume="small" performance="fast" attr_user_id="12345" attr_method="POST"
service=payment-service operation=process_payment status=ERROR error="insufficient_funds" log_volume="large" performance="slow" attr_amount="100.00" attr_currency="USD"
```

### **Full Trace Template with Multiple Categorizations**
```go
`Trace {{.trace_id}} from {{.service_name}}: {{.duration_ms | categorizeDuration}} performance ({{.duration_ms}}ms), {{.span_count | categorizeSpanCount}} complexity ({{.span_count}} spans), {{.log_count | categorizeLogCount}} logging ({{.log_count}} logs){{if and .error_count .span_count}}, {{.error_count | categorizeErrorRate .span_count}} error rate{{end}}`
```

**Output Examples:**
```
Trace abc123 from user-service: fast performance (45ms), simple complexity (3 spans), small logging (2 logs)
Trace def456 from payment-service: slow performance (2500ms), complex complexity (25 spans), large logging (35 logs), high error rate
```

### **Conditional Logic with Categorization**
```go
`{{if gt .log_count 10}}
High log volume detected: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{else if gt .log_count 5}}
Moderate log volume: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{else}}
Low log volume: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{end}}

{{if and (eq .status_code "ERROR") (gt .duration_ms 1000)}}
Slow error detected: {{.duration_ms}}ms duration ({{.duration_ms | categorizeDuration}})
{{end}}`
```

## 🗄️ **Database Configuration**

### **Insert Template with Categorization**
```sql
INSERT INTO admin.config_processor_template (name, content, description) VALUES (
    'categorized-traces',
    '{
        "traces": "service={{.service_name}} operation={{.name}} status={{.status_code}}{{if .status_message}} error=\"{{.status_message}}\"{{end}}{{if .log_count}} log_volume=\"{{.log_count | categorizeLogCount}}\"{{end}}{{if .duration_ms}} performance=\"{{.duration_ms | categorizeDuration}}\"{{end}}{{range $key, $value := .attributes}} attr_{{$key}}=\"{{$value}}\"{{end}}",
        "full_trace": "Trace {{.trace_id}} from {{.service_name}}: {{.duration_ms | categorizeDuration}} performance ({{.duration_ms}}ms), {{.span_count | categorizeSpanCount}} complexity ({{.span_count}} spans), {{.log_count | categorizeLogCount}} logging ({{.log_count}} logs){{if and .error_count .span_count}}, {{.error_count | categorizeErrorRate .span_count}} error rate{{end}}"
    }',
    'Template with automatic categorization of log counts, durations, and error rates'
);
```

### **Update Existing Template**
```sql
UPDATE admin.config_processor_template 
SET content = '{
    "traces": "service={{.service_name}} operation={{.name}} status={{.status_code}}{{if .status_message}} error=\"{{.status_message}}\"{{end}}{{if .log_count}} log_volume=\"{{.log_count | categorizeLogCount}}\"{{end}}{{range $key, $value := .attributes}} attr_{{$key}}=\"{{$value}}\"{{end}}"
}'
WHERE name = 'default';
```

## 🔍 **Search Benefits**

### **Semantic Search Examples**
With categorization, you can now search for:

- **Performance issues**: `"slow performance"` or `"fast performance"`
- **Log volume patterns**: `"large logging"` or `"small logging"`
- **Complexity levels**: `"complex complexity"` or `"simple complexity"`
- **Error patterns**: `"high error rate"` or `"low error rate"`

### **Combined Searches**
- `"slow performance AND large logging"` - Find slow operations with high log volume
- `"complex complexity AND high error rate"` - Find complex traces with many errors
- `"fast performance AND small logging"` - Find efficient operations

## ⚙️ **Customizing Ranges**

If you need different categorization ranges, you can modify the functions in `pkg/util/template.go`:

```go
// Example: Custom log count ranges
func CategorizeLogCount(count interface{}) string {
    var num int
    switch v := count.(type) {
    case int:
        num = v
    case int64:
        num = int(v)
    case float64:
        num = int(v)
    default:
        return "unknown"
    }
    
    // Custom ranges
    if num < 5 {
        return "minimal"
    } else if num <= 15 {
        return "moderate"
    } else if num <= 50 {
        return "high"
    } else {
        return "excessive"
    }
}
```

## 🎯 **Best Practices**

1. **Always provide fallbacks**: `{{.log_count | default 0 | categorizeLogCount}}`
2. **Use consistent terminology**: Stick to the predefined categories
3. **Combine with conditional logic**: Only show categorization when relevant
4. **Test your templates**: Verify the output matches your expectations
5. **Document custom ranges**: If you modify the functions, update this documentation

## 🚀 **Next Steps**

1. **Test the template**: Try the categorized template with your data
2. **Customize ranges**: Adjust the categorization thresholds if needed
3. **Add more functions**: Create additional categorization functions for your specific needs
4. **Monitor performance**: Ensure the categorization doesn't impact processing speed 