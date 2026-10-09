# domquery

`pkg/domquery` provides generic building blocks for repository list queries:
a filter contract, a sort direction, and a `Query` struct that combines filtering,
sorting, and pagination. It imports nothing and knows nothing about storage, HTTP,
or a particular domain. Each domain defines its own filter and sort types and
instantiates `Query` with them.

Import `github.com/radynsade/faryengo/pkg/domquery`.

## Types

| Type | Purpose |
| --- | --- |
| `Filter` | Interface implemented by a domain's filter struct. `Count()` returns the number of applied criteria. |
| `SortOrder` | Sort direction backed by `bool`. `SortOrderAsc` is `true`, `SortOrderDesc` is `false`. |
| `Query[FilterType, SortType]` | A filter, sort field, sort direction, `Limit`, and `Page`. |

`SortOrder` reports its direction through `IsAsc()` and `IsDesc()`. The zero
value is `SortOrderDesc`, so set the direction explicitly when ascending order is
the intended default.

`Query` is parameterized by a filter type implementing `Filter` and a sort type
whose underlying type is `string`. The sort type is usually a named string with
constants for the sortable fields, which keeps arbitrary column names out of the
query.

`Limit` and `Page` are unsigned and uninterpreted: the package does not apply
defaults, bounds, or a page base. The domain that defines the query documents
and validates them, and the repository converts them to an offset.

## Defining a domain query

Define the filter, the sort fields, and a named query type in the domain package.
Validation belongs to the domain, not to `domquery`.

```go
package catalog

import (
	"errors"

	"github.com/radynsade/faryengo/pkg/domquery"
)

var ErrInvalidProductQuery = errors.New("invalid product query")

type ProductFilter struct {
	NameLike string
	InStock  *bool
}

// Count reports how many criteria are applied, e.g. for a "2 filters" badge.
func (f ProductFilter) Count() int {
	count := 0

	for _, applied := range []bool{f.NameLike != "", f.InStock != nil} {
		if applied {
			count++
		}
	}

	return count
}

type ProductSort string

const (
	ProductSortName  = ProductSort("name")
	ProductSortPrice = ProductSort("price")
)

type ProductQuery domquery.Query[ProductFilter, ProductSort]

func (q ProductQuery) Validate() error {
	var err error

	if q.SortBy != ProductSortName && q.SortBy != ProductSortPrice || q.Limit == 0 || q.Page == 0 {
		err = ErrInvalidProductQuery
	}

	return err
}
```

Declaring `ProductQuery` as a named type rather than an alias lets the domain
attach methods such as `Validate` while keeping the fields of `domquery.Query`.

## Building and consuming a query

Callers fill the struct directly:

```go
inStock := true
query := catalog.ProductQuery{
	Filter:    catalog.ProductFilter{NameLike: "lamp", InStock: &inStock},
	SortBy:    catalog.ProductSortPrice,
	SortOrder: domquery.SortOrderAsc,
	Limit:     20,
	Page:      1,
}
```

A repository translates the sort field through a closed `switch`, so only known
fields reach SQL, and reads the direction with `IsDesc`:

```go
column := "name"

if query.SortBy == catalog.ProductSortPrice {
	column = "price"
}

order := goqu.C(column).Asc()

if query.SortOrder.IsDesc() {
	order = goqu.C(column).Desc()
}

sql, args, err := productFilterQuery(query.Filter).
	Order(order).
	Limit(query.Limit).
	Offset((query.Page - 1) * query.Limit). // This domain treats Page as 1-based.
	Prepared(true).
	ToSQL()
```

Because the filter is a separate type, the same value can drive both `Find`
(with sorting and pagination) and `Count` (filter only) repository methods.
