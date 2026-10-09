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
	Count() int
}

// Query

type Query[FilterType Filter, SortType ~string] struct {
	Filter    FilterType
	SortOrder SortOrder
	SortBy    SortType
	Limit     uint
	Page      uint
}
