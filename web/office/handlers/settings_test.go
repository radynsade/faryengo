package handlers

import (
	"slices"
	"strings"
	"testing"
)

// The top bar's settings link leads to the profile settings: outside a
// workspace to their own address, inside one to their address in that
// workspace, so the dashboard stays. Only the profile settings mark it. The
// sidebar's last item leads to the workspace's settings, and only that page
// marks it; the profile settings mark no sidebar item.

func TestSettingsNavigation(t *testing.T) {
	const (
		sidebarLink   = `<a class="sidebar__link" href="` + designPath + `/settings"`
		sidebarActive = sidebarLink + ` aria-current="page"`
	)

	f := newFixture(t)
	cookies := f.signIn(t)

	for _, path := range slices.Concat(dashboardPages, standalonePages) {
		body := f.do(request{path: path, cookies: cookies}).Body.String()
		inWorkspace := slices.Contains(dashboardPages, path)
		profileLink := `<a class="dropdown__item" href="/office/en/profile-settings"`

		if inWorkspace {
			profileLink = `<a class="dropdown__item" href="` + designPath + `/profile-settings"`
		}

		profileMenu := body[strings.Index(body, `<details class="dropdown profile-menu"`):]
		profileMenu = profileMenu[:strings.Index(profileMenu, "</details>")]

		if !strings.Contains(profileMenu, profileLink) || strings.Count(profileMenu, "settings") != 2 {
			t.Fatalf("GET %s top bar does not lead to %s:\n%s", path, profileLink, profileMenu)
		}

		onProfile := strings.HasSuffix(path, "/profile-settings")

		if active := strings.Contains(profileMenu, profileLink+` aria-current="page"`); active != onProfile {
			t.Fatalf("GET %s marks the profile settings link = %t", path, active)
		}

		if hasItem := strings.Contains(body, sidebarLink); hasItem != inWorkspace {
			t.Fatalf("GET %s has the workspace settings item = %t", path, hasItem)
		}

		if active := strings.Contains(body, sidebarActive); active != (path == designPath+"/settings") {
			t.Fatalf("GET %s marks the workspace settings item = %t", path, active)
		}

		if inWorkspace {
			menu := body[strings.Index(body, `<nav id="dashboard-menu"`):]
			menu = menu[:strings.Index(menu, "</nav>")]
			item := menu[strings.Index(menu, sidebarLink):]

			// The settings close the menu, with their own icon and label.
			if strings.Count(menu, `class="sidebar__link"`) != 5 || strings.Count(item, "</li>") != 1 ||
				!strings.Contains(item, `#workspace-settings"></use></svg><span>Settings</span>`) {
				t.Fatalf("GET %s menu does not end with the workspace settings:\n%s", path, menu)
			}

			if onProfile && strings.Contains(menu, `aria-current`) {
				t.Fatalf("GET %s marks a sidebar item:\n%s", path, menu)
			}
		}
	}
}
