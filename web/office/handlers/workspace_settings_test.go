package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// An owner renames the workspace for everyone; the top bar shows the new name.

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

// An owner adds an existing User by email, ignoring letter case; the
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

	if role := f.memberRole(workspaces.WorkspaceID(uuid.MustParse(designID)), grace.ID); role != workspaces.RoleViewer {
		t.Fatalf("Grace's role = %q, want her added as a viewer by default", role)
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
			body:     "email=grace%40example.com&owner=me",
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

// An owner removes another member; their own row has no remove button.

func TestRemoveWorkspaceMember(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	grace := f.others[0]
	design := workspaces.WorkspaceID(uuid.MustParse(designID))
	f.setMemberRole(design, grace.ID, workspaces.RoleViewer)
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

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" || f.memberRole(design, grace.ID) != "" {
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

//
// Workspace roles
//

// The signed-in User takes the role; Grace is an owner beside a non-owner,
// and an editor beside an owner, so the workspace always keeps an owner.

func withRole(t *testing.T, role workspaces.Role) (*fixture, []*http.Cookie) {
	t.Helper()

	f := newFixture(t)
	design := workspaces.WorkspaceID(uuid.MustParse(designID))
	graceRole := workspaces.RoleEditor

	if role != workspaces.RoleOwner {
		graceRole = workspaces.RoleOwner
	}

	f.setMemberRole(design, f.user.ID, role)
	f.setMemberRole(design, f.others[0].ID, graceRole)

	return f, f.signIn(t)
}

// Every member sees the settings with each member's role; only an owner sees
// the forms that change them. The owner is the only one, so only Grace's
// role can be changed.

func TestWorkspaceSettingsByRole(t *testing.T) {
	for _, tt := range []struct {
		role       workspaces.Role
		contains   []string
		unwanted   []string
		roleForms  int
		removeForm int
	}{
		{
			role: workspaces.RoleOwner,
			contains: []string{
				`<form id="workspace-form"`,
				`<form id="workspace-member-form"`,
				`<select class="field__input field__input--select select__native" id="member-role" name="role">`,
				`<option value="viewer" selected>Viewer</option>`,
				`aria-labelledby="member-role-label member-role-toggle"><span class="select__value">Viewer</span>`,
				`aria-label="Role of Grace Hopper"`,
			},
			unwanted:   []string{`class="field__value"`, "Only the workspace's owners"},
			roleForms:  1,
			removeForm: 1,
		},
		{
			role: workspaces.RoleEditor,
			contains: []string{
				`<p class="field__value">Design</p>`,
				"<p>Only the workspace's owners can change its name and members.</p>",
				`<span class="member-list__role">Editor</span>`,
				`<span class="member-list__role">Owner</span>`,
			},
			unwanted: []string{`id="workspace-form"`, `id="workspace-member-form"`, "<select", "/settings/workspace", "/settings/members"},
		},
		{
			role: workspaces.RoleViewer,
			contains: []string{
				`<p class="field__value">Design</p>`,
				"<p>Only the workspace's owners can change its name and members.</p>",
				`<span class="member-list__role">Viewer</span>`,
				`<span class="member-list__role">Owner</span>`,
			},
			unwanted: []string{`id="workspace-form"`, `id="workspace-member-form"`, "<select", "/settings/workspace", "/settings/members"},
		},
	} {
		t.Run(string(tt.role), func(t *testing.T) {
			f, cookies := withRole(t, tt.role)

			response := f.do(request{path: designPath + "/settings", cookies: cookies})
			body := response.Body.String()

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d:\n%s", response.Code, body)
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}

			for _, unwanted := range tt.unwanted {
				if strings.Contains(body, unwanted) {
					t.Fatalf("body contains %s:\n%s", unwanted, body)
				}
			}

			if got := strings.Count(body, `class="member-list__role-form"`); got != tt.roleForms {
				t.Fatalf("role forms = %d, want %d", got, tt.roleForms)
			}

			if got := strings.Count(body, `class="member-list__remove"`); got != tt.removeForm {
				t.Fatalf("remove forms = %d, want %d", got, tt.removeForm)
			}
		})
	}
}

// The only owner's role cannot be changed in place; beside another owner it
// can.

func TestWorkspaceSettingsLastOwner(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	design := workspaces.WorkspaceID(uuid.MustParse(designID))
	grace := f.others[0]
	adaRole := designPath + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/role"
	graceRole := designPath + "/settings/members/" + uuid.UUID(grace.ID).String() + "/role"

	f.setMemberRole(design, grace.ID, workspaces.RoleEditor)
	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if strings.Contains(body, adaRole) || !strings.Contains(body, graceRole) || !strings.Contains(body, `<span class="member-list__role">Owner</span>`) {
		t.Fatalf("settings let the last owner's role change, or not Grace's:\n%s", body)
	}

	f.setMemberRole(design, grace.ID, workspaces.RoleOwner)
	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, adaRole) || !strings.Contains(body, graceRole) {
		t.Fatalf("settings do not let either owner's role change:\n%s", body)
	}
}

