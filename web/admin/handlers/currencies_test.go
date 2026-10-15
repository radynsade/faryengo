package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/users"
)

// Every page of the section needs permission to manage currencies; refused
// requests write nothing.

func TestCurrencyAuthorization(t *testing.T) {
	routes := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/admin/en/currencies", "", http.StatusOK},
		{http.MethodGet, "/admin/en/currencies/create", "", http.StatusOK},
		{http.MethodPost, "/admin/en/currencies/create", "code=GBP&symbol=£", http.StatusSeeOther},
		{http.MethodGet, "/admin/en/currencies/EUR/view", "", http.StatusOK},
		{http.MethodGet, "/admin/en/currencies/EUR/edit", "", http.StatusOK},
		{http.MethodPost, "/admin/en/currencies/EUR/edit", "symbol=€", http.StatusSeeOther},
		{http.MethodPost, "/admin/en/currencies/EUR/delete", "confirm=delete", http.StatusOK},
	}

	for _, tt := range []struct {
		name        string
		permissions users.Permissions
		isSuper     bool
		allowed     bool
	}{
		{"no permissions", nil, false, false},
		{"other permissions", users.Permissions{users.PermissionManageRole, users.PermissionManageLanguage}, false, false},
		{"manage currencies", users.Permissions{users.PermissionManageCurrency}, false, true},
		{"super role", nil, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range routes {
				f := newFixture(t, tt.permissions, tt.isSuper)
				response := f.do(request{method: route.method, path: route.path, body: route.body, cookies: f.signIn(t)})
				want := route.want

				if !tt.allowed {
					want = http.StatusForbidden
				}

				if response.Code != want {
					t.Fatalf("%s %s = %d, want %d: %s", route.method, route.path, response.Code, want, response.Body.String())
				}

				if !tt.allowed && f.writes != 0 {
					t.Fatalf("%s %s wrote despite being forbidden", route.method, route.path)
				}
			}
		})
	}
}

func TestCurrencyNavigationFollowsThePermission(t *testing.T) {
	for _, tt := range []struct {
		name        string
		permissions users.Permissions
		wantLink    bool
	}{
		{"without the permission", users.Permissions{users.PermissionViewRole}, false},
		{"with the permission", users.Permissions{users.PermissionManageCurrency}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.permissions, false)
			home := f.do(request{path: "/admin/en", cookies: f.signIn(t)}).Body.String()

			if strings.Contains(home, `href="/admin/en/currencies"`) != tt.wantLink {
				t.Fatalf("currency link shown = %t, want %t", !tt.wantLink, tt.wantLink)
			}
		})
	}
}

func TestCurrenciesList(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionManageCurrency}, false)
	cookies := f.signIn(t)

	t.Run("lists every currency by code", func(t *testing.T) {
		body := f.do(request{path: "/admin/en/currencies", cookies: cookies}).Body.String()
		eur, usd := strings.Index(body, `id="currency-EUR"`), strings.Index(body, `id="currency-USD"`)

		if eur < 0 || usd < eur || !strings.Contains(body, "Total found: 2") || !strings.Contains(body, "<td>€</td>") {
			t.Fatalf("list = %s", body)
		}

		if !strings.Contains(body, `<a class="panel-menu__link" href="/admin/en/currencies" aria-current="page"`) {
			t.Fatal("the currencies section is not marked active")
		}
	})

	t.Run("empty catalog", func(t *testing.T) {
		saved := f.currencies
		f.currencies = map[currencies.Code]*currencies.Currency{}
		defer func() { f.currencies = saved }()

		if body := f.do(request{path: "/admin/en/currencies", cookies: cookies}).Body.String(); !strings.Contains(body, "No currencies found.") {
			t.Fatalf("empty list = %s", body)
		}
	})

	t.Run("loading failure offers a retry", func(t *testing.T) {
		f.listErr = errors.New("connection refused")
		defer func() { f.listErr = nil }()

		response := f.do(request{path: "/admin/en/currencies", cookies: cookies})
		body := response.Body.String()

		if response.Code != http.StatusInternalServerError || !strings.Contains(body, "Retry") ||
			!strings.Contains(body, "Currencies are temporarily unavailable.") || strings.Contains(body, "No currencies found.") {
			t.Fatalf("failed list = %d: %s", response.Code, body)
		}
	})
}

