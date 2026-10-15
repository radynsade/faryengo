package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/accounts"
	"github.com/radynsade/faryengo/internal/workspaces"
)

const accountsPath = designPath + "/accounts"

func TestAccountsPage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	cash := f.addAccount(designID, "Cash", &f.user.ID)
	f.addAccount(designID, "Bank", nil)
	f.addAccount(designID, "Old card", &f.others[0].ID)
	f.addAccount(familyID, "Family only", nil)

	response := f.do(request{path: accountsPath, cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	cashID := uuid.UUID(cash.ID).String()

	for _, want := range []string{
		"<title>Accounts · Faryen</title>",
		`<h1 id="page-title">Accounts</h1>`,
		`href="` + accountsPath + `" aria-current="page"`,
		`<a class="button" href="` + accountsPath + `/new"`,
		`<li id="account-` + cashID + `" class="account-list__item"><a class="account-list__link" href="` + accountsPath + "/" + cashID + `"`,
		`<span class="account-list__member">Ada Lovelace</span>`,
		`<span class="account-list__member">No member</span>`,
		`<span class="account-list__member">Former member</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Index(body, ">Bank<") > strings.Index(body, ">Cash<") {
		t.Fatalf("accounts are not in name order:\n%s", body)
	}

	if strings.Contains(body, "Family only") || strings.Contains(body, `class="pagination"`) || strings.Contains(body, `id="account-form"`) ||
		strings.Count(body, `#accounts"`) != 1 {
		t.Fatalf("body shows another workspace's account, a pagination, the form, or an icon beside the menu's:\n%s", body)
	}
}

// The menu leads to the accounts from every section of the workspace.

func TestAccountsInSidebar(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: designPath, cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(body, `href="`+accountsPath+`"`) || !strings.Contains(body, ">Accounts<") || !strings.Contains(body, `#accounts"`) {
		t.Fatalf("menu lacks the accounts:\n%s", body)
	}
}

// The form that adds an account has its own page, which offers the
// workspace's members and leads back to the list.

func TestNewAccountPage(t *testing.T) {
	f := newFixture(t)
	response := f.do(request{path: accountsPath + "/new", cookies: f.signIn(t)})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	for _, want := range []string{
		"<title>New account · Faryen</title>",
		`<div id="messages" class="messages"></div>`,
		`href="` + accountsPath + `" aria-current="page"`,
		`<form id="account-form" class="form" method="post" action="` + accountsPath + `" novalidate`,
		`name="user"`,
		`<option value selected>No member</option>`,
		`<option value selected>Choose a currency</option>`,
		`<option value="EUR">EUR (€)</option>`,
		`<option value="USD">USD ($)</option>`,
		`<option value="` + uuid.UUID(f.user.ID).String() + `">Ada Lovelace</option>`,
		`<a class="form__cancel" href="` + accountsPath + `"`,
		">Add account</button>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "Grace Hopper") || strings.Contains(body, "account-list") {
		t.Fatalf("new account page offers a non-member or shows the list:\n%s", body)
	}
}

// Without the currencies the form cannot be offered, so its page reports the
// accounts as unavailable.

func TestNewAccountWithoutCurrencies(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.currencyErr = errors.New("storage unavailable")

	response := f.do(request{path: accountsPath + "/new", cookies: cookies})

	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `id="account-form"`) {
		t.Fatalf("new account without currencies = %d:\n%s", response.Code, response.Body.String())
	}
}

func TestAccountsEmpty(t *testing.T) {
	f := newFixture(t)
	body := f.do(request{path: accountsPath, cookies: f.signIn(t)}).Body.String()

	if !strings.Contains(body, `<p class="account-list__empty">There are no accounts yet.</p>`) || strings.Contains(body, "account-list__item") {
		t.Fatalf("empty list is not explained:\n%s", body)
	}
}

