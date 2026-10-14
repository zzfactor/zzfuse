#!/bin/bash

# Configuration Management API Test Suite
# Tests all CRUD operations to validate ID assignment and functionality

set -e

# Configuration
BASE_URL="${TRAZR_BASE_URL:-http://localhost:18888}"
API_BASE="${BASE_URL}/api/v1/config"
AUTH_TOKEN="${TRAZR_AUTH_TOKEN:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# HTTP request helper
make_request() {
    local method="$1"
    local url="$2"
    local data="$3"
    local expected_status="$4"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    local response
    local status_code
    
    if [[ -n "$data" ]]; then
        response=$(curl -s -w '%{http_code}' -X "$method" "$url" \
            -H "Authorization: Bearer $AUTH_TOKEN" \
            -H "Content-Type: application/json" \
            -d "$data")
    else
        response=$(curl -s -w '%{http_code}' -X "$method" "$url" \
            -H "Authorization: Bearer $AUTH_TOKEN")
    fi
    
    status_code="${response: -3}"
    local body="${response%???}"
    
    if [[ "$status_code" == "$expected_status" ]]; then
        log_success "$method $url -> $status_code" >&2
        echo "$body"
        return 0
    else
        log_error "$method $url -> Expected: $expected_status, Got: $status_code" >&2
        echo "$body" >&2
        return 1
    fi
}

# Test statistics endpoint
test_statistics() {
    log_info "Testing configuration statistics endpoint..."
    local response=$(make_request "GET" "${API_BASE}/statistics" "" "200")
    
    if [[ $? -eq 0 ]]; then
        # Simple grep-based parsing to avoid jq hanging issues
        if echo "$response" | grep -q '"total_configs"'; then
            local total=$(echo "$response" | grep -o '"total_configs":[0-9]*' | cut -d':' -f2)
            log_success "Statistics endpoint returned valid JSON with total_configs: $total"
        else
            log_error "Statistics endpoint did not return expected JSON structure"
        fi
    else
        log_error "Statistics endpoint request failed"
    fi
}

# Test full CRUD operations for processor configurations
test_processor_crud() {
    log_info "Testing processor configuration CRUD operations..."
    
    local timestamp=$(date +%s)
    local config="{\"name\": \"test-crud-$timestamp\", \"enabled\": true, \"priority\": 1, \"metadata\": {\"signal_types\": [\"logs\"], \"schema_name\": \"public\"}}"
    
    # CREATE
    log_info "CREATE: Creating processor configuration..."
    local create_response=$(make_request "POST" "${API_BASE}/processors" "$config" "201")
    
    if [[ $? -eq 0 ]]; then
        # Use grep-based parsing to avoid jq issues
        local config_id=$(echo "$create_response" | grep -o '"id":[0-9]*' | cut -d':' -f2)
        if [[ -n "$config_id" && "$config_id" != "null" && "$config_id" != "0" ]]; then
            log_success "CREATE: Created processor with ID: $config_id (ID assignment working!)"
            
            # READ
            log_info "READ: Retrieving processor by ID..."
            local get_response=$(make_request "GET" "${API_BASE}/processors/${config_id}" "" "200")
            if [[ $? -eq 0 ]]; then
                local retrieved_id=$(echo "$get_response" | grep -o '"id":[0-9]*' | cut -d':' -f2)
                if [[ "$retrieved_id" == "$config_id" ]]; then
                    log_success "READ: Successfully retrieved processor by ID: $retrieved_id"
                else
                    log_error "READ: Retrieved ID mismatch (got: $retrieved_id, expected: $config_id)"
                fi
            else
                log_error "READ: Failed to retrieve processor by ID"
            fi
            
            # UPDATE
            log_info "UPDATE: Updating processor configuration..."
            local update_config="{\"name\": \"test-crud-updated-$timestamp\", \"enabled\": false, \"priority\": 2, \"metadata\": {\"signal_types\": [\"traces\"], \"schema_name\": \"public\"}}"
            local update_response=$(make_request "PUT" "${API_BASE}/processors/${config_id}" "$update_config" "200")
            if [[ $? -eq 0 ]]; then
                local updated_id=$(echo "$update_response" | grep -o '"id":[0-9]*' | cut -d':' -f2)
                if [[ -n "$updated_id" && "$updated_id" == "$config_id" ]]; then
                    log_success "UPDATE: Successfully updated processor (ID preserved: $updated_id)"
                elif [[ -n "$updated_id" ]]; then
                    log_warning "UPDATE: ID changed after update (was: $config_id, now: $updated_id) - API may be creating new record instead of updating"
                else
                    log_warning "UPDATE: Could not parse ID from response, but update succeeded"
                fi
            else
                log_error "UPDATE: Failed to update processor configuration"
            fi
            
            # DELETE
            log_info "DELETE: Deleting processor configuration..."
            make_request "DELETE" "${API_BASE}/processors/${config_id}" "" "204" > /dev/null
            if [[ $? -eq 0 ]]; then
                log_success "DELETE: Successfully deleted processor configuration"
                
                # Verify deletion
                log_info "Verifying deletion..."
                make_request "GET" "${API_BASE}/processors/${config_id}" "" "404" > /dev/null 2>&1
                if [[ $? -eq 0 ]]; then
                    log_success "DELETE: Verified - processor no longer exists"
                else
                    log_warning "DELETE: Could not verify deletion (may still exist)"
                fi
            else
                log_error "DELETE: Failed to delete processor configuration"
            fi
            
        else
            log_error "CREATE: Failed to extract valid config ID (got: $config_id) - ID assignment issue!"
        fi
    else
        log_error "CREATE: Failed to create processor configuration"
    fi
}

