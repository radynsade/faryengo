package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/categories"
	"github.com/radynsade/faryengo/internal/workspaces"
)

const categoriesPath = designPath + "/categories"

func TestCategoriesPage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	groceries := f.addCategory(designID, "Groceries", "shopping-cart", categories.ColorGreen)
	f.addCategory(designID, "Bills", "", categories.ColorBlue)
	f.addCategory(familyID, "Family only", "", categories.ColorRed)

	response := f.do(request{path: categoriesPath, cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	groceriesID := uuid.UUID(groceries.ID).String()

	for _, want := range []string{
		"<title>Categories · Faryen</title>",
		`<h1 id="page-title">Categories</h1>`,
		`href="` + categoriesPath + `" aria-current="page"`,
		`<a class="button" href="` + categoriesPath + `/new"`,
		`<span class="visually-hidden">Shopping cart</span>`,
		`<li id="category-` + groceriesID + `" class="category-list__item"><a class="category-list__link" href="` + categoriesPath + "/" + groceriesID + `"`,
		`class="category-icon category-color--green category-icon--shopping-cart"`,
		`class="category-icon category-color--blue category-icon--empty"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Index(body, ">Bills<") > strings.Index(body, ">Groceries<") {
		t.Fatalf("categories are not in name order:\n%s", body)
	}

	if strings.Contains(body, "Family only") || strings.Contains(body, `class="pagination"`) || strings.Contains(body, `id="category-form"`) {
		t.Fatalf("body shows another workspace's category, a pagination, or the form:\n%s", body)
	}
}

// The form that adds a category has its own page, which leads back to the
// list.

func TestNewCategoryPage(t *testing.T) {
	f := newFixture(t)
	response := f.do(request{path: categoriesPath + "/new", cookies: f.signIn(t)})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	for _, want := range []string{
		"<title>New category · Faryen</title>",
		`<h1 id="page-title">New category</h1>`,
		`href="` + categoriesPath + `" aria-current="page"`,
		`<form id="category-form" class="form" method="post" action="` + categoriesPath + `" novalidate`,
		`<input type="radio" id="category-color-green" name="color" value="green" checked>`,
		`<input type="radio" id="category-icon-none" name="icon" value checked>`,
		`<a class="form__cancel" href="` + categoriesPath + `"`,
		">Add category</button>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "category-list") {
		t.Fatalf("new category page shows the list:\n%s", body)
	}
}

func TestCategoriesEmpty(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: categoriesPath, cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(body, `<p class="category-list__empty">There are no categories yet.</p>`) || strings.Contains(body, "category-list__item") {
		t.Fatalf("empty list is not explained:\n%s", body)
	}
}

func TestCategoriesUnavailable(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.categoryErr = errors.New("storage unavailable")

	response := f.do(request{path: categoriesPath, cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK || !strings.Contains(body, "Categories are unavailable right now.") || !strings.Contains(body, `href="`+categoriesPath+`/new"`) {
		t.Fatalf("unavailable list = %d:\n%s", response.Code, body)
	}
}

// A viewer sees the categories without ways to change them, and is refused
// every change.

func TestCategoriesAsViewer(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	category := f.addCategory(designID, "Groceries", "", categories.ColorGreen)
	f.setRole(designID, workspaces.RoleViewer)

	body := f.do(request{path: categoriesPath, cookies: cookies}).Body.String()

	if !strings.Contains(body, "Only editors and owners can change categories.") || !strings.Contains(body, ">Groceries<") {
		t.Fatalf("viewer list lacks the categories:\n%s", body)
	}

	for _, unwanted := range []string{`id="category-form"`, `class="button"`, categoriesPath + "/"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("viewer list contains %s:\n%s", unwanted, body)
		}
	}

	categoryPath := categoriesPath + "/" + uuid.UUID(category.ID).String()

	for _, r := range []request{
		{path: categoryPath},
		{path: categoriesPath + "/new"},
		{method: http.MethodPost, path: categoriesPath, body: "name=Rent&color=red"},
		{method: http.MethodPost, path: categoryPath, body: "name=Rent&color=red"},
		{method: http.MethodPost, path: categoryPath + "/delete", body: "confirm=1"},
	} {
		r.cookies = cookies

		if response := f.do(r); response.Code != http.StatusForbidden {
			t.Fatalf("%s %s as a viewer = %d", r.method, r.path, response.Code)
		}
	}

	if len(f.categories) != 1 || f.categories[0].Name != "Groceries" {
		t.Fatalf("categories changed: %+v", f.categories)
	}
}

// Another workspace's categories, and those of a workspace the user does not
// belong to, are not found.

func TestCategoriesNotFound(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	family := f.addCategory(familyID, "Family only", "", categories.ColorRed)
	other := f.addCategory(otherID, "Someone else's", "", categories.ColorRed)

	for _, path := range []string{
		categoriesPath + "/" + uuid.UUID(family.ID).String(),
		categoriesPath + "/" + uuid.UUID(other.ID).String(),
		categoriesPath + "/01920000-0000-7000-8000-0000000000ff",
		categoriesPath + "/not-a-uuid",
		"/office/en/workspaces/" + otherID + "/categories",
		"/office/en/workspaces/" + otherID + "/categories/" + uuid.UUID(other.ID).String(),
	} {
		response := f.do(request{path: path, cookies: cookies})

		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "only") || strings.Contains(response.Body.String(), "Someone") {
			t.Fatalf("GET %s = %d:\n%s", path, response.Code, response.Body.String())
		}
	}

	response := f.do(request{
		method:  http.MethodPost,
		path:    categoriesPath + "/" + uuid.UUID(family.ID).String(),
		body:    "name=Mine&color=green",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != categoriesPath || family.Name != "Family only" {
		t.Fatalf("update of another workspace's category = %d %q, name %q", response.Code, response.Header().Get("Location"), family.Name)
	}

	if response := f.do(request{path: categoriesPath}); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("categories as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestCreateCategory(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    categoriesPath,
		body:    "name=" + url.QueryEscape("  Transport ") + "&icon=bus&color=blue",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != categoriesPath {
		t.Fatalf("create = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if len(f.categories) != 1 {
		t.Fatalf("categories = %+v, want one", f.categories)
	}

	created := f.categories[0]

	if created.Name != "Transport" || created.Icon == nil || *created.Icon != categories.IconBus || created.Color != categories.ColorBlue ||
		created.WorkspaceID != workspaces.WorkspaceID(uuid.MustParse(designID)) {
		t.Fatalf("created = %+v", created)
	}

	body := f.do(request{path: categoriesPath, cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Category “Transport” was added.</p>") || !strings.Contains(body, "category-icon--bus") {
		t.Fatalf("list does not confirm the new category:\n%s", body)
	}
}

// A rejected category replaces only the form, keeping what was typed.

func TestCreateCategoryRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   string
		status int
		wants  []string
	}{
		{
			name:   "blank name",
			body:   "name=++&color=green",
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="category-name-error">Please fill in this field.</p>`},
		},
		{
			name:   "long name",
			body:   "name=" + strings.Repeat("a", categories.MaxCategoryNameLength+1) + "&color=green",
			status: http.StatusUnprocessableEntity,
			wants:  []string{fmt.Sprintf("Use at most %d characters.", categories.MaxCategoryNameLength)},
		},
		{
			name:   "missing color and unknown icon",
			body:   "name=Rent&icon=rocket",
			status: http.StatusUnprocessableEntity,
			wants: []string{
				`<p class="field__error" id="category-color-error">Choose one of the options.</p>`,
				`<p class="field__error" id="category-icon-error">`,
				`value="Rent"`,
			},
		},
		{
			name:   "unknown field",
			body:   "name=Rent&color=green&budget=1",
			status: http.StatusBadRequest,
			wants:  []string{`<div class="form-alert" role="alert">`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			response := f.do(request{method: http.MethodPost, path: categoriesPath, body: tt.body, cookies: f.signIn(t), headers: toContent})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("HX-Retarget") != "#category-form" || !strings.HasPrefix(body, `<form id="category-form"`) {
				t.Fatalf("create = %d %v, want %d with only the form:\n%s", response.Code, response.Header(), tt.status, body)
			}

			for _, want := range tt.wants {
				if !strings.Contains(body, want) {
					t.Fatalf("form lacks %s:\n%s", want, body)
				}
			}

			if len(f.categories) != 0 {
				t.Fatalf("categories = %+v, want none", f.categories)
			}
		})
	}
}

