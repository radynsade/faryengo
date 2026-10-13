package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/workspaces"
)

func TestWorkspaceSettingsPage(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{path: designPath + "/settings", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d:\n%s", response.Code, body)
	}

	for _, want := range []string{
		"<title>Workspace settings · Faryen</title>",
		`<h1 id="page-title">Workspace settings</h1>`,
		`<aside id="dashboard-sidebar" class="sidebar">`,
		`<main id="dashboard-main" class="dashboard__main dashboard__main--narrow"`,
		`<h2 id="workspace-title">Workspace</h2>`,
		`<form id="workspace-form" class="form" method="post" action="` + designPath + `/settings/workspace" novalidate`,
		`name="name" type="text" placeholder autocomplete="off" value="Design" required`,
		`<h3 id="members-title">Members</h3>`,
		`<ul class="member-list" aria-labelledby="members-title">`,
		`<h3 id="member-title">Add a member</h3>`,
		`<form id="workspace-member-form" class="form" method="post" action="` + designPath + `/settings/members" novalidate aria-labelledby="member-title"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s:\n%s", want, body)
		}
	}

	for _, unwanted := range []string{
		"profile-form", "profile-title", "profile-confirm", "password-form", "password-title", "password-confirm",
		"/settings/profile", "/settings/password", `class="message"`, "hx-swap-oob",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains %s", unwanted)
		}
	}
}

// The settings of a workspace the user does not belong to, or of one that
// does not exist, are not found; the user's own workspace is unchanged.

func TestWorkspaceSettingsNotFound(t *testing.T) {
	for _, path := range []string{
		"/office/en/workspaces/" + otherID + "/settings",
		"/office/en/workspaces/01920000-0000-7000-8000-0000000000ff/settings",
		"/office/en/workspaces/not-a-uuid/settings",
	} {
		t.Run(path, func(t *testing.T) {
			f := newFixture(t)
			response := f.do(request{path: path, cookies: f.signIn(t)})
			body := response.Body.String()

			if response.Code != http.StatusNotFound {
				t.Fatalf("GET %s = %d %q, want 404", path, response.Code, response.Header().Get("Location"))
			}

			if strings.Contains(body, "Someone else's") || strings.Contains(body, "workspace-form") || strings.Contains(body, "member-list") {
				t.Fatalf("GET %s exposes the workspace:\n%s", path, body)
			}
		})
	}

	f := newFixture(t)
	response := f.do(request{path: designPath + "/settings"})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("workspace settings as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

// A failed in-page submission replaces only the form, keeping the address.

func TestWorkspaceSettingsReplaceOnlyTheForm(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	for _, tt := range []struct {
		path   string
		body   string
		formID string
	}{
		{path: designPath + "/settings/workspace", body: "name=", formID: "workspace-form"},
		{path: designPath + "/settings/members", body: "email=nobody%40example.com", formID: "workspace-member-form"},
	} {
		response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies, headers: toContent})
		body := response.Body.String()

		if response.Header().Get("HX-Retarget") != "#"+tt.formID || !strings.HasPrefix(body, `<form id="`+tt.formID+`"`) || strings.Contains(body, `<head hx-head`) {
			t.Fatalf("POST %s = %d %v, body is not only the form:\n%s", tt.path, response.Code, response.Header(), body)
		}

		// An earlier confirmation must not stand beside the new errors.
		if !strings.HasSuffix(body, `<div id="messages" class="messages" hx-swap-oob="morph"></div>`) {
			t.Fatalf("POST %s does not clear the confirmations:\n%s", tt.path, body)
		}
	}
}

// A member renames the workspace for everyone; the top bar shows the new name.

func TestRenameWorkspace(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/workspace",
		body:    "name=" + url.QueryEscape("  Studio  "),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" {
		t.Fatalf("rename = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if name := workspaceName(f, designID); name != "Studio" {
		t.Fatalf("workspace name = %q, want the trimmed new name", name)
	}

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	for _, want := range []string{
		"<p>The workspace has been saved.</p>",
		`<span class="topbar__workspace-name">Studio</span>`,
		`value="Studio"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}
}

