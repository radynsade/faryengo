package utils

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/web/office/templates/layouts"
)

//
// Dashboard
//

var sections = []string{"overview", "budgets", "transactions", "goals", "reports"}

func DashboardProps(
	request *http.Request,
	current *security.Identity,
	active string,
	title string,
) layouts.DashboardProps {
	base := OfficePath(request)
	navigation := make([]layouts.NavigationItem, 0, len(sections))

	for _, key := range sections {
		href := base + "/" + key

		if key == "overview" {
			href = base
		}

		navigation = append(navigation, layouts.NavigationItem{Key: key, Href: href})
	}

	return layouts.DashboardProps{
		Title:       title,
		Active:      active,
		Navigation:  navigation,
		SettingsURL: base + "/settings",
		SignOutURL:  base + "/sign-out",
		Profile:     profile(current),
	}
}

//
// Helpers
//

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
