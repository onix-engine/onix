package module

// These helpers compose a single OnMessage callback for resource-focused tests.
func handleValue[T any](c *Context, topic string, handler func(*Context, T) error) error {
	previous := c.message
	c.message = func(m Message) error {
		if m.Topic == topic && !m.IsReply() {
			v, ok := Value[T](m)
			if !ok {
				return ErrType
			}
			return handler(c, v)
		}
		if previous != nil {
			return previous(m)
		}
		return nil
	}
	return nil
}
func handleRequest[Q, R any](c *Context, topic string, handler func(*Context, Request[Q, R]) error) error {
	previous := c.message
	c.message = func(m Message) error {
		if m.Topic == topic && m.IsRequest() {
			v, ok := RequestValue[Q, R](m)
			if !ok {
				return ErrType
			}
			return handler(c, v)
		}
		if previous != nil {
			return previous(m)
		}
		return nil
	}
	return nil
}
func testRequester[Q, R any](c *Context, topic string, handler func(*Context, Result[R]) error) (Requester[Q, R], error) {
	if handler != nil {
		previous := c.message
		c.message = func(m Message) error {
			if m.Topic == topic && m.IsReply() {
				v, ok := ResultValue[R](m)
				if !ok {
					return ErrType
				}
				return handler(c, v)
			}
			if previous != nil {
				return previous(m)
			}
			return nil
		}
	}
	return BindRequester[Q, R](c, topic)
}