func TestEditCategory(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	category := f.addCategory(designID, "Groceries", "shopping-cart", categories.ColorGreen)
	path := categoriesPath + "/" + uuid.UUID(category.ID).String()

	body := f.do(request{path: path, cookies: cookies}).Body.String()

	for _, want := range []string{
		"<title>Groceries · Faryen</title>",
		`href="` + categoriesPath + `" aria-current="page"`,
		`<form id="category-form" class="form" method="post" action="` + path + `" novalidate`,
		`value="Groceries"`,
		`<input type="radio" id="category-icon-shopping-cart" name="icon" value="shopping-cart" checked>`,
		`<input type="radio" id="category-color-green" name="color" value="green" checked>`,
		`<form id="category-delete-form" class="danger-zone" method="post" action="` + path + `/delete" data-confirm-always`,
		"“Groceries” will be deleted. This cannot be undone.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	response := f.do(request{method: http.MethodPost, path: path, body: "name=Food&icon=&color=orange", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != categoriesPath {
		t.Fatalf("update = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if updated := f.categories[0]; updated.Name != "Food" || updated.Icon != nil || updated.Color != categories.ColorOrange || updated.ID != category.ID {
		t.Fatalf("updated = %+v", updated)
	}

	if body := f.do(request{path: categoriesPath, cookies: cookies}).Body.String(); !strings.Contains(body, "<p>Category “Food” was saved.</p>") {
		t.Fatalf("list does not confirm the change:\n%s", body)
	}

	response = f.do(request{method: http.MethodPost, path: path, body: "name=&color=orange", cookies: cookies, headers: toContent})

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "Please fill in this field.") || f.categories[0].Name != "Food" {
		t.Fatalf("rejected update = %d:\n%s", response.Code, response.Body.String())
	}
}