func TestRenameWorkspaceRejected(t *testing.T) {
	for _, tt := range []struct {
		name      string
		path      string
		body      string
		updateErr error
		status    int
		location  string
		contains  []string
	}{
		{
			name:     "blank name",
			path:     designPath + "/settings/workspace",
			body:     "name=+++",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="workspace-name-error">Please fill in this field.</p>`},
		},
		{
			name:     "too long name in characters",
			path:     "/office/ru/workspaces/" + designID + "/settings/workspace",
			body:     "name=" + url.QueryEscape(strings.Repeat("ā", 101)),
			status:   http.StatusUnprocessableEntity,
			contains: []string{"Используйте не более 100 символов."},
		},
		{
			name:     "malformed form",
			path:     designPath + "/settings/workspace",
			body:     "name=Studio&owner=me",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:      "unavailable storage",
			path:      designPath + "/settings/workspace",
			body:      "name=Studio",
			updateErr: errors.New("storage unavailable"),
			status:    http.StatusServiceUnavailable,
			contains:  []string{"Your settings could not be loaded or saved.", `value="Studio"`},
		},
		{
			name:     "another user's workspace",
			path:     "/office/en/workspaces/" + otherID + "/settings/workspace",
			body:     "name=Studio",
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			f.updateErr = tt.updateErr

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d %q, want %d %q:\n%s", response.Code, response.Header().Get("Location"), tt.status, tt.location, body)
			}

			if workspaceName(f, designID) != "Design" || workspaceName(f, otherID) != "Someone else's" {
				t.Fatal("a rejected rename changed a workspace")
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}

			if strings.Contains(body, "storage unavailable") {
				t.Fatal("response exposes the cause")
			}
		})
	}

	f := newFixture(t)
	response := f.do(request{method: http.MethodPost, path: designPath + "/settings/workspace", body: "name=Studio"})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("rename as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

func workspaceName(f *fixture, id string) workspaces.WorkspaceName {
	var name workspaces.WorkspaceName

	for _, workspace := range f.workspaces {
		if uuid.UUID(workspace.ID).String() == id {
			name = workspace.Name
		}
	}

	return name
}

// A member adds an existing User by email, ignoring letter case; the
// confirmation names the added User.

func TestAddWorkspaceMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/members",
		body:    "email=" + url.QueryEscape("GRACE@example.com"),
		cookies: cookies,
	})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" {
		t.Fatalf("add = %d %q: %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	if !slices.Contains(f.members[workspaces.WorkspaceID(uuid.MustParse(designID))], grace.ID) {
		t.Fatalf("members = %v, want Grace added", f.members)
	}

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Grace Hopper (grace@example.com) has been added to the workspace.</p>") {
		t.Fatalf("settings lack the confirmation:\n%s", body)
	}
}

func TestAddWorkspaceMemberRejected(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     string
		body     string
		status   int
		location string
		contains []string
	}{
		{
			name:     "invalid email",
			path:     designPath + "/settings/members",
			body:     "email=not-an-email",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`<p class="field__error" id="member-email-error">Enter a valid email address.</p>`, `value="not-an-email"`},
		},
		{
			name:     "blank email",
			path:     designPath + "/settings/members",
			body:     "email=",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="member-email-error">Please fill in this field.</p>`},
		},
		{
			name:     "unknown email",
			path:     designPath + "/settings/members",
			body:     "email=nobody%40example.com",
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="member-email-error">No user has this email address.</p>`},
		},
		{
			name:     "already a member",
			path:     designPath + "/settings/members",
			body:     "email=" + url.QueryEscape(testEmail),
			status:   http.StatusUnprocessableEntity,
			contains: []string{`id="member-email-error">This user is already a member of the workspace.</p>`},
		},
		{
			name:     "malformed form",
			path:     designPath + "/settings/members",
			body:     "email=grace%40example.com&role=owner",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:     "another user's workspace",
			path:     "/office/en/workspaces/" + otherID + "/settings/members",
			body:     "email=grace%40example.com",
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			before := len(f.members[workspaces.WorkspaceID(uuid.MustParse(designID))]) + len(f.members[workspaces.WorkspaceID(uuid.MustParse(otherID))])

			response := f.do(request{method: http.MethodPost, path: tt.path, body: tt.body, cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d %q, want %d %q:\n%s", response.Code, response.Header().Get("Location"), tt.status, tt.location, body)
			}

			if after := len(f.members[workspaces.WorkspaceID(uuid.MustParse(designID))]) + len(f.members[workspaces.WorkspaceID(uuid.MustParse(otherID))]); after != before {
				t.Fatal("a rejected addition changed the memberships")
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}
		})
	}

	f := newFixture(t)
	response := f.do(request{method: http.MethodPost, path: designPath + "/settings/members", body: "email=grace%40example.com"})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("add as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

// The members are listed in name order, the signed-in User marked; an added
// member appears in the list.

func TestWorkspaceMembers(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()
	ada := `<li id="member-` + uuid.UUID(f.user.ID).String() + `" class="member-list__item">`

	for _, want := range []string{
		`<h3 id="members-title">Members</h3>`,
		`<ul class="member-list" aria-labelledby="members-title">`,
		ada,
		`<span class="member-list__name">Ada Lovelace</span>`,
		`<span class="member-list__badge">You</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}

	if strings.Contains(body, "Grace Hopper") {
		t.Fatal("the list shows a user who is not a member")
	}

	f.do(request{method: http.MethodPost, path: designPath + "/settings/members", body: "email=grace%40example.com", cookies: cookies})
	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()
	graceRow := `<li id="member-` + uuid.UUID(grace.ID).String() + `" class="member-list__item">`

	if !strings.Contains(body, graceRow) || strings.Index(body, ada) > strings.Index(body, graceRow) || strings.Count(body, "member-list__badge") != 1 {
		t.Fatalf("settings do not list Ada, then Grace, with only Ada marked:\n%s", body)
	}
}

