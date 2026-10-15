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

// The profile settings have an address outside any workspace and one inside
// each workspace, where they keep the workspace's dashboard.

func ProfileSettingsPath(request *http.Request) string {
	return OfficePath(request) + "/profile-settings"
}

func WorkspaceProfileSettingsPath(request *http.Request, id workspaces.ID) string {
	return WorkspacePath(request, id) + "/profile-settings"
}

func WorkspacePath(request *http.Request, id workspaces.ID) string {
	return WorkspacesPath(request) + "/" + uuid.UUID(id).String()
}

//
// Dashboard
//

// WorkspaceSettingsSection is the menu item of the workspace's own settings.
// ProfileSettingsSection marks the profile settings inside a workspace, which
// the top bar's settings link leads to rather than a menu item.

const (
	WorkspaceSettingsSection = "workspace_settings"
	ProfileSettingsSection   = "profile_settings"
)

// Every section belongs to the workspace in the address. The workspace's
// settings close the menu, with their own icon, apart from the profile
// settings' one in the top bar.

var sections = []layouts.NavigationItem{
	{Key: "overview", Icon: "overview"},
	{Key: "budgets", Icon: "budgets", Href: "/budgets"},
	{Key: "transactions", Icon: "transactions", Href: "/transactions"},
	{Key: "goals", Icon: "goals", Href: "/goals"},
	{Key: "reports", Icon: "reports", Href: "/reports"},
	{Key: CategoriesSection, Icon: "categories", Href: "/categories"},
	{Key: WorkspaceSettingsSection, Icon: "workspace-settings", Href: "/settings"},
}

func DashboardProps(
	request *http.Request,
	current *security.Identity,
	workspace *workspaces.Workspace,
	active string,
	title string,
) layouts.DashboardProps {
	base := WorkspacePath(request, workspace.ID)
	navigation := make([]layouts.NavigationItem, 0, len(sections))

	for _, item := range sections {
		item.Href = base + item.Href
		navigation = append(navigation, item)
	}

	props := layouts.DashboardProps{
		Title:      title,
		Active:     active,
		Navigation: navigation,
		TopBar:     topBarProps(request, current, active == ProfileSettingsSection),
	}

	props.TopBar.SettingsURL = WorkspaceProfileSettingsPath(request, workspace.ID)

	props.TopBar.Workspace = &layouts.WorkspaceSummary{
		Name:      string(workspace.Name),
		Initial:   initial(string(workspace.Name)),
		SwitchURL: WorkspacesPath(request),
	}

	return props
}

//
// Profile settings
//

func ProfileSettingsLayoutProps(request *http.Request, current *security.Identity) layouts.StandaloneProps {
	props := layouts.StandaloneProps{
		TopBar: topBarProps(request, current, true),
	}

	props.TopBar.SelectURL = WorkspacesPath(request)

	return props
}

//
// Helpers
//

// The settings link leads to the profile settings; inside a workspace the
// dashboard leads to its own address for them. A workspace's own settings are
// in its sidebar.

func topBarProps(
	request *http.Request,
	current *security.Identity,
	settingsActive bool,
) layouts.TopBarProps {
	return layouts.TopBarProps{
		SettingsURL:    ProfileSettingsPath(request),
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
