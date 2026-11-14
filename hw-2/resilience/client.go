package resilience

import "context"

type Client interface {
	Do(ctx context.Context, sender Sender) (Response, error)
}

type MultiClient interface {
	Do(ctx context.Context, senders []Sender) (Response, error)
}
