# Go code conventions

This guide is the single source of Go coding conventions for the project.

The examples use an imaginary `catalog` domain with a `Product` aggregate. They
show the expected shape of the code, not code that exists in the repository.

## 1. File layout

### Files

- Put each aggregate and its value types in `<entity>.go` in the domain
  package: `product.go` holds `ProductID`, `ProductName`, and `Product`.
- Put a value type that several aggregates share in its own file, named after
  the concept: `price.go` (`Price`, `Currency`), `text.go` (`Translation`,
  `Text`).
- Put the repository contract in `<entity>_repository.go` in the domain
  package. That file holds the repository sentinels, the typed write-failure
  interfaces, the repository interface and, when needed, the
  `<Entity>Filter`, `<Entity>Sort` and `<Entity>Query` types.
- Put the PostgreSQL implementation in `<domain>/pgxgoqu/<entity>_repository.go`
  as a `<Entity>Repository` struct.

```text
internal/catalog/
├── price.go
├── product.go
├── product_repository.go
└── pgxgoqu/
    └── product_repository.go
```

### Section banners

- Separate top-level sections with a three-line banner:

  ```go
  //
  // Product name
  //
  ```

- In domain files, give each named type its own banner, named in sentence case
  after the concept: `// Product ID`, `// Product name`, `// Price`.
- Put the aggregate's banner last, after all its value types:

  ```go
  //
  // Product ID
  //

  type ProductID uuid.UUID

  //
  // Product name
  //

  type ProductName string

  //
  // Product
  //

  type Product struct {
  	ID   ProductID
  	Name ProductName
  }
  ```

- In adapter files, use exactly these banners, in this order: `Errors`,
  `Repository`, `Helpers`.
- In a domain `_repository.go` that defines query types, use the banners
  `Filter`, `Sort`, `Query`, `Repository` in that order:

  ```go
  //
  // Filter
  //

  type ProductFilter struct { /* ... */ }

  //
  // Sort
  //

  type ProductSort string

  //
  // Query
  //

  type ProductQuery domquery.Query[ProductFilter, ProductSort]

  //
  // Repository
  //

  type ProductRepository interface { /* ... */ }
  ```

### Sub-headers

- Inside the `Repository` section, put a one-line comment, followed by a blank
  line, above each method. It names the operation in sentence case with no
  period and uses articles:

  ```go
  // Find by an ID

  func (r *ProductRepository) FindByID(
  	ctx context.Context,
  	id catalog.ProductID,
  ) (*catalog.Product, error) {
  ```

  Other examples: `// Create`, `// Find by a name`,
  `// Find the default category`, `// Count products matching filters`.
- Inside the `Errors` section, put `// Implementation of <pkg>.<Interface>`
  above each typed failure implementation:

  ```go
  // Implementation of catalog.ErrProductCreateFailed

  type errProductCreateFailed struct {
  	errProductWriteFailed
  }

  func (e *errProductCreateFailed) Error() string {
  	return "failed to create a product"
  }
  ```

- A sub-header may become a full-sentence explanation when the method relies
  on behaviour that isn't obvious, such as database triggers or concurrency
  tokens. See [Comments](#10-comments).

### Order inside a section

- Value-type sections: constants, then the `var` block of errors (with any
  regexps), then the type, then `Validate`:

  ```go
  //
  // Product name
  //

  const MaxProductNameLength = 200

  var (
  	ErrProductNameInvalid = errors.New("invalid product name")
  	ErrProductNameEmpty   = errors.New("is empty")
  	ErrProductNameTooLong = fmt.Errorf("exceeds the limit of %d characters", MaxProductNameLength)
  )

  type ProductName string

  func (n ProductName) Validate() error {
  	var err error

  	if strings.TrimSpace(string(n)) == "" {
  		err = ErrProductNameEmpty
  	} else if utf8.RuneCountInString(string(n)) > MaxProductNameLength {
  		err = ErrProductNameTooLong
  	}

  	if err != nil {
  		err = fmt.Errorf("%w: %w", ErrProductNameInvalid, err)
  	}

  	return err
  }
  ```

- Constants of an enum-like type come right after the type:

  ```go
  type Availability string

  const (
  	AvailabilityInStock    Availability = "in_stock"
  	AvailabilityPreorder   Availability = "preorder"
  	AvailabilityOutOfStock Availability = "out_of_stock"
  )

  func (a Availability) Validate() error {
  ```

- Aggregate sections: error `var` block, struct, `New<Entity>`, `Validate`:

  ```go
  //
  // Product
  //

  var (
  	ErrProductInvalid = errors.New("invalid product")
  	ErrProductNil     = errors.New("is nil")
  )

  type Product struct {
  	ID    ProductID
  	Name  ProductName
  	Price Price
  }

  func NewProduct(id ProductID, name ProductName, price Price) *Product {
  	return &Product{ID: id, Name: name, Price: price}
  }

  func (p *Product) Validate() error {
  ```

- Domain repository section: sentinels, then `Err<Entity>CreateFailed`,
  `UpdateFailed`, `DeleteFailed`, then the repository interface:

  ```go
  var (
  	ErrProductNotFound      = errors.New("product not found")
  	ErrProductAlreadyExists = errors.New("product already exists")
  )

  type ErrProductCreateFailed interface {
  	error
  	Product() *Product
  	Unwrap() error
  }

  type ErrProductUpdateFailed interface {
  	error
  	Product() *Product
  	Unwrap() error
  }

  type ErrProductDeleteFailed interface {
  	error
  	ProductID() ProductID
  	Unwrap() error
  }

  type ProductRepository interface {
  	Create(ctx context.Context, product *Product) ErrProductCreateFailed
  	Update(ctx context.Context, product *Product) ErrProductUpdateFailed
  	Delete(ctx context.Context, id ProductID) ErrProductDeleteFailed
  	FindByID(ctx context.Context, id ProductID) (*Product, error)
  }
  ```

- Adapter `Repository` section: the DB interface (if it's local to the file),
  then the struct, the `var _` check, `New<X>Repository`, then the methods in
  the interface's order:

  ```go
  //
  // Repository
  //

  type productDB interface {
  	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
  	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
  }

  type ProductRepository struct {
  	pool productDB
  }

  var _ catalog.ProductRepository = (*ProductRepository)(nil)

  func NewProductRepository(pool *pgxpool.Pool) (*ProductRepository, error) {
  	// ...
  }

  // Create

  func (r *ProductRepository) Create( /* ... */ )

  // Update

  func (r *ProductRepository) Update( /* ... */ )

  // Delete

  func (r *ProductRepository) Delete( /* ... */ )

  // Find by an ID

  func (r *ProductRepository) FindByID( /* ... */ )
  ```

- Adapter `Helpers` section: the private lookup or write helpers, then
  `<entity>Columns`, query builders, `scan<Entity>`, `map<Entity>Error`:

  ```go
  //
  // Helpers
  //

  func (r *ProductRepository) find(ctx context.Context, predicate exp.Expression) (*catalog.Product, error) {
  	// ...
  }

  func productColumns() []any {
  	return []any{"id", "name", "price", "currency"}
  }

  func productFilterQuery(filter catalog.ProductFilter) *goqu.SelectDataset {
  	// ...
  }

  func scanProduct(row pgx.Row) (*catalog.Product, error) {
  	// ...
  }

  func mapProductError(err error) error {
  	// ...
  }
  ```
