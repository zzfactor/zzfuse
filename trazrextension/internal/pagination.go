package internal

// PaginationResponse provides a standardized pagination structure
// that can be reused across all API endpoints (logs, traces, metrics, etc.)
// Cursors are always returned (not omitempty) to support streaming use cases
type PaginationResponse struct {
	NextCursor string `json:"next_cursor"`
	PrevCursor string `json:"prev_cursor"`
	HasNext    bool   `json:"has_next_page"`
	HasPrev    bool   `json:"has_prev_page"`
}

// PaginatedResponse wraps any data with standardized pagination metadata
type PaginatedResponse[T any] struct {
	Data       T                  `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

// NewPaginatedResponse creates a new paginated response with the given data and pagination info
// Cursors are passed through as-is from the repository layer, which generates real cursors from data
func NewPaginatedResponse[T any](data T, nextCursor, prevCursor string, hasNext, hasPrev bool) PaginatedResponse[T] {
	return PaginatedResponse[T]{
		Data: data,
		Pagination: PaginationResponse{
			NextCursor: nextCursor, // Real cursor from repository data
			PrevCursor: prevCursor, // Real cursor from repository data
			HasNext:    hasNext,
			HasPrev:    hasPrev,
		},
	}
}
