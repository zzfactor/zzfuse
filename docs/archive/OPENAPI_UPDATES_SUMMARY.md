# OpenAPI Specification Updates Summary

## Overview
This document summarizes all the updates made to the OpenAPI specification files to reflect the new validation rules and constraints implemented in the trazrextension package.

## Key Updates Made

### 1. **Enhanced Parameter Validation** (`components/parameters.yaml`)

#### Updated Limit Parameter
- **Before**: Basic integer with default 100
- **After**: Enhanced with validation constraints
  ```yaml
  Limit:
    in: query
    name: limit
    required: false
    schema:
      type: integer
      minimum: 1
      maximum: 1000
      default: 100
    description: Maximum number of results to return. Must be between 1 and 1000. Values exceeding 1000 will be automatically normalized to 1000.
  ```

#### New Offset Parameter
- **Added**: Comprehensive offset validation
  ```yaml
  Offset:
    in: query
    name: offset
    required: false
    schema:
      type: integer
      minimum: 0
      maximum: 100000
      default: 0
    description: Zero-based offset for pagination. Must be between 0 and 100000.
  ```

#### Enhanced Time Parameters
- **Start/End**: Added RFC3339 pattern validation and constraints
  ```yaml
  Start:
    schema:
      type: string
      format: date-time
      pattern: '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z?$'
    description: ISO 8601 timestamp (RFC3339 format) - include entries >= this time. Must be before end time if both are provided.
  
  End:
    schema:
      type: string
      format: date-time
      pattern: '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z?$'
    description: ISO 8601 timestamp (RFC3339 format) - include entries <= this time. Must be after start time if both are provided.
  ```

#### New Standardized Parameters
- **MetricType**: Centralized metric type validation
  ```yaml
  MetricType:
    schema:
      type: string
      enum: [gauge, sum, histogram, summary, exponential_histogram]
    description: Filter by metric type
  ```

- **Severity**: Centralized severity level validation
  ```yaml
  Severity:
    schema:
      type: string
      enum: [trace, debug, info, warn, error, fatal]
    description: Filter by log severity/level
  ```

- **Service**: Centralized service name validation
  ```yaml
  Service:
    schema:
      type: string
      minLength: 1
    description: Filter by service name
  ```

- **Query**: Centralized search query validation
  ```yaml
  Query:
    schema:
      type: string
      minLength: 1
    description: Search query to filter logs, traces, or metrics
  ```

- **TimestampGte/TimestampLte**: Centralized timestamp validation
  ```yaml
  TimestampGte:
    schema:
      type: string
      format: date-time
      pattern: '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z?$'
    description: ISO 8601 timestamp (RFC3339 format) - include entries >= this time
  
  TimestampLte:
    schema:
      type: string
      format: date-time
      pattern: '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z?$'
    description: ISO 8601 timestamp (RFC3339 format) - include entries <= this time
  ```

### 2. **Enhanced Error Responses** (`components/responses.yaml`)

#### Updated BadRequest Response
- **Before**: Single Error schema
- **After**: Supports both Error and ValidationErrors schemas
  ```yaml
  BadRequest:
    description: Invalid input
    content:
      application/json:
        schema:
          oneOf:
            - $ref: './schemas.yaml#/components/schemas/Error'
            - $ref: './schemas.yaml#/components/schemas/ValidationErrors'
        examples:
          sample:
            value:
              error: "Invalid request payload"
              code: "bad_request"
              details: { field: "Value", reason: "missing" }
          validation_errors:
            summary: Validation errors
            value:
              errors:
                - field: "start"
                  message: "invalid start time format, expected RFC3339"
                - field: "limit"
                  message: "limit cannot exceed 1000"
  ```

### 3. **New Validation Schemas** (`components/schemas.yaml`)

#### ValidationError Schema
```yaml
ValidationError:
  type: object
  properties:
    field:
      type: string
      description: The field that failed validation
    message:
      type: string
      description: Human-readable validation error message
  required: [field, message]
  example:
    field: "limit"
    message: "limit cannot exceed 1000"
```