// Every change a non-owner posts is forbidden and changes nothing; an owner's
// is saved.

func TestWorkspaceSettingsPostsByRole(t *testing.T) {
	routes := []struct {
		name string
		path func(f *fixture) string
		body string
	}{
		{"rename", func(*fixture) string { return designPath + "/settings/workspace" }, "name=Studio"},
		{"add member", func(*fixture) string { return designPath + "/settings/members" }, "email=nobody%40example.com"},
		{"change role", func(f *fixture) string {
			return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/role"
		}, "role=viewer"},
		{"remove member", func(f *fixture) string {
			return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/remove"
		}, ""},
	}

	for _, role := range []workspaces.Role{workspaces.RoleEditor, workspaces.RoleViewer} {
		for _, route := range routes {
			t.Run(string(role)+" "+route.name, func(t *testing.T) {
				f, cookies := withRole(t, role)
				before := fmt.Sprint(f.members)

				response := f.do(request{method: http.MethodPost, path: route.path(f), body: route.body, cookies: cookies})
				body := response.Body.String()

				if response.Code != http.StatusForbidden {
					t.Fatalf("status = %d %q, want 403:\n%s", response.Code, response.Header().Get("Location"), body)
				}

				for _, want := range []string{"Access denied", "Your role in this workspace does not allow this change."} {
					if !strings.Contains(body, want) {
						t.Fatalf("body lacks %s:\n%s", want, body)
					}
				}

				if fmt.Sprint(f.members) != before || workspaceName(f, designID) != "Design" {
					t.Fatal("a forbidden change changed the workspace")
				}
			})
		}
	}

	// An owner passes the role check; the unknown email then fails on its
	// own.
	for index, status := range []int{http.StatusSeeOther, http.StatusUnprocessableEntity, http.StatusSeeOther, http.StatusSeeOther} {
		route := routes[index]

		t.Run("owner "+route.name, func(t *testing.T) {
			f, cookies := withRole(t, workspaces.RoleOwner)

			response := f.do(request{method: http.MethodPost, path: route.path(f), body: route.body, cookies: cookies})

			if response.Code != status {
				t.Fatalf("%s as an owner = %d, want %d:\n%s", route.name, response.Code, status, response.Body.String())
			}
		})
	}

	// The refusal is in the language of the address.
	f, cookies := withRole(t, workspaces.RoleViewer)
	response := f.do(request{method: http.MethodPost, path: "/office/ru/workspaces/" + designID + "/settings/workspace", body: "name=Studio", cookies: cookies})

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "Ваша роль в этом рабочем пространстве не позволяет внести это изменение.") {
		t.Fatalf("forbidden rename in Russian = %d:\n%s", response.Code, response.Body.String())
	}
}

// An owner changes another member's role in place: the select's styled list
// saves as soon as an option is chosen. The confirmation names the member and
// the new role.

