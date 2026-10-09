package layouts

import "context"

//
// Render state
//

// The dashboard's main region is the target of in-page navigation inside the
// dashboard; the page region replaces the whole layout.

const (
	PageRegionID    = "page-content"
	ContentRegionID = "dashboard-main"
)

// A request that targets the main region already shows the dashboard, so the
// dashboard renders only its main region and the parts of the sidebar that
// depend on the page. Rendering records whether a dashboard was rendered, so
// a page with another layout can replace the whole page region instead.

type RenderState struct {
	ContentOnly       bool
	DashboardRendered bool
}

type renderStateKey struct{}

func WithRenderState(ctx context.Context, state *RenderState) context.Context {
	return context.WithValue(ctx, renderStateKey{}, state)
}

func renderState(ctx context.Context) *RenderState {
	state, found := ctx.Value(renderStateKey{}).(*RenderState)

	if !found {
		state = &RenderState{}
	}

	return state
}

func contentOnly(ctx context.Context) bool {
	return renderState(ctx).ContentOnly
}

func markDashboardRendered(ctx context.Context) {
	renderState(ctx).DashboardRendered = true
}
