package domquery

// Sort order

type SortOrder bool

const (
	SortOrderAsc  = true
	SortOrderDesc = false
)

func (o SortOrder) IsAsc() bool {
	return bool(o)
}

func (o SortOrder) IsDesc() bool {
	return !bool(o)
}

// Filter

type Filter interface {
	IsApplied() bool
	Count() int
}

// Query

type Query[FilterType Filter, SortType ~string] struct {
	filter    FilterType
	sortOrder SortOrder
	sortBy    SortType
	limit     int
	page      int
}

func NewQuery[FilterType Filter, SortType ~string](
	filter FilterType,
	sortBy SortType,
	sortOrder SortOrder,
	limit, page int,
) Query[FilterType, SortType] {
	return Query[FilterType, SortType]{
		filter:    filter,
		sortBy:    sortBy,
		sortOrder: sortOrder,
		limit:     limit,
		page:      page,
	}
}

func (q *Query[FilterType, SortType]) Filters() FilterType {
	return q.filter
}

func (q *Query[FilterType, SortType]) SortOrder() SortOrder {
	return q.sortOrder
}

func (q *Query[FilterType, SortType]) SortBy() SortType {
	return q.sortBy
}

func (q *Query[FilterType, SortType]) Limit() int {
	return q.limit
}

func (q *Query[FilterType, SortType]) Page() int {
	return q.page
}