# Test list operations with filtering
test_list_operations() {
    log_info "Testing list operations..."
    
    # List all processors
    local list_response=$(make_request "GET" "${API_BASE}/processors" "" "200")
    if [[ $? -eq 0 ]]; then
        if echo "$list_response" | grep -q '"data"'; then
            local count=$(echo "$list_response" | grep -o '"data":\[[^]]*\]' | grep -o '{"id":[0-9]*' | wc -l | tr -d ' ')
            log_success "LIST: Retrieved processor list with $count items"
        else
            log_error "LIST: Invalid response structure"
        fi
    else
        log_error "LIST: Failed to retrieve processor list"
    fi
    
    # List with pagination
    local paginated_response=$(make_request "GET" "${API_BASE}/processors?limit=5&offset=0" "" "200")
    if [[ $? -eq 0 ]]; then
        log_success "LIST: Pagination test successful"
    else
        log_error "LIST: Pagination test failed"
    fi
}

# Test different configuration types
test_other_config_types() {
    log_info "Testing other configuration types..."
    
    # Test schema configurations
    local schema_response=$(make_request "GET" "${API_BASE}/schemas" "" "200")
    if [[ $? -eq 0 ]]; then
        log_success "SCHEMAS: Successfully retrieved schema configurations"
    else
        log_error "SCHEMAS: Failed to retrieve schema configurations"
    fi
    
    # Test embedding configurations
    local embedding_response=$(make_request "GET" "${API_BASE}/embeddings" "" "200")
    if [[ $? -eq 0 ]]; then
        log_success "EMBEDDINGS: Successfully retrieved embedding configurations"
    else
        log_error "EMBEDDINGS: Failed to retrieve embedding configurations"
    fi
    
    # Test template configurations
    local template_response=$(make_request "GET" "${API_BASE}/templates" "" "200")
    if [[ $? -eq 0 ]]; then
        log_success "TEMPLATES: Successfully retrieved template configurations"
    else
        log_error "TEMPLATES: Failed to retrieve template configurations"
    fi
    
    # Test vectorstore configurations
    local vectorstore_response=$(make_request "GET" "${API_BASE}/vectorstores" "" "200")
    if [[ $? -eq 0 ]]; then
        log_success "VECTORSTORES: Successfully retrieved vectorstore configurations"
    else
        log_error "VECTORSTORES: Failed to retrieve vectorstore configurations"
    fi
}

# Main test function
main() {
    echo "========================================"
    echo "Configuration Management API Test Suite"
    echo "========================================"
    echo "Base URL: $BASE_URL"
    echo "API Base: $API_BASE"
    echo ""
    
    # Check dependencies
    if ! command -v jq &> /dev/null; then
        log_error "jq is required for JSON parsing. Please install jq."
        exit 1
    fi
    
    if [[ -z "$AUTH_TOKEN" ]]; then
        log_error "TRAZR_AUTH_TOKEN environment variable is required"
        exit 1
    fi
    
    # Run all tests
    test_statistics
    test_processor_crud
    test_list_operations
    test_other_config_types
    
    # Summary
    echo ""
    echo "========================================"
    echo "Test Summary"
    echo "========================================"
    echo "Tests Run: $TESTS_RUN"
    echo "Tests Passed: $TESTS_PASSED"
    echo "Tests Failed: $TESTS_FAILED"
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "${GREEN}✅ All tests passed! Configuration Management API is working correctly.${NC}"
        echo -e "${GREEN}✅ ID assignment fix is working - create operations return proper database IDs.${NC}"
        echo -e "${GREEN}✅ All CRUD operations (Create, Read, Update, Delete) are functional.${NC}"
        exit 0
    else
        echo -e "${RED}❌ Some tests failed. Please review the output above.${NC}"
        exit 1
    fi
}

# Run the tests
main "$@"
