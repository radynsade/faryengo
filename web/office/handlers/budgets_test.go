package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/budgets"
	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/period"
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

// A budget's period is typed in the time zone the form sends, and kept in it.

func TestCreateBudgetWithPeriod(t *testing.T) {
	riga, err := time.LoadLocation("Europe/Riga")

	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}

	for _, tt := range []struct {
		name   string
		fields url.Values
		want   period.Period
	}{
		{
			name: "every month until a date",
			fields: url.Values{
				"period_repeat": {"monthly"}, "period_from": {"2026-01-01T00:00"}, "period_to": {"2026-02-01T00:00"},
				"period_until": {"2027-01-01T00:00"}, "time_zone": {"Europe/Riga"},
				// Fields a monthly repeat does not use are ignored.
				"period_every": {"nonsense"}, "period_every_unit": {"weeks"},
			},
			want: period.NewPeriod(
				new(time.Date(2026, 1, 1, 0, 0, 0, 0, riga)),
				new(time.Date(2026, 2, 1, 0, 0, 0, 0, riga)),
				period.NewRepeat(period.RepeatMonthly, 0, new(time.Date(2027, 1, 1, 0, 0, 0, 0, riga))),
			),
		},
		{
			name: "every two days",
			fields: url.Values{
				"period_repeat": {"every"}, "period_from": {"2026-01-01T13:00"}, "period_to": {"2026-01-01T15:20"},
				"period_every": {"2"}, "period_every_unit": {"days"}, "time_zone": {"Europe/Riga"},
			},
			want: period.NewPeriod(
				new(time.Date(2026, 1, 1, 13, 0, 0, 0, riga)),
				new(time.Date(2026, 1, 1, 15, 20, 0, 0, riga)),
				period.NewRepeat(period.RepeatEvery, 48*time.Hour, nil),
			),
		},
		{
			name: "one window in UTC without a zone",
			fields: url.Values{
				"period_repeat": {""}, "period_from": {"2026-03-01T00:00"}, "period_to": {""},
				// A repetition's end is ignored when nothing repeats.
				"period_until": {"2026-01-01T00:00"},
			},
			want: period.NewPeriod(new(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)), nil, period.Repeat{}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			food := f.addCategory(designID, "Food", "", "green")
			body := budgetBody("EUR", "100", food) + "&" + tt.fields.Encode()

			response := f.do(request{method: http.MethodPost, path: budgetsPath, body: body, cookies: f.signIn(t)})

			if response.Code != http.StatusSeeOther || len(f.budgets) != 1 {
				t.Fatalf("create = %d with %d budgets:\n%s", response.Code, len(f.budgets), response.Body.String())
			}

			got := f.budgets[0].Period

			if !sameTime(got.From, tt.want.From) || !sameTime(got.To, tt.want.To) || got.Repeat.Kind != tt.want.Repeat.Kind ||
				got.Repeat.Every != tt.want.Repeat.Every || !sameTime(got.Repeat.Until, tt.want.Repeat.Until) ||
				got.Location().String() != tt.want.Location().String() {
				t.Fatalf("period = %+v in %s, want %+v in %s", got, got.Location(), tt.want, tt.want.Location())
			}
		})
	}
}

// A rejected period keeps what was typed and explains which field is wrong.

func TestCreateBudgetPeriodRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fields url.Values
		status int
		wants  []string
	}{
		{
			name:   "repeating without an end",
			fields: url.Values{"period_repeat": {"monthly"}, "period_from": {"2026-01-01T00:00"}},
			status: http.StatusUnprocessableEntity,
			wants: []string{
				`<p class="field__error" id="period-to-error">A repeating period needs a start and an end.</p>`,
				`value="2026-01-01T00:00"`,
			},
		},
		{
			name:   "end before start",
			fields: url.Values{"period_from": {"2026-02-01T00:00"}, "period_to": {"2026-01-01T00:00"}},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="period-to-error">The end must be after the start.</p>`},
		},
		{
			name:   "window longer than a week",
			fields: url.Values{"period_repeat": {"weekly"}, "period_from": {"2026-01-01T00:00"}, "period_to": {"2026-01-09T00:00"}},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="period-to-error">The window must end before its next repeat starts.</p>`},
		},
		{
			name: "stops before it starts",
			fields: url.Values{
				"period_repeat": {"daily"}, "period_from": {"2026-01-02T00:00"}, "period_to": {"2026-01-03T00:00"},
				"period_until": {"2026-01-01T00:00"},
			},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="period-until-error">Repeating must stop after the start.</p>`},
		},
		{
			name:   "malformed start",
			fields: url.Values{"period_from": {"yesterday"}},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="period-from-error">This value is not valid.</p>`, `value="yesterday"`},
		},
		{
			name: "interval that is not a number",
			fields: url.Values{
				"period_repeat": {"every"}, "period_from": {"2026-01-01T00:00"}, "period_to": {"2026-01-02T00:00"},
				"period_every": {"0"}, "period_every_unit": {"days"},
			},
			status: http.StatusUnprocessableEntity,
			wants: []string{
				`<p class="field__error" id="period-every-error">Enter a whole number from 1 to 99999.</p>`,
				`<div class="form__row" data-period-every>`,
			},
		},
		{
			name:   "unknown repeat",
			fields: url.Values{"period_repeat": {"hourly"}},
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="period-repeat-error">Choose one of the options.</p>`},
		},
		{
			name:   "unknown time zone",
			fields: url.Values{"period_from": {"2026-01-01T00:00"}, "time_zone": {"Mars/Olympus_Mons"}},
			status: http.StatusBadRequest,
			wants:  []string{`<div class="form-alert" role="alert">`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			food := f.addCategory(designID, "Food", "", "green")
			body := budgetBody("EUR", "100", food) + "&" + tt.fields.Encode()

			response := f.do(request{method: http.MethodPost, path: budgetsPath, body: body, cookies: f.signIn(t), headers: toContent})
			got := response.Body.String()

			if response.Code != tt.status || !strings.HasPrefix(got, `<form id="budget-form"`) {
				t.Fatalf("create = %d, want %d with only the form:\n%s", response.Code, tt.status, got)
			}

			for _, want := range tt.wants {
				if !strings.Contains(got, want) {
					t.Fatalf("form lacks %s:\n%s", want, got)
				}
			}

			if len(f.budgets) != 0 {
				t.Fatalf("budgets = %+v, want none", f.budgets)
			}
		})
	}
}

// A new budget applies at any time, its selector shows only what that uses,
// and the browser fills in the time zone.

func TestNewBudgetPeriodSelector(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: budgetsPath + "/new", cookies: f.signIn(t)}).Body.String()

	for _, want := range []string{
		`<fieldset class="period-selector" data-period-selector data-period-fits=`,
		`<button class="period-selector__preset" type="button" data-period-preset="monthly">Every month</button>`,
		`<option value selected>Does not repeat</option>`,
		`<option value="custom" selected>Custom range</option>`,
		`<div class="form__row" data-period-window-show="custom">`,
		`<div data-period-window-show="day" hidden>`,
		`<div class="form__row" data-period-window-show="month year" hidden>`,
		`value="` + time.Now().Format(time.DateOnly) + `"`,
		`<input class="field__input" id="period-from" name="period_from" type="datetime-local"`,
		`<div class="form__row" data-period-every hidden>`,
		`<div data-period-repeating hidden>`,
		`<input type="hidden" name="time_zone" data-period-time-zone>`,
		"Times are in your device's time zone.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("selector lacks %s:\n%s", want, body)
		}
	}
}

// An existing budget's period is shown in its own time zone, which it keeps,
// and the list describes it.

func TestEditBudgetPeriod(t *testing.T) {
	riga, err := time.LoadLocation("Europe/Riga")

	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}

	f := newFixture(t)
	cookies := f.signIn(t)
	budget := f.addBudget(designID, 100_00, f.addCategory(designID, "Food", "", "green"))
	budget.Period = period.NewPeriod(
		new(time.Date(2026, 1, 1, 13, 0, 0, 0, riga)),
		new(time.Date(2026, 1, 1, 15, 20, 0, 0, riga)),
		period.NewRepeat(period.RepeatEvery, 90*time.Minute, new(time.Date(2026, 2, 1, 0, 0, 0, 0, riga))),
	)
	path := budgetsPath + "/" + uuid.UUID(budget.ID).String()

	body := f.do(request{path: path, cookies: cookies}).Body.String()

	for _, want := range []string{
		`<option value="every" selected>At a custom interval</option>`,
		`value="2026-01-01T13:00"`,
		`value="2026-01-01T15:20"`,
		`value="2026-02-01T00:00"`,
		`value="90"`,
		`<option value="minutes" selected>minutes</option>`,
		`<div class="form__row" data-period-every>`,
		`<input type="hidden" name="time_zone" value="Europe/Riga" data-period-time-zone>`,
		"Times are in the Europe/Riga time zone.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit page lacks %s:\n%s", want, body)
		}
	}

	list := f.do(request{path: budgetsPath, cookies: cookies}).Body.String()

	if !strings.Contains(list, `<span class="budget-list__period">Every 90 minutes, first 2026-01-01 13:00 – 2026-01-01 15:20, until 2026-02-01 00:00</span>`) {
		t.Fatalf("list does not describe the period:\n%s", list)
	}
}

func TestBudgetPeriodSummaries(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	food := f.addCategory(designID, "Food", "", "green")

	for _, tt := range []struct {
		period period.Period
		want   string
	}{
		{period.Period{}, "Any time"},
		{period.NewPeriod(new(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)), nil, period.Repeat{}), "From 2026-03-01"},
		{period.NewPeriod(nil, new(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)), period.Repeat{}), "Before 2026-03-01"},
		{
			period.NewPeriod(
				new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
				new(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
				period.NewRepeat(period.RepeatMonthly, 0, nil),
			),
			"Every month, first January 2026",
		},
		{
			period.NewPeriod(
				new(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)),
				new(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)),
				period.NewRepeat(period.RepeatWeekly, 0, new(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))),
			),
			"Every week, first 2026-01-05, until 2026-04-01",
		},
		{
			period.NewPeriod(new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), new(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), period.Repeat{}),
			"2026",
		},
		{
			period.NewPeriod(new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), new(time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)), period.Repeat{}),
			"2026-01-01 – 2026-01-08",
		},
	} {
		f.budgets = nil
		f.addBudget(designID, 100_00, food).Period = tt.period

		if body := f.do(request{path: budgetsPath, cookies: cookies}).Body.String(); !strings.Contains(body, `<span class="budget-list__period">`+tt.want+`</span>`) {
			t.Fatalf("list lacks %q:\n%s", tt.want, body)
		}
	}
}

func sameTime(a *time.Time, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

// A whole day, month, or year is chosen without typing its bounds, and becomes
// the range from its first moment to the first moment of the next one.

func TestCreateBudgetWithWholeWindow(t *testing.T) {
	riga, err := time.LoadLocation("Europe/Riga")

	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}

	for _, tt := range []struct {
		name     string
		fields   url.Values
		wantFrom time.Time
		wantTo   time.Time
	}{
		{
			name: "day",
			fields: url.Values{
				"period_window": {"day"}, "period_day": {"2026-03-29"}, "period_repeat": {"weekly"}, "time_zone": {"Europe/Riga"},
				// The custom range is ignored.
				"period_from": {"nonsense"},
			},
			// The clocks move forward that night, so the day is 23 hours long.
			wantFrom: time.Date(2026, 3, 29, 0, 0, 0, 0, riga),
			wantTo:   time.Date(2026, 3, 30, 0, 0, 0, 0, riga),
		},
		{
			name:     "month",
			fields:   url.Values{"period_window": {"month"}, "period_month": {"2"}, "period_year": {"2028"}, "period_repeat": {"monthly"}, "time_zone": {"Europe/Riga"}},
			wantFrom: time.Date(2028, 2, 1, 0, 0, 0, 0, riga),
			wantTo:   time.Date(2028, 3, 1, 0, 0, 0, 0, riga),
		},
		{
			name: "year",
			fields: url.Values{
				"period_window": {"year"}, "period_year": {"2027"}, "time_zone": {"Europe/Riga"},
				// The month is only for a whole month.
				"period_month": {"13"},
			},
			wantFrom: time.Date(2027, 1, 1, 0, 0, 0, 0, riga),
			wantTo:   time.Date(2028, 1, 1, 0, 0, 0, 0, riga),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			food := f.addCategory(designID, "Food", "", "green")
			body := budgetBody("EUR", "100", food) + "&" + tt.fields.Encode()

			response := f.do(request{method: http.MethodPost, path: budgetsPath, body: body, cookies: f.signIn(t)})

			if response.Code != http.StatusSeeOther || len(f.budgets) != 1 {
				t.Fatalf("create = %d with %d budgets:\n%s", response.Code, len(f.budgets), response.Body.String())
			}

			got := f.budgets[0].Period

			if !sameTime(got.From, &tt.wantFrom) || !sameTime(got.To, &tt.wantTo) || got.Location().String() != "Europe/Riga" {
				t.Fatalf("window = [%s, %s) in %s, want [%s, %s)", got.From, got.To, got.Location(), tt.wantFrom, tt.wantTo)
			}
		})
	}
}

func TestCreateBudgetWholeWindowRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fields url.Values
		want   string
	}{
		{"no day", url.Values{"period_window": {"day"}, "period_day": {""}}, `<p class="field__error" id="period-day-error">Please fill in this field.</p>`},
		{"malformed day", url.Values{"period_window": {"day"}, "period_day": {"2026-02-30"}}, `<p class="field__error" id="period-day-error">This value is not valid.</p>`},
		{"unknown month", url.Values{"period_window": {"month"}, "period_month": {"13"}, "period_year": {"2026"}}, `<p class="field__error" id="period-month-error">Choose one of the options.</p>`},
		{"no year", url.Values{"period_window": {"year"}, "period_year": {""}}, `<p class="field__error" id="period-year-error">Please fill in this field.</p>`},
		{"year out of range", url.Values{"period_window": {"year"}, "period_year": {"10000"}}, `<p class="field__error" id="period-year-error">Enter a year from 1 to 9999.</p>`},
		{"unknown window", url.Values{"period_window": {"week"}}, `<p class="field__error" id="period-window-error">Choose one of the options.</p>`},
		{
			"month every week",
			url.Values{"period_window": {"month"}, "period_month": {"1"}, "period_year": {"2026"}, "period_repeat": {"weekly"}},
			`<p class="field__error" id="period-repeat-error">This repeat does not fit the chosen window.</p>`,
		},
		{
			"month every day",
			url.Values{"period_window": {"month"}, "period_month": {"1"}, "period_year": {"2026"}, "period_repeat": {"daily"}},
			`<p class="field__error" id="period-repeat-error">This repeat does not fit the chosen window.</p>`,
		},
		{
			"year every month",
			url.Values{"period_window": {"year"}, "period_year": {"2026"}, "period_repeat": {"monthly"}},
			`<p class="field__error" id="period-repeat-error">This repeat does not fit the chosen window.</p>`,
		},
		{
			// A long enough interval would be a valid period, but it moves the
			// window off the calendar's months.
			"month at a custom interval",
			url.Values{
				"period_window": {"month"}, "period_month": {"1"}, "period_year": {"2026"},
				"period_repeat": {"every"}, "period_every": {"40"}, "period_every_unit": {"days"},
			},
			`<p class="field__error" id="period-repeat-error">This repeat does not fit the chosen window.</p>`,
		},
		{
			"day at a custom interval",
			url.Values{"period_window": {"day"}, "period_day": {"2026-01-01"}, "period_repeat": {"every"}, "period_every": {"2"}, "period_every_unit": {"days"}},
			`<p class="field__error" id="period-repeat-error">This repeat does not fit the chosen window.</p>`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			food := f.addCategory(designID, "Food", "", "green")
			body := budgetBody("EUR", "100", food) + "&" + tt.fields.Encode()

			response := f.do(request{method: http.MethodPost, path: budgetsPath, body: body, cookies: f.signIn(t), headers: toContent})

			if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), tt.want) || len(f.budgets) != 0 {
				t.Fatalf("create = %d with %d budgets, want %s:\n%s", response.Code, len(f.budgets), tt.want, response.Body.String())
			}

			if window := tt.fields.Get("period_window"); window != "week" &&
				!strings.Contains(response.Body.String(), `<option value="`+window+`" selected>`) {
				t.Fatalf("form does not keep the %s window:\n%s", window, response.Body.String())
			}
		})
	}
}

// An existing window that is a whole day, month, or year reopens as one.

func TestEditBudgetWholeWindow(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	budget := f.addBudget(designID, 100_00, f.addCategory(designID, "Food", "", "green"))
	path := budgetsPath + "/" + uuid.UUID(budget.ID).String()

	for _, tt := range []struct {
		from, to time.Time
		wants    []string
	}{
		{
			time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			[]string{`<option value="month" selected>Whole month</option>`, `<option value="10" selected>October</option>`, `value="2026"`,
				`<div class="form__row" data-period-window-show="custom" hidden>`},
		},
		{
			time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
			[]string{`<option value="day" selected>Whole day</option>`, `value="2026-10-15"`},
		},
		{
			time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			[]string{`<option value="year" selected>Whole year</option>`, `value="2027"`},
		},
		{
			time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC),
			[]string{`<option value="custom" selected>Custom range</option>`, `value="2026-10-15T09:00"`},
		},
	} {
		budget.Period = period.NewPeriod(&tt.from, &tt.to, period.Repeat{})
		body := f.do(request{path: path, cookies: cookies}).Body.String()

		for _, want := range tt.wants {
			if !strings.Contains(body, want) {
				t.Fatalf("edit page for [%s, %s) lacks %s:\n%s", tt.from, tt.to, want, body)
			}
		}
	}
}

// A whole window offers only the repeats that fit it; the others stay in the
// list, disabled, and the script learns the same rule.

func TestPeriodSelectorRepeatsFitTheWindow(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	body := f.do(request{path: budgetsPath + "/new", cookies: cookies}).Body.String()

	if !strings.Contains(body, `data-period-fits='{"day":["daily","","weekly","monthly","yearly"],"month":["monthly","","yearly"],"year":["yearly",""]}'`) ||
		strings.Contains(body, `<option value="daily" disabled>`) {
		t.Fatalf("new selector lacks the rule, or limits a custom range:\n%s", body)
	}

	budget := f.addBudget(designID, 100_00, f.addCategory(designID, "Food", "", "green"))
	path := budgetsPath + "/" + uuid.UUID(budget.ID).String()
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	budget.Period = period.NewPeriod(&from, &to, period.NewRepeat(period.RepeatMonthly, 0, nil))

	body = f.do(request{path: path, cookies: cookies}).Body.String()

	for _, want := range []string{
		`<option value="month" selected>Whole month</option>`,
		`<option value="daily" disabled>Every day</option>`,
		`<option value="weekly" disabled>Every week</option>`,
		`<option value="every" disabled>At a custom interval</option>`,
		`<option value="monthly" selected>Every month</option>`,
		`data-value="daily" aria-selected="false" aria-disabled="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit page lacks %s:\n%s", want, body)
		}
	}

	// A whole month at a custom interval, saved before the rule, reopens as a
	// custom range, which it may still be.
	budget.Period = period.NewPeriod(&from, &to, period.NewRepeat(period.RepeatEvery, 40*24*time.Hour, nil))
	body = f.do(request{path: path, cookies: cookies}).Body.String()

	if !strings.Contains(body, `<option value="custom" selected>Custom range</option>`) || strings.Contains(body, `<option value="every" disabled`) {
		t.Fatalf("edit page does not reopen the window as a custom range:\n%s", body)
	}
}
