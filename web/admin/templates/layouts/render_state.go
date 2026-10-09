package layouts

import "context"

//
// Render state
//

// The panel's content region is the target of in-page navigation inside the
// dashboard; the page region replaces the whole layout.

const (
	PageRegionID    = "page-content"
	ContentRegionID = "panel-main"
)

// A request that targets the content region already shows the dashboard, so
// the panel renders only its content and the parts of the sidebar that depend
// on the page. Rendering records whether a panel was rendered, so a page with
// another layout can replace the whole page region instead.

type RenderState struct {
	ContentOnly   bool
	PanelRendered bool
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

func markPanelRendered(ctx context.Context) {
	renderState(ctx).PanelRendered = true
}