func TestChangeWorkspaceMemberRole(t *testing.T) {
	f, cookies := withRole(t, workspaces.RoleOwner)
	design := workspaces.WorkspaceID(uuid.MustParse(designID))
	grace := f.others[0]
	rolePath := designPath + "/settings/members/" + uuid.UUID(grace.ID).String() + "/role"

	body := f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	for _, want := range []string{
		`<form class="member-list__role-form" method="post" action="` + rolePath + `"`,
		`<select class="field__input field__input--select select__native" id="member-` + uuid.UUID(grace.ID).String() + `-role" name="role" data-auto-submit aria-label="Role of Grace Hopper">`,
		`<option value="editor" selected>Editor</option>`,
		`<li class="select__option" role="option" tabindex="-1" data-value="editor" aria-selected="true"><span>Editor</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings lack %s:\n%s", want, body)
		}
	}

	response := f.do(request{method: http.MethodPost, path: rolePath, body: "role=owner", cookies: cookies})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != designPath+"/settings" || f.memberRole(design, grace.ID) != workspaces.RoleOwner {
		t.Fatalf("change role = %d %q, role %q", response.Code, response.Header().Get("Location"), f.memberRole(design, grace.ID))
	}

	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if !strings.Contains(body, "<p>Grace Hopper's role has been changed to Owner.</p>") {
		t.Fatalf("settings lack the confirmation:\n%s", body)
	}

	// Beside Grace, Ada is no longer the last owner and may step down; the
	// settings then become read-only for her.
	adaPath := designPath + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/role"
	response = f.do(request{method: http.MethodPost, path: adaPath, body: "role=editor", cookies: cookies})

	if response.Code != http.StatusSeeOther || f.memberRole(design, f.user.ID) != workspaces.RoleEditor {
		t.Fatalf("step down = %d, role %q", response.Code, f.memberRole(design, f.user.ID))
	}

	body = f.do(request{path: designPath + "/settings", cookies: cookies}).Body.String()

	if strings.Contains(body, `id="workspace-form"`) || !strings.Contains(body, `<p class="field__value">Design</p>`) {
		t.Fatalf("settings after stepping down are not read-only:\n%s", body)
	}
}

func TestChangeWorkspaceMemberRoleRejected(t *testing.T) {
	for _, tt := range []struct {
		name     string
		path     func(f *fixture) string
		body     string
		status   int
		location string
		contains []string
	}{
		{
			name: "last owner demoted",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/role"
			},
			body:     "role=viewer",
			status:   http.StatusConflict,
			contains: []string{"The workspace must keep at least one owner."},
		},
		{
			name: "unknown role",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/role"
			},
			body:     "role=admin",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name: "missing role",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/role"
			},
			body:     "role=",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name: "malformed form",
			path: func(f *fixture) string {
				return designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/role"
			},
			body:     "role=viewer&user=me",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:     "malformed member",
			path:     func(*fixture) string { return designPath + "/settings/members/not-a-uuid/role" },
			body:     "role=viewer",
			status:   http.StatusBadRequest,
			contains: []string{"The form could not be read."},
		},
		{
			name:     "unknown user",
			path:     func(*fixture) string { return designPath + "/settings/members/" + otherID + "/role" },
			body:     "role=viewer",
			status:   http.StatusSeeOther,
			location: designPath + "/settings",
		},
		{
			name: "another user's workspace",
			path: func(f *fixture) string {
				return "/office/en/workspaces/" + otherID + "/settings/members/" + uuid.UUID(f.user.ID).String() + "/role"
			},
			body:     "role=viewer",
			status:   http.StatusSeeOther,
			location: "/office/en/workspaces",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cookies := f.signIn(t)
			f.setMemberRole(workspaces.WorkspaceID(uuid.MustParse(designID)), f.others[0].ID, workspaces.RoleEditor)
			before := fmt.Sprint(f.members)

			response := f.do(request{method: http.MethodPost, path: tt.path(f), body: tt.body, cookies: cookies})
			body := response.Body.String()

			if response.Code != tt.status || response.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d %q, want %d %q:\n%s", response.Code, response.Header().Get("Location"), tt.status, tt.location, body)
			}

			if fmt.Sprint(f.members) != before {
				t.Fatalf("members = %v, want unchanged %v", f.members, before)
			}

			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %s:\n%s", want, body)
				}
			}
		})
	}

	f := newFixture(t)
	response := f.do(request{method: http.MethodPost, path: designPath + "/settings/members/" + uuid.UUID(f.others[0].ID).String() + "/role", body: "role=viewer"})

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/office/en/sign-in" {
		t.Fatalf("change role as a visitor = %d %q", response.Code, response.Header().Get("Location"))
	}
}

// The added member gets the chosen role; a role the workspace does not know
// is a problem with the role field, and keeps what was chosen.

func TestAddWorkspaceMemberWithRole(t *testing.T) {
	f := newFixture(t)
	cookies := f.signIn(t)
	design := workspaces.WorkspaceID(uuid.MustParse(designID))

	response := f.do(request{
		method:  http.MethodPost,
		path:    designPath + "/settings/members",
		body:    "email=grace%40example.com&role=admin",
		cookies: cookies,
		headers: toContent,
	})
	body := response.Body.String()

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `<p class="field__error" id="member-role-error">This value is not valid.</p>`) ||
		f.memberRole(design, f.others[0].ID) != "" {
		t.Fatalf("add with an unknown role = %d:\n%s", response.Code, body)
	}

	response = f.do(request{method: http.MethodPost, path: designPath + "/settings/members", body: "email=grace%40example.com&role=editor", cookies: cookies})

	if response.Code != http.StatusSeeOther || f.memberRole(design, f.others[0].ID) != workspaces.RoleEditor {
		t.Fatalf("add as an editor = %d, role %q", response.Code, f.memberRole(design, f.others[0].ID))
	}
}
