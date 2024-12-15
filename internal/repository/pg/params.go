package pg

import "time"

// ListParams represents the parameters for listing items in a repository.
// It includes filters for IP address, user agent, anomaly status, time range,
// pagination, and sorting.
type ListParams struct {
	IP         string
	UserAgent  string
	HasAnomaly bool
	TimeRange  TimeRange
	Pagination PaginationParams
	SortParams SortParams
}

// TimeRange represents a range of time, with a Start and End time.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// PaginationParams represents the parameters for pagination, including the
// limit of items to return and the offset to start from.
type PaginationParams struct {
	Limit  int
	Offset int
}

// SortParams represents the parameters for sorting a list of items, including
// the field to sort by and the sort direction (ASC or DESC).
type SortParams struct {
	Field     string
	Direction string // ASC or DESC
}
