package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
)

const budgetsPath = designPath + "/budgets"

func TestBudgetsPage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "shopping-cart", "green")
	travel := f.addCategory(designID, "Travel", "", "blue")
	small := f.addBudget(designID, 50_00, travel)
	large := f.addBudget(designID, 1234_56, food, travel)
	f.addBudget(familyID, 10_00, f.addCategory(familyID, "Family only", "", "red"))

	response := f.do(request{path: budgetsPath, cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	largeID := uuid.UUID(large.ID).String()

	for _, want := range []string{
		"<title>Budgets · Faryen</title>",
		`<h1 id="page-title">Budgets</h1>`,
		`href="` + budgetsPath + `" aria-current="page"`,
		`<a class="button" href="` + budgetsPath + `/new"`,
		`<li id="budget-` + largeID + `" class="budget-list__item"><a class="budget-list__link" href="` + budgetsPath + "/" + largeID + `"`,
		`<span class="budget-list__title">Food, Travel</span>`,
		`<span class="budget-list__amount">1234.56 EUR</span>`,
		`<span class="budget-list__amount">50.00 EUR</span>`,
		`category-icon--shopping-cart`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Index(body, "budget-"+largeID) > strings.Index(body, "budget-"+uuid.UUID(small.ID).String()) {
		t.Fatalf("budgets are not the largest first:\n%s", body)
	}

	if strings.Contains(body, "Family only") || strings.Contains(body, `class="pagination"`) || strings.Contains(body, `id="budget-form"`) {
		t.Fatalf("body shows another workspace's budget, a pagination, or the form:\n%s", body)
	}
}

// The menu leads to the budgets from every section of the workspace, after
// the categories.

func TestBudgetsInSidebar(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: designPath, cookies: f.signIn(t)}).Body.String()
	link := `href="` + budgetsPath + `"`

	if !strings.Contains(body, link) || !strings.Contains(body, `#budgets"></use></svg><span>Budgets</span>`) ||
		strings.Index(body, link) < strings.Index(body, `href="`+designPath+`/categories"`) {
		t.Fatalf("menu lacks the budgets after the categories:\n%s", body)
	}
}

// Amounts are written with the interface language's decimal separator.

func TestBudgetsPageInLatvian(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.addBudget(designID, 250_50, f.addCategory(designID, "Pārtika", "", "green"))

	body := f.do(request{path: "/office/lv/workspaces/" + designID + "/budgets", cookies: cookies}).Body.String()

	if !strings.Contains(body, `<span class="budget-list__amount">250,50 EUR</span>`) || !strings.Contains(body, "<span>Budžeti</span>") {
		t.Fatalf("Latvian list does not use its decimal separator:\n%s", body)
	}
}

// The form that adds a budget has its own page, which offers the workspace's
// categories and every currency, and leads back to the list.

func TestNewBudgetPage(t *testing.T) {
	f := newFixture(t)
	food := f.addCategory(designID, "Food", "shopping-cart", "green")
	f.addCategory(familyID, "Family only", "", "red")

	response := f.do(request{path: budgetsPath + "/new", cookies: f.signIn(t)})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	for _, want := range []string{
		"<title>New budget · Faryen</title>",
		`<div id="messages" class="messages"></div>`,
		`href="` + budgetsPath + `" aria-current="page"`,
		`<form id="budget-form" class="form" method="post" action="` + budgetsPath + `" novalidate`,
		`<input type="checkbox" id="budget-category-` + uuid.UUID(food.ID).String() + `" name="categories" value="` + uuid.UUID(food.ID).String() + `">`,
		`<span class="budget-categories__name">Food</span>`,
		`<option value selected>Choose a currency</option>`,
		`<option value="EUR">EUR (€)</option>`,
		`name="amount"`,
		`<a class="form__cancel" href="` + budgetsPath + `"`,
		">Add budget</button>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "Family only") || strings.Contains(body, "budget-list") {
		t.Fatalf("new budget page offers another workspace's category or shows the list:\n%s", body)
	}
}

// A workspace without categories has nothing to budget, and its form says so.

func TestNewBudgetWithoutCategories(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: budgetsPath + "/new", cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(body, "This workspace has no categories yet.") || strings.Contains(body, `name="categories"`) {
		t.Fatalf("form does not explain the missing categories:\n%s", body)
	}
}

// Without the categories or the currencies the form cannot be offered, so its
// page reports the budgets as unavailable.

