#!/bin/bash

# Simple Configuration Management API Test
TOKEN="${TRAZR_TEST_JWT}"
BASE_URL="http://localhost:18888/api/v1/config"

echo "=== Configuration Management API Simple Test ==="

echo "1. Statistics:"
curl -s -H "Authorization: Bearer $TOKEN" "$BASE_URL/statistics" | jq .

echo -e "\n2. Create Processor:"
CREATE_RESPONSE=$(curl -s -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "name": "simple-test-processor",
    "enabled": true,
    "priority": 30,
    "metadata": {
      "description": "Simple test processor",
      "type": "batch",
      "batch_size": 500
    }
  }' \
  "$BASE_URL/processors")

echo "$CREATE_RESPONSE" | jq .
PROCESSOR_ID=$(echo "$CREATE_RESPONSE" | jq -r '.id // empty')

if [[ -n "$PROCESSOR_ID" && "$PROCESSOR_ID" != "null" ]]; then
    echo -e "\n3. Read Processor by ID:"
    curl -s -H "Authorization: Bearer $TOKEN" "$BASE_URL/processors/$PROCESSOR_ID" | jq .
    
    echo -e "\n4. Update Processor:"
    curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
      -d '{
        "name": "simple-test-processor-updated",
        "enabled": false,
        "priority": 60,
        "metadata": {
          "description": "Updated simple test processor",
          "type": "stream",
          "batch_size": 1000
        }
      }' \
      "$BASE_URL/processors/$PROCESSOR_ID" | jq .
    
    echo -e "\n5. Delete Processor:"
    curl -s -X DELETE -H "Authorization: Bearer $TOKEN" "$BASE_URL/processors/$PROCESSOR_ID"
    
    echo -e "\n6. Verify Deletion:"
    curl -s -H "Authorization: Bearer $TOKEN" "$BASE_URL/processors/$PROCESSOR_ID" | jq .
else
    echo "Failed to create processor"
fi

echo -e "\n7. List Processors with Filters:"
curl -s -H "Authorization: Bearer $TOKEN" "$BASE_URL/processors?enabled=true&limit=5" | jq .

echo -e "\n=== Test Complete ==="
