package utils

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/workspaces"
	"github.com/radynsade/faryengo/web/office/templates/layouts"
	"github.com/radynsade/faryengo/web/office/templates/pages"
)

//
// Workspace creation
//

func CreateWorkspaceProps(
	request *http.Request,
	current *security.Identity,
	name string,
) pages.CreateWorkspaceProps {
	props := pages.CreateWorkspaceProps{
		Layout: layouts.StandaloneProps{
			TopBar: topBarProps(request, current, false),
		},
		Action:    WorkspacesPath(request),
		CancelURL: WorkspacesPath(request),
		Name:      name,
	}

	props.Layout.TopBar.SelectURL = WorkspacesPath(request)

	return props
}

//
// Workspace selection
//

// Each choice opens the workspace's dashboard.

func SelectWorkspaceProps(
	request *http.Request,
	current *security.Identity,
	found []*workspaces.Workspace,
) pages.SelectWorkspaceProps {
	items := make([]pages.WorkspaceItem, 0, len(found))

	for _, workspace := range found {
		items = append(items, pages.WorkspaceItem{
			ID:      uuid.UUID(workspace.ID).String(),
			Name:    string(workspace.Name),
			Initial: initial(string(workspace.Name)),
			Href:    WorkspacePath(request, workspace.ID),
		})
	}

	return pages.SelectWorkspaceProps{
		Layout: layouts.StandaloneProps{
			TopBar: topBarProps(request, current, false),
		},
		Workspaces: items,
		RetryURL:   WorkspacesPath(request),
		CreateURL:  WorkspacesPath(request) + "/new",
	}
}