func TestNewBudgetWithoutChoices(t *testing.T) {
	for _, tt := range []struct {
		name string
		fail func(f *fixture)
	}{
		{"categories", func(f *fixture) { f.categoryErr = errors.New("storage unavailable") }},
		{"currencies", func(f *fixture) { f.currencyErr = errors.New("storage unavailable") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			tt.fail(f)

			response := f.do(request{path: budgetsPath + "/new", cookies: cookies})

			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "Budgets are unavailable right now.") {
				t.Fatalf("new budget without %s = %d:\n%s", tt.name, response.Code, response.Body.String())
			}
		})
	}
}

func TestBudgetsEmpty(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: budgetsPath, cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(body, `<p class="budget-list__empty">There are no budgets yet.</p>`) || strings.Contains(body, "budget-list__item") {
		t.Fatalf("empty list is not explained:\n%s", body)
	}
}

func TestBudgetsUnavailable(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.budgetErr = errors.New("storage unavailable")

	response := f.do(request{path: budgetsPath, cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK || !strings.Contains(body, "Budgets are unavailable right now.") || !strings.Contains(body, `href="`+budgetsPath+`/new"`) {
		t.Fatalf("unavailable list = %d:\n%s", response.Code, body)
	}
}

// A viewer sees the budgets without ways to change them, and is refused every
// change.

func TestBudgetsAsViewer(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "", "green")
	budget := f.addBudget(designID, 100_00, food)
	f.setRole(designID, workspaces.RoleViewer)

	body := f.do(request{path: budgetsPath, cookies: cookies}).Body.String()

	if !strings.Contains(body, "Only editors and owners can change budgets.") || !strings.Contains(body, ">Food<") {
		t.Fatalf("viewer list lacks the budgets:\n%s", body)
	}

	for _, unwanted := range []string{`id="budget-form"`, `class="button"`, budgetsPath + "/"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("viewer list contains %s:\n%s", unwanted, body)
		}
	}

	budgetPath := budgetsPath + "/" + uuid.UUID(budget.ID).String()
	values := budgetBody("EUR", "5", food)

	for _, r := range []request{
		{path: budgetPath},
		{path: budgetsPath + "/new"},
		{method: http.MethodPost, path: budgetsPath, body: values},
		{method: http.MethodPost, path: budgetPath, body: values},
		{method: http.MethodPost, path: budgetPath + "/delete", body: "confirm=1"},
	} {
		r.cookies = cookies

		if response := f.do(r); response.Code != http.StatusForbidden {
			t.Fatalf("%s %s as a viewer = %d", r.method, r.path, response.Code)
		}
	}

	if len(f.budgets) != 1 || f.budgets[0].Amount != 100_00 {
		t.Fatalf("budgets changed: %+v", f.budgets)
	}
}

// Another workspace's budgets, and those of a workspace the user does not
// belong to, are not found.