// A member list that cannot be loaded shows its error; the rest of the
// settings stay usable.

func TestWorkspaceMembersUnavailable(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	f.membersErr = errors.New("storage unavailable")

	response := f.do(request{path: designPath + "/settings", cookies: cookies})
	body := response.Body.String()

	if response.Code != http.StatusOK || !strings.Contains(body, "The members could not be loaded.") || !strings.Contains(body, `id="workspace-member-form"`) {
		t.Fatalf("settings = %d:\n%s", response.Code, body)
	}

	if strings.Contains(body, "storage unavailable") || strings.Contains(body, `class="member-list"`) {
		t.Fatal("settings expose the cause or show a list")
	}
}

// A member removes another one; their own row has no remove button.

func TestRemoveWorkspaceMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]
	design := workspaces.WorkspaceID(uuid.MustParse(designID))
	f.members[design] = append(f.members[design], grace.ID)
	removePath := designPath + "/settings/members/" + uuid.UUID(grace.ID).String() + "/remove"

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	for _, want := range []string{
		`<form class="member-list__remove" method="post" action="` + removePath + `"`,
		`aria-label="Remove Grace Hopper from the workspace"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}

	if strings.Count(body, `class="member-list__remove"`) != 1 {
		t.Fatal("the signed-in user's own row has a remove button")
	}

	response := f.do(request{method: http.MethodPost, path: removePath, cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" || slices.Contains(f.members[design], grace.ID) {
		t.Fatalf("remove = %d %q, members %v", response.Code, response.Header().Get("Location"), f.members[design])
	}

	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Grace Hopper has been removed from the workspace.</p>") || strings.Contains(body, `id="member-`+uuid.UUID(grace.ID).String()+`"`) {
		t.Fatalf("settings do not confirm the removal or still list Grace:\n%s", body)
	}
}

// Removing oneself, a user who is no longer a member, or a member of another
// user's workspace changes nothing.

func TestRemoveWorkspaceMemberIgnored(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     func(f *fixture) string
		body     string
		status   int
		location string
	}{
		{
			name: "oneself",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/remove"
			},
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name: "not a member",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/remove"
			},
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name:     "unknown user",
			path:     func(*fixture) string { return designPath + "/settings/members/" + otherID + "/remove" },
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name: "another user's workspace",
			path: func(f *fixture) string {
				return "/office/en/workspaces/" + otherID + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/remove"
			},
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
		{
			name: "malformed form",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/remove"
			},
			body:   "user=me",
			status: http.StatusBadRequest,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			before := fmt.Sprint(f.members)

			response := f.do(request{method: http.MethodPost, path: tt.path(f), body: tt.body, cookies: cookies})

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("remove = %d %q, want %d %q", response.Code, response.Header().Get("Location"), tt.status, tt.location)
			}

			if fmt.Sprint(f.members) != before {
				t.Fatalf("members = %v, want unchanged %v", f.members, before)
			}
		})
	}
}