func TestAccountsUnavailable(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.accountErr = errors.New("storage unavailable")

	response := f.do(request{path: accountsPath, cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK || !strings.Contains(body, "Accounts are unavailable right now.") || !strings.Contains(body, `href="`+accountsPath+`/new"`) {
		t.Fatalf("unavailable list = %d:\n%s", response.Code, body)
	}
}

// A viewer sees the accounts without ways to change them, and is refused every
// change.

func TestAccountsAsViewer(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	account := f.addAccount(designID, "Cash", nil)
	f.setRole(designID, workspaces.RoleViewer)

	body := f.do(request{path: accountsPath, cookies: cookies}).Body.String()

	if !strings.Contains(body, "Only editors and owners can change accounts.") || !strings.Contains(body, ">Cash<") {
		t.Fatalf("viewer list lacks the accounts:\n%s", body)
	}

	for _, unwanted := range []string{`id="account-form"`, `class="button"`, accountsPath + "/"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("viewer list contains %s:\n%s", unwanted, body)
		}
	}

	accountPath := accountsPath + "/" + uuid.UUID(account.ID).String()

	for _, r := range []request{
		{path: accountPath},
		{path: accountsPath + "/new"},
		{method: http.MethodPost, path: accountsPath, body: "name=Card&currency=EUR&user="},
		{method: http.MethodPost, path: accountPath, body: "name=Card&user="},
		{method: http.MethodPost, path: accountPath + "/delete", body: "confirm=1"},
	} {
		r.cookies = cookies

		if response := f.do(r); response.Code != http.StatusForbidden {
			t.Fatalf("%s %s as a viewer = %d", r.method, r.path, response.Code)
		}
	}

	if len(f.accounts) != 1 || f.accounts[0].Name != "Cash" {
		t.Fatalf("accounts changed: %+v", f.accounts)
	}
}

// Another workspace's accounts, and those of a workspace the user does not
// belong to, are not found.

func TestAccountsNotFound(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	family := f.addAccount(familyID, "Family only", nil)
	other := f.addAccount(otherID, "Someone else's", nil)

	for _, path := range []string{
		accountsPath + "/" + uuid.UUID(family.ID).String(),
		accountsPath + "/" + uuid.UUID(other.ID).String(),
		accountsPath + "/01920000-0000-7000-8000-0000000000ff",
		accountsPath + "/not-a-uuid",
		"/office/en/workspaces/" + otherID + "/accounts",
		"/office/en/workspaces/" + otherID + "/accounts/" + uuid.UUID(other.ID).String(),
	} {
		response := f.do(request{path: path, cookies: cookies})

		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "only") || strings.Contains(response.Body.String(), "Someone") {
			t.Fatalf("GET %s = %d:\n%s", path, response.Code, response.Body.String())
		}
	}

	response := f.do(request{
		method:  http.MethodPost,
		path:    accountsPath + "/" + uuid.UUID(family.ID).String(),
		body:    "name=Mine&currency=EUR&user=",
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != accountsPath || family.Name != "Family only" {
		t.Fatalf("update of another workspace's account = %d %q, name %q", response.Code, response.Header().Get("Location"), family.Name)
	}

	if response := f.do(request{path: accountsPath}); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("accounts as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestCreateAccount(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    accountsPath,
		body:    "name=" + url.QueryEscape("  Cash ") + "&currency=EUR&user=" + uuid.UUID(f.user.ID).String(),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != accountsPath {
		t.Fatalf("create = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if len(f.accounts) != 1 {
		t.Fatalf("accounts = %+v, want one", f.accounts)
	}

	created := f.accounts[0]

	if created.Name != "Cash" || created.CurrencyCode != "EUR" || created.UserID == nil || *created.UserID != f.user.ID ||
		created.WorkspaceID != workspaces.ID(uuid.MustParse(designID)) {
		t.Fatalf("created = %+v", created)
	}

	body := f.do(request{path: accountsPath, cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Account “Cash” was added.</p>") || !strings.Contains(body, ">Ada Lovelace<") ||
		!strings.Contains(body, `<span class="account-list__currency">EUR</span>`) {
		t.Fatalf("list does not confirm the new account:\n%s", body)
	}
}

// A rejected account replaces only the form, keeping what was typed.

func TestCreateAccountRejected(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   func(f *fixture) string
		status int
		wants  []string
	}{
		{
			name:   "blank name",
			body:   func(*fixture) string { return "name=++&currency=EUR&user=" },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="account-name-error">Please fill in this field.</p>`},
		},
		{
			name: "long name",
			body: func(*fixture) string {
				return "name=" + strings.Repeat("a", accounts.MaxNameLength+1) + "&currency=EUR&user="
			},
			status: http.StatusUnprocessableEntity,
			wants:  []string{fmt.Sprintf("Use at most %d characters.", accounts.MaxNameLength)},
		},
		{
			name:   "malformed member",
			body:   func(*fixture) string { return "name=Cash&currency=EUR&user=nobody" },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="account-user-error">Choose one of the options.</p>`, `value="Cash"`},
		},
		{
			name:   "not a member",
			body:   func(f *fixture) string { return "name=Cash&currency=EUR&user=" + uuid.UUID(f.others[0].ID).String() },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="account-user-error">Choose one of the options.</p>`},
		},
		{
			name:   "no currency",
			body:   func(*fixture) string { return "name=Cash&currency=&user=" },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="account-currency-error">Choose one of the options.</p>`, `value="Cash"`},
		},
		{
			name:   "unknown currency",
			body:   func(*fixture) string { return "name=Cash&currency=GBP&user=" },
			status: http.StatusUnprocessableEntity,
			wants:  []string{`<p class="field__error" id="account-currency-error">Choose one of the options.</p>`},
		},
		{
			name:   "unknown field",
			body:   func(*fixture) string { return "name=Cash&currency=EUR&user=&budget=1" },
			status: http.StatusBadRequest,
			wants:  []string{`<div class="form-alert" role="alert">`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			response := f.do(request{method: http.MethodPost, path: accountsPath, body: tt.body(f), cookies: f.signIn(t), headers: toContent})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("HX-Retarget") != "#account-form" || !strings.HasPrefix(body, `<form id="account-form"`) {
				t.Fatalf("create = %d %v, want %d with only the form:\n%s", response.Code, response.Header(), tt.status, body)
			}

			for _, want := range tt.wants {
				if !strings.Contains(body, want) {
					t.Fatalf("form lacks %s:\n%s", want, body)
				}
			}

			if len(f.accounts) != 0 {
				t.Fatalf("accounts = %+v, want none", f.accounts)
			}
		})
	}
}

func TestEditAccount(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	account := f.addAccount(designID, "Cash", &f.user.ID)
	path := accountsPath + "/" + uuid.UUID(account.ID).String()
	userID := uuid.UUID(f.user.ID).String()

	body := f.do(request{path: path, cookies: cookies}).Body.String()

	for _, want := range []string{
		"<title>Cash · Faryen</title>",
		`<div id="messages" class="messages"></div>`,
		`href="` + accountsPath + `" aria-current="page"`,
		`<form id="account-form" class="form" method="post" action="` + path + `" novalidate`,
		`value="Cash"`,
		`<option value="` + userID + `" selected>Ada Lovelace</option>`,
		`<span class="field__label">Currency</span><p class="field__value">EUR</p>`,
		"The currency is chosen when the account is created and cannot be changed.",
		`<form id="account-delete-form" class="danger-zone" method="post" action="` + path + `/delete" data-confirm-always`,
		"“Cash” will be deleted. This cannot be undone.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	response := f.do(request{method: http.MethodPost, path: path, body: "name=Wallet&user=", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != accountsPath {
		t.Fatalf("update = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if updated := f.accounts[0]; updated.Name != "Wallet" || updated.CurrencyCode != "EUR" || updated.UserID != nil || updated.ID != account.ID {
		t.Fatalf("updated = %+v", updated)
	}

	if body := f.do(request{path: accountsPath, cookies: cookies}).Body.String(); !strings.Contains(body, "<p>Account “Wallet” was saved.</p>") {
		t.Fatalf("list does not confirm the change:\n%s", body)
	}

	response = f.do(request{method: http.MethodPost, path: path, body: "name=&user=", cookies: cookies, headers: toContent})

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "Please fill in this field.") ||
		!strings.Contains(response.Body.String(), `<p class="field__value">EUR</p>`) || f.accounts[0].Name != "Wallet" {
		t.Fatalf("rejected update = %d:\n%s", response.Code, response.Body.String())
	}
}

// An account keeps the currency it was created with, so an edit that submits
// one is refused and changes nothing.

func TestEditAccountKeepsItsCurrency(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	account := f.addAccount(designID, "Cash", nil)
	path := accountsPath + "/" + uuid.UUID(account.ID).String()

	response := f.do(request{method: http.MethodPost, path: path, body: "name=Wallet&currency=USD&user=", cookies: cookies, headers: toContent})

	if response.Code != http.StatusBadRequest || f.accounts[0].Name != "Cash" || f.accounts[0].CurrencyCode != "EUR" {
		t.Fatalf("edit with a currency = %d, account %+v:\n%s", response.Code, f.accounts[0], response.Body.String())
	}
}

// An account keeps the User who has left the workspace when it is saved
// unchanged; the form offers that User as a former member.

func TestEditAccountOfFormerMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	formerID := f.others[0].ID
	account := f.addAccount(designID, "Old card", &formerID)
	path := accountsPath + "/" + uuid.UUID(account.ID).String()
	former := uuid.UUID(formerID).String()

	body := f.do(request{path: path, cookies: cookies}).Body.String()

	if !strings.Contains(body, `<option value="`+former+`" selected>Former member</option>`) || strings.Contains(body, "Grace") {
		t.Fatalf("form does not keep the former member:\n%s", body)
	}

	response := f.do(request{method: http.MethodPost, path: path, body: "name=Card&user=" + former, cookies: cookies})

	if response.Code != http.StatusSeeOther || f.accounts[0].Name != "Card" || f.accounts[0].UserID == nil || *f.accounts[0].UserID != formerID {
		t.Fatalf("update = %d, account %+v", response.Code, f.accounts[0])
	}
}

// Only a confirmed deletion removes the account.

func TestDeleteAccount(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	account := f.addAccount(designID, "Cash", nil)
	path := accountsPath + "/" + uuid.UUID(account.ID).String()

	response := f.do(request{method: http.MethodPost, path: path + "/delete", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != path || len(f.accounts) != 1 {
		t.Fatalf("unconfirmed delete = %d %q with %d accounts", response.Code, response.Header().Get("Location"), len(f.accounts))
	}

	response = f.do(request{method: http.MethodPost, path: path + "/delete", body: "confirm=1", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != accountsPath || len(f.accounts) != 0 {
		t.Fatalf("confirmed delete = %d %q with %d accounts", response.Code, response.Header().Get("Location"), len(f.accounts))
	}

	if body := f.do(request{path: accountsPath, cookies: cookies}).Body.String(); !strings.Contains(body, "<p>Account “Cash” was deleted.</p>") {
		t.Fatalf("list does not confirm the deletion:\n%s", body)
	}

	// A page left open after the deletion leads back to the list.
	response = f.do(request{method: http.MethodPost, path: path + "/delete", body: "confirm=1", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != accountsPath {
		t.Fatalf("repeated delete = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestAccountsPagination(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	for index := range 51 {
		f.addAccount(designID, fmt.Sprintf("Account %02d", index), nil)
	}

	first := f.do(request{path: accountsPath, cookies: cookies}).Body.String()
	second := f.do(request{path: accountsPath + "?page=2", cookies: cookies}).Body.String()

	if strings.Count(first, `class="account-list__item"`) != 50 || !strings.Contains(first, `href="`+accountsPath+`?page=2"`) ||
		!strings.Contains(first, "Page 1 of 2") || strings.Contains(first, ">Previous<") {
		t.Fatalf("first page does not lead to the second:\n%s", first)
	}

	if strings.Count(second, `class="account-list__item"`) != 1 || !strings.Contains(second, ">Account 50<") ||
		!strings.Contains(second, `href="`+accountsPath+`" hx-boost`) || strings.Contains(second, ">Next<") {
		t.Fatalf("second page does not lead back to the first:\n%s", second)
	}
}
