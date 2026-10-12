package utils

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/web/office/templates/layouts"
)

//
// Paths
//

func WorkspacesPath(request *http.Request) string {
	return OfficePath(request) + "/workspaces"
}

func WorkspacePath(request *http.Request, id workspaces.WorkspaceID) string {
	return WorkspacesPath(request) + "/" + uuid.UUID(id).String()
}

//
// Dashboard
//

var sections = []string{"overview", "budgets", "transactions", "goals", "reports"}

// Every section, the settings among them, belongs to the workspace in the
// address.

func DashboardProps(
	request *http.Request,
	current *security.Identity,
	workspace *workspaces.Workspace,
	active string,
	title string,
) layouts.DashboardProps {
	base := WorkspacePath(request, workspace.ID)
	navigation := make([]layouts.NavigationItem, 0, len(sections))

	for _, key := range sections {
		href := base + "/" + key

		if key == "overview" {
			href = base
		}

		navigation = append(navigation, layouts.NavigationItem{Key: key, Href: href})
	}

	props := layouts.DashboardProps{
		Title:      title,
		Active:     active,
		Navigation: navigation,
		TopBar:     topBarProps(request, current, base+"/settings", active == "settings"),
	}

	props.TopBar.Workspace = &layouts.WorkspaceSummary{
		Name:      string(workspace.Name),
		Initial:   initial(string(workspace.Name)),
		SwitchURL: WorkspacesPath(request),
	}

	return props
}

//
// Settings
//

func SettingsLayoutProps(request *http.Request, current *security.Identity) layouts.StandaloneProps {
	props := layouts.StandaloneProps{
		TopBar: topBarProps(request, current, OfficePath(request)+"/settings", true),
	}

	props.TopBar.SelectURL = WorkspacesPath(request)

	return props
}

//
// Helpers
//

// Inside a workspace the settings button leads to the workspace's settings;
// elsewhere it leads to the settings without a workspace.

func topBarProps(
	request *http.Request,
	current *security.Identity,
	settingsURL string,
	settingsActive bool,
) layouts.TopBarProps {
	return layouts.TopBarProps{
		SettingsURL:    settingsURL,
		SignOutURL:     OfficePath(request) + "/sign-out",
		SettingsActive: settingsActive,
		Profile:        profile(current),
	}
}

func profile(current *security.Identity) layouts.Profile {
	first, last := string(current.User.FirstName), string(current.User.LastName)

	return layouts.Profile{
		Name:     strings.TrimSpace(first + " " + last),
		Email:    string(current.User.Email),
		Initials: initial(first) + initial(last),
	}
}

func initial(name string) string {
	letter, _ := utf8.DecodeRuneInString(strings.TrimSpace(name))
	result := ""

	if letter != utf8.RuneError {
		result = string(unicode.ToUpper(letter))
	}

	return result
}