// Only a confirmed deletion removes the category.

func TestDeleteCategory(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	category := f.addCategory(designID, "Groceries", "", categories.ColorGreen)
	path := categoriesPath + "/" + uuid.UUID(category.ID).String()

	response := f.do(request{method: http.MethodPost, path: path + "/delete", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != path || len(f.categories) != 1 {
		t.Fatalf("unconfirmed delete = %d %q with %d categories", response.Code, response.Header().Get("Location"), len(f.categories))
	}

	response = f.do(request{method: http.MethodPost, path: path + "/delete", body: "confirm=1", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != categoriesPath || len(f.categories) != 0 {
		t.Fatalf("confirmed delete = %d %q with %d categories", response.Code, response.Header().Get("Location"), len(f.categories))
	}

	if body := f.do(request{path: categoriesPath, cookies: cookies}).Body.String(); !strings.Contains(body, "<p>Category “Groceries” was deleted.</p>") {
		t.Fatalf("list does not confirm the deletion:\n%s", body)
	}

	// A page left open after the deletion leads back to the list.
	response = f.do(request{method: http.MethodPost, path: path + "/delete", body: "confirm=1", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != categoriesPath {
		t.Fatalf("repeated delete = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestCategoriesPagination(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	for index := range 51 {
		f.addCategory(designID, fmt.Sprintf("Category %02d", index), "", categories.ColorGrey)
	}

	first := f.do(request{path: categoriesPath, cookies: cookies}).Body.String()
	second := f.do(request{path: categoriesPath + "?page=2", cookies: cookies}).Body.String()

	if strings.Count(first, `class="category-list__item"`) != 50 || !strings.Contains(first, `href="`+categoriesPath+`?page=2"`) ||
		!strings.Contains(first, "Page 1 of 2") || strings.Contains(first, ">Previous<") {
		t.Fatalf("first page does not lead to the second:\n%s", first)
	}

	if strings.Count(second, `class="category-list__item"`) != 1 || !strings.Contains(second, ">Category 50<") ||
		!strings.Contains(second, `href="`+categoriesPath+`" hx-boost`) || strings.Contains(second, ">Next<") {
		t.Fatalf("second page does not lead back to the first:\n%s", second)
	}
}

// The role is changed in place for the signed-in User.

func (f *fixture) setRole(workspaceID string, role workspaces.Role) {
	id := workspaces.WorkspaceID(uuid.MustParse(workspaceID))
	f.members[id][f.memberIndex(id, f.user.ID)].Role = role
}