func TestBudgetsNotFound(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	familyCategory := f.addCategory(familyID, "Family only", "", "red")
	family := f.addBudget(familyID, 10_00, familyCategory)
	other := f.addBudget(otherID, 20_00, f.addCategory(otherID, "Someone else's", "", "red"))

	for _, path := range []string{
		budgetsPath + "/" + uuid.UUID(family.ID).String(),
		budgetsPath + "/" + uuid.UUID(other.ID).String(),
		budgetsPath + "/01920000-0000-7000-8000-0000000000ff",
		budgetsPath + "/not-a-uuid",
		"/office/en/workspaces/" + otherID + "/budgets",
		"/office/en/workspaces/" + otherID + "/budgets/" + uuid.UUID(other.ID).String(),
	} {
		response := f.do(request{path: path, cookies: cookies})

		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "only") || strings.Contains(response.Body.String(), "Someone") {
			t.Fatalf("GET %s = %d:\n%s", path, response.Code, response.Body.String())
		}
	}

	response := f.do(request{
		method:  http.MethodPost,
		path:    budgetsPath + "/" + uuid.UUID(family.ID).String(),
		body:    budgetBody("EUR", "99", familyCategory),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != budgetsPath || family.Amount != 10_00 {
		t.Fatalf("update of another workspace's budget = %d %q, amount %d", response.Code, response.Header().Get("Location"), family.Amount)
	}

	if response := f.do(request{path: budgetsPath}); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("budgets as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestCreateBudget(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "", "green")
	travel := f.addCategory(designID, "Travel", "", "blue")

	response := f.do(request{
		method:  http.MethodPost,
		path:    budgetsPath,
		body:    budgetBody("USD", " 1 250,5 ", travel, food),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != budgetsPath {
		t.Fatalf("create = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if len(f.budgets) != 1 {
		t.Fatalf("budgets = %+v, want one", f.budgets)
	}

	created := f.budgets[0]

	if created.Amount != 1250_50 || created.Currency != "USD" || !slices.Equal(created.Categories, budgets.CategoryIDs{travel.ID, food.ID}) ||
		created.WorkspaceID != workspaces.ID(uuid.MustParse(designID)) {
		t.Fatalf("created = %+v", created)
	}

	body := f.do(request{path: budgetsPath, cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>The budget for Travel, Food was added.</p>") ||
		!strings.Contains(body, `<span class="budget-list__amount">1250.50 USD</span>`) {
		t.Fatalf("list does not confirm the new budget:\n%s", body)
	}
}

// A rejected budget replaces only the form, keeping what was chosen and typed.

func TestCreateBudgetRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   func(f *fixture, food *categories.Category) string
		status int
		wants  []string
	}{
		{
			name:   "no categories",
			body:   func(*fixture, *categories.Category) string { return "currency=EUR&amount=10" },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-categories-error">Choose at least one category.</p>`, `value="10"`},
		},
		{
			name: "malformed category",
			body: func(_ *fixture, food *categories.Category) string {
				return budgetBody("EUR", "10", food) + "&categories=nothing"
			},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-categories-error">This value is not valid.</p>`},
		},
		{
			name: "another workspace's category",
			body: func(f *fixture, _ *categories.Category) string {
				return budgetBody("EUR", "10", f.addCategory(familyID, "Family only", "", "red"))
			},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-categories-error">This value is not valid.</p>`},
		},
		{
			name:   "repeated category",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("EUR", "10", food, food) },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-categories-error">This value is not valid.</p>`},
		},
		{
			name:   "no currency",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("", "10", food) },
			status: http.StatusUnprocessableEntity,
			wants: []string{
				`<p class="field__error" id="budget-currency-error">Choose one of the options.</p>`,
				`<input type="checkbox" id="budget-category-`, `" checked>`,
			},
		},
		{
			name:   "unknown currency",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("GBP", "10", food) },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-currency-error">Choose one of the options.</p>`},
		},
		{
			name:   "no amount",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("EUR", " ", food) },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-amount-error">Please fill in this field.</p>`},
		},
		{
			name:   "negative amount",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("EUR", "-5", food) },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-amount-error">Enter an amount such as 250 or 250.50.</p>`, `value="-5"`},
		},
		{
			name:   "three decimal places",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("EUR", "1.505", food) },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-amount-error">Enter an amount such as 250 or 250.50.</p>`},
		},
		{
			name: "too large",
			body: func(_ *fixture, food *categories.Category) string {
				return budgetBody("EUR", strings.Repeat("9", 16), food)
			},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="budget-amount-error">Enter an amount such as 250 or 250.50.</p>`},
		},
		{
			name:   "every field",
			body:   func(*fixture, *categories.Category) string { return "currency=&amount=x" },
			status: http.StatusUnprocessableEntity,
			wants: []string{
				`id="budget-categories-error"`,
				`id="budget-currency-error"`,
				`id="budget-amount-error"`,
			},
		},
		{
			name:   "unknown field",
			body:   func(_ *fixture, food *categories.Category) string { return budgetBody("EUR", "10", food) + "&name=x" },
			status: http.StatusBadRequest,
			wants:  []string{`<div class="form-alert" role="alert">`},
		},
		{
			name: "repeated amount",
			body: func(_ *fixture, food *categories.Category) string {
				return budgetBody("EUR", "10", food) + "&amount=20"
			},
			status: http.StatusBadRequest,
			wants:  []string{`<div class="form-alert" role="alert">`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			food := f.addCategory(designID, "Food", "", "green")

			response := f.do(request{method: http.MethodPost, path: budgetsPath, body: tt.body(f, food), cookies: f.signIn(t), headers: toContent})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("HX-Retarget") != "#budget-form" || !strings.HasPrefix(body, `<form id="budget-form"`) {
				t.Fatalf("create = %d %v, want %d with only the form:\n%s", response.Code, response.Header(), tt.status, body)
			}

			for _, want := range tt.wants {
				if !strings.Contains(body, want) {
					t.Fatalf("form lacks %s:\n%s", want, body)
				}
			}

			if len(f.budgets) != 0 {
				t.Fatalf("budgets = %+v, want none", f.budgets)
			}
		})
	}
}

func TestEditBudget(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "", "green")
	travel := f.addCategory(designID, "Travel", "", "blue")
	budget := f.addBudget(designID, 100_00, food)
	path := budgetsPath + "/" + uuid.UUID(budget.ID).String()
	foodID, travelID := uuid.UUID(food.ID).String(), uuid.UUID(travel.ID).String()

	body := f.do(request{path: path, cookies: cookies}).Body.String()

	for _, want := range []string{
		"<title>Food · Faryen</title>",
		`<div id="messages" class="messages"></div>`,
		`href="` + budgetsPath + `" aria-current="page"`,
		`<form id="budget-form" class="form" method="post" action="` + path + `" novalidate`,
		`value="` + foodID + `" checked>`,
		`value="` + travelID + `">`,
		`<option value="EUR" selected>EUR (€)</option>`,
		`value="100.00"`,
		`<form id="budget-delete-form" class="danger-zone" method="post" action="` + path + `/delete" data-confirm-always`,
		"The budget for Food will be deleted. This cannot be undone.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	response := f.do(request{method: http.MethodPost, path: path, body: budgetBody("USD", "75.25", travel), cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != budgetsPath {
		t.Fatalf("update = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if updated := f.budgets[0]; updated.Amount != 75_25 || updated.Currency != "USD" ||
		!slices.Equal(updated.Categories, budgets.CategoryIDs{travel.ID}) || updated.ID != budget.ID {
		t.Fatalf("updated = %+v", updated)
	}

	if body := f.do(request{path: budgetsPath, cookies: cookies}).Body.String(); !strings.Contains(body, "<p>The budget for Travel was saved.</p>") {
		t.Fatalf("list does not confirm the change:\n%s", body)
	}

	response = f.do(request{method: http.MethodPost, path: path, body: "currency=USD&amount=1", cookies: cookies, headers: toContent})

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "Choose at least one category.") ||
		strings.Contains(response.Body.String(), " checked>") || f.budgets[0].Amount != 75_25 {
		t.Fatalf("rejected update = %d:\n%s", response.Code, response.Body.String())
	}
}

// Only a confirmed deletion removes the budget; its categories are kept.

func TestDeleteBudget(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	budget := f.addBudget(designID, 100_00, f.addCategory(designID, "Food", "", "green"))
	path := budgetsPath + "/" + uuid.UUID(budget.ID).String()

	response := f.do(request{method: http.MethodPost, path: path + "/delete", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != path || len(f.budgets) != 1 {
		t.Fatalf("unconfirmed delete = %d %q with %d budgets", response.Code, response.Header().Get("Location"), len(f.budgets))
	}

	response = f.do(request{method: http.MethodPost, path: path + "/delete", body: "confirm=1", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != budgetsPath || len(f.budgets) != 0 || len(f.categories) != 1 {
		t.Fatalf("confirmed delete = %d %q with %d budgets and %d categories", response.Code, response.Header().Get("Location"), len(f.budgets), len(f.categories))
	}

	if body := f.do(request{path: budgetsPath, cookies: cookies}).Body.String(); !strings.Contains(body, "<p>The budget for Food was deleted.</p>") {
		t.Fatalf("list does not confirm the deletion:\n%s", body)
	}

	// A page left open after the deletion leads back to the list.
	response = f.do(request{method: http.MethodPost, path: path + "/delete", body: "confirm=1", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != budgetsPath {
		t.Fatalf("repeated delete = %d %q", response.Code, response.Header().Get("Location"))
	}
}

// A category that a budget covers is kept, and the member is told why.

func TestDeleteCategoryInBudget(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "", "green")
	f.addBudget(designID, 100_00, food)

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/categories/" + uuid.UUID(food.ID).String() + "/delete",
		body:    "confirm=1",
		cookies: cookies,
	})

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "A budget covers this category") || len(f.categories) != 1 {
		t.Fatalf("delete of a budgeted category = %d with %d categories:\n%s", response.Code, len(f.categories), response.Body.String())
	}
}

func TestBudgetsPagination(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "", "green")

	for index := range 51 {
		f.addBudget(designID, budgets.Amount(100+index), food)
	}

	first := f.do(request{path: budgetsPath, cookies: cookies}).Body.String()
	second := f.do(request{path: budgetsPath + "?page=2", cookies: cookies}).Body.String()

	if strings.Count(first, `class="budget-list__item"`) != 50 || !strings.Contains(first, `href="`+budgetsPath+`?page=2"`) ||
		!strings.Contains(first, "Page 1 of 2") || strings.Contains(first, ">Previous<") {
		t.Fatalf("first page does not lead to the second:\n%s", first)
	}

	if strings.Count(second, `class="budget-list__item"`) != 1 || !strings.Contains(second, ">1.00 EUR<") ||
		!strings.Contains(second, `href="`+budgetsPath+`" hx-boost`) || strings.Contains(second, ">Next<") {
		t.Fatalf("second page does not lead back to the first:\n%s", second)
	}
}

// A budget form's body: the currency, the amount as typed, and each category
// as a repeated field.

func budgetBody(currency string, amount string, categoryList ...*categories.Category) string {
	values := url.Values{"currency": {currency}, "amount": {amount}}

	for _, category := range categoryList {
		values.Add("categories", uuid.UUID(category.ID).String())
	}

	return values.Encode()
}
