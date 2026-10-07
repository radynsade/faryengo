package domquery

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

type Query[FilterType any, SortType ~string] struct {
	filter    FilterType
	sortOrder SortOrder
	sortBy    SortType
	limit     int
	page      int
}

func NewQuery[F any, S ~string](filter F, sortBy S, sortOrder SortOrder, limit, page int) Query[F, S] {
	return Query[F, S]{filter: filter, sortBy: sortBy, sortOrder: sortOrder, limit: limit, page: page}
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