#### ValidationErrors Schema
```yaml
ValidationErrors:
  type: object
  properties:
    errors:
      type: array
      items:
        $ref: '#/components/schemas/ValidationError'
      description: Collection of validation errors
  required: [errors]
  example:
    errors:
      - field: "start"
        message: "invalid start time format, expected RFC3339"
      - field: "limit"
        message: "limit cannot exceed 1000"
```

### 4. **Updated Path Files**

#### Logs Path (`paths/logs.yaml`)
- ✅ Updated to use new `Service` parameter
- ✅ Updated to use new `Severity` parameter
- ✅ Updated to use new `Query` parameter
- ✅ Updated to use new `TimestampGte` and `TimestampLte` parameters
- ✅ Updated to use new `Offset` parameter for cursor endpoint

#### Traces Path (`paths/traces.yaml`)
- ✅ Updated to use new `Service` parameter
- ✅ Updated to use new `Query` parameter
- ✅ Updated to use new `Offset` parameter for cursor endpoint

#### Metrics Path (`paths/metrics.yaml`)
- ✅ Updated to use new `Service` parameter
- ✅ Updated to use new `Query` parameter
- ✅ Updated to use new `MetricType` parameter
- ✅ Updated to use new `Offset` parameter for cursor endpoint

## Validation Rules Documented

### 1. **Limit Validation**
- **Minimum**: 1
- **Maximum**: 1000
- **Default**: 100
- **Behavior**: Values exceeding 1000 are automatically normalized

### 2. **Offset Validation**
- **Minimum**: 0
- **Maximum**: 100,000
- **Default**: 0
- **Behavior**: Prevents negative values and enforces reasonable bounds

### 3. **Time Range Validation**
- **Format**: RFC3339 (ISO 8601)
- **Pattern**: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z?$`
- **Constraints**: Start time must be before end time
- **Maximum Range**: 30 days (enforced by backend)

### 4. **Required Field Validation**
- **Service Name**: Minimum length 1
- **Search Query**: Minimum length 1
- **Metric Type**: Must be one of predefined enum values
- **Severity Level**: Must be one of predefined enum values

## Benefits of Updates

### 1. **API Documentation Accuracy**
- OpenAPI spec now accurately reflects actual validation behavior
- Developers can see exact constraints and limits
- Clear error response formats documented

### 2. **Client Code Generation**
- Generated clients will include proper validation
- Type-safe parameter handling
- Consistent error handling across all endpoints

### 3. **Developer Experience**
- Clear parameter constraints visible in API docs
- Expected error formats documented
- Validation rules transparent and discoverable

### 4. **API Consistency**
- Standardized parameter definitions across all endpoints
- Consistent validation error responses
- Uniform constraint enforcement

## Files Modified

1. **`trazrextension/openapi/components/parameters.yaml`** - Enhanced parameter validation
2. **`trazrextension/openapi/components/responses.yaml`** - Updated error responses
3. **`trazrextension/openapi/components/schemas.yaml`** - Added validation schemas
4. **`trazrextension/openapi/paths/logs.yaml`** - Updated parameter references
5. **`trazrextension/openapi/paths/traces.yaml`** - Updated parameter references
6. **`trazrextension/openapi/paths/metrics.yaml`** - Updated parameter references

## Testing Status

- ✅ All parameter references updated
- ✅ New validation schemas added
- ✅ Error response formats documented
- ✅ Constraint limits documented
- ✅ Pattern validation added for timestamps

## Future Enhancements

### 1. **Additional Validation Rules**
- Service name format validation patterns
- Metric type enum expansion
- Cursor format validation

### 2. **Enhanced Examples**
- More comprehensive error examples
- Validation failure scenarios
- Best practice usage examples

### 3. **Performance Documentation**
- Query performance implications
- Recommended parameter ranges
- Rate limiting information

## Conclusion

The OpenAPI specification has been comprehensively updated to reflect all the new validation rules and constraints implemented in the trazrextension package. The documentation now provides:

- **Accurate parameter constraints** with clear limits and validation rules
- **Structured error responses** for validation failures
- **Consistent parameter definitions** across all endpoints
- **Clear validation patterns** for timestamps and other fields
- **Comprehensive examples** of error scenarios

All updates maintain backward compatibility while providing developers with the information they need to properly validate parameters and handle errors in their API clients.