func TestCurrencyCreateAndUpdate(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionManageCurrency}, false)
	cookies := f.signIn(t)

	// A code typed in lower case or with spaces is accepted.
	response := f.do(request{method: http.MethodPost, path: "/admin/en/currencies/create", body: "code=+brl+&symbol=R%24", cookies: cookies})

	if response.Code != http.StatusSeeOther || f.writes != 1 {
		t.Fatalf("create = %d, writes = %d: %s", response.Code, f.writes, response.Body.String())
	}

	if created := f.currencies["BRL"]; created == nil || created.Symbol != "R$" {
		t.Fatalf("created currency = %+v", created)
	}

	location := "/admin/en/currencies/BRL/view"

	if response.Header().Get("Location") != location {
		t.Fatalf("Location = %q, want %q", response.Header().Get("Location"), location)
	}

	view := f.do(request{path: location, cookies: cookies}).Body.String()

	if !strings.Contains(view, `Currency &#34;BRL&#34; created successfully.`) && !strings.Contains(view, `Currency "BRL" created successfully.`) {
		t.Fatalf("the success message is missing: %s", view)
	}

	// Editing changes only the symbol of the currency the route names.
	response = f.do(request{method: http.MethodPost, path: "/admin/en/currencies/BRL/edit", body: "symbol=%24", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != location || f.currencies["BRL"].Symbol != "$" {
		t.Fatalf("update = %d, currency = %+v", response.Code, f.currencies["BRL"])
	}

	if body := f.do(request{path: location, cookies: cookies}).Body.String(); !strings.Contains(body, "updated successfully") {
		t.Fatal("the update was not confirmed")
	}
}

func TestCurrencyEditShowsTheFixedCode(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionManageCurrency}, false)
	body := f.do(request{path: "/admin/en/currencies/EUR/edit", cookies: f.signIn(t)}).Body.String()

	if strings.Contains(body, `name="code"`) || !strings.Contains(body, `<p class="currency-form__value">EUR</p>`) {
		t.Fatalf("the edit form offers to change the code: %s", body)
	}

	if !strings.Contains(body, `name="symbol"`) || !strings.Contains(body, `value="€"`) {
		t.Fatalf("the edit form lacks the current symbol: %s", body)
	}
}

func TestCurrencyFormValidation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantBody   []string
	}{
		{
			name:       "invalid code and symbol together",
			path:       "/admin/en/currencies/create",
			body:       "code=EU&symbol=",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   []string{`id="currency-code-errors"`, "Enter three uppercase Latin letters, such as EUR.", `id="currency-symbol-errors"`, "This field is required."},
		},
		{
			name:       "symbol without a sign",
			path:       "/admin/en/currencies/create",
			body:       "code=GBP&symbol=.%2F",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   []string{"Use letters, currency signs, and punctuation", `value="GBP"`},
		},
		{
			name:       "symbol too long",
			path:       "/admin/en/currencies/EUR/edit",
			body:       "symbol=Bs.F.S",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   []string{"This value is too long."},
		},
		{
			name:       "taken code",
			path:       "/admin/en/currencies/create",
			body:       "code=EUR&symbol=E",
			wantStatus: http.StatusConflict,
			wantBody:   []string{"A currency with this code already exists."},
		},
		{
			name:       "code on editing",
			path:       "/admin/en/currencies/EUR/edit",
			body:       "code=USD&symbol=%24",
			wantStatus: http.StatusBadRequest,
			wantBody:   []string{"Submit a valid currency form."},
		},
		{
			name:       "unknown field",
			path:       "/admin/en/currencies/create",
			body:       "code=GBP&symbol=£&name=Pound",
			wantStatus: http.StatusBadRequest,
			wantBody:   []string{"Submit a valid currency form."},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, users.Permissions{users.PermissionManageCurrency}, false)
			before := len(f.currencies)
			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: f.signIn(t), headers: partial})
			body := response.Body.String()

			if response.Code != tt.wantStatus || len(f.currencies) != before || f.currencies["EUR"].Symbol != "€" {
				t.Fatalf("status = %d, want %d, currencies = %v", response.Code, tt.wantStatus, f.currencies)
			}

			if response.Header().Get("HX-Retarget") != "#currency-form" || response.Header().Get("HX-Push-Url") != "false" || strings.Contains(body, `<head hx-head`) {
				t.Fatalf("the failure does not replace only the form region: %v", response.Header())
			}

			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Fatalf("body does not contain %q: %s", want, body)
				}
			}
		})
	}
}

