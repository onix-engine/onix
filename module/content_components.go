package module

import (
	"github.com/diiyw/onix/ecs"
	html "github.com/diiyw/onix/ui/html"
)

// ContentRenderContext is passed by value; Props and State refer to the component.
type ContentRenderContext[P, S any] struct {
	World     *Content
	Component ecs.Entity
	Props     *P
	State     *S
}
type ContentComponentContext[P, S any] = ContentRenderContext[P, S]
type ContentEventContext[P, S any] struct {
	World     *Content
	Component ecs.Entity
	Ref       string
	Event     html.Event
	Pointer   *html.PointerEvent
	Props     *P
	State     *S
	event     *html.ComponentEventContext[P, S]
}

func (c *ContentEventContext[P, S]) UpdateState(update func(*S)) bool {
	return c.event.UpdateState(update)
}
func (c *ContentEventContext[P, S]) PreventDefault()  { c.event.PreventDefault() }
func (c *ContentEventContext[P, S]) StopPropagation() { c.event.StopPropagation() }

type ContentBehavior[P, S any] struct {
	Render      func(ContentRenderContext[P, S], *html.View)
	Unmount     func(*ContentComponentContext[P, S])
	HandleEvent func(*ContentEventContext[P, S])
}

func ContentDefine[P, S any](c *Content, template html.Template, css string, b ContentBehavior[P, S]) (*html.Definition[P, S], error) {
	return html.Define(template, css, html.Behavior[P, S]{
		Render: func(ctx html.RenderContext[P, S], v *html.View) {
			if b.Render != nil {
				b.Render(ContentRenderContext[P, S]{c, ctx.Component, ctx.Props, ctx.State}, v)
			}
		},
		Unmount: func(ctx *html.ComponentContext[P, S]) {
			if b.Unmount != nil {
				b.Unmount(&ContentComponentContext[P, S]{c, ctx.Component, ctx.Props, ctx.State})
			}
		},
		HandleEvent: func(ctx *html.ComponentEventContext[P, S]) {
			if b.HandleEvent != nil {
				b.HandleEvent(&ContentEventContext[P, S]{World: c, Component: ctx.Component, Ref: ctx.Ref, Event: ctx.Event, Pointer: ctx.Pointer, Props: ctx.Props, State: ctx.State, event: ctx})
			}
		},
	})
}