func TestCurrencyDelete(t *testing.T) {
	for _, tt := range []struct {
		name       string
		code       string
		wantStatus int
		wantBody   string
	}{
		{"existing currency", "USD", http.StatusOK, ""},
		{"currency used by accounts", "EUR", http.StatusConflict, "This currency is used by accounts."},
		{"missing currency", "GBP", http.StatusNotFound, "Currency not found."},
		{"invalid code", "usd", http.StatusBadRequest, "Submit a valid currency form."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, users.Permissions{users.PermissionManageCurrency}, false)
			f.usedCurrency = "EUR"
			cookies := f.signIn(t)
			response := f.do(request{
				method:  http.MethodPost,
				path:    "/admin/en/currencies/" + tt.code + "/delete",
				body:    "confirm=delete",
				cookies: cookies,
				headers: partial,
			})
			body := response.Body.String()

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, body)
			}

			if tt.wantStatus != http.StatusOK {
				if !strings.Contains(body, `<dialog id="confirm-delete"`) || !strings.Contains(body, tt.wantBody) || strings.Contains(body, "<html") {
					t.Fatalf("the failure is not shown in the dialog: %s", body)
				}

				return
			}

			var location map[string]string

			must(t, json.Unmarshal([]byte(response.Header().Get("HX-Location")), &location))

			if location["path"] != "/admin/en/currencies" || location["target"] != "#panel-main" || f.currencies[currencies.Code(tt.code)] != nil {
				t.Fatalf("HX-Location = %v, currencies = %v", location, f.currencies)
			}

			if list := f.do(request{path: "/admin/en/currencies", cookies: cookies}).Body.String(); !strings.Contains(list, "deleted successfully") {
				t.Fatal("the deletion was not confirmed on the next page")
			}
		})
	}
}

func TestCurrencyNotFound(t *testing.T) {
	f := newFixture(t, users.Permissions{users.PermissionManageCurrency}, false)
	cookies := f.signIn(t)

	for path, want := range map[string]int{
		"/admin/en/currencies/GBP/view":  http.StatusNotFound,
		"/admin/en/currencies/GBP/edit":  http.StatusNotFound,
		"/admin/en/currencies/eur/view":  http.StatusBadRequest,
		"/admin/en/currencies/EURO/edit": http.StatusBadRequest,
	} {
		if response := f.do(request{path: path, cookies: cookies}); response.Code != want {
			t.Fatalf("GET %s = %d, want %d", path, response.Code, want)
		}
	}
}

// Language links on a currency page lead to the same currency; a deletion's
// links lead to the currency it started from.

func TestCurrencyLanguageLinks(t *testing.T) {
	f := newFixture(t, nil, true)
	body := f.do(request{path: "/admin/en/currencies/EUR/edit", cookies: f.signIn(t)}).Body.String()

	for _, want := range []string{`href="/admin/lv/currencies/EUR/edit"`, `href="/admin/ru/currencies/EUR/edit"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s", want)
		}
	}

	lv := f.do(request{path: "/admin/lv/currencies/EUR/view", cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(lv, "Rediģēt valūtu") || !strings.Contains(lv, "Valūtas") {
		t.Fatal("the currency view is not translated")
	}
}
