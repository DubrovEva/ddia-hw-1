package resilience

import "context"

var _ Sender = clientSender{}

type clientSender struct {
	client Client
	sender Sender
}

func WithClient(client Client, sender Sender) Sender {
	return clientSender{client: client, sender: sender}
}

func (s clientSender) Send(ctx context.Context) (Response, error) {
	return s.client.Do(ctx, s.sender)
}

func PerReplica(client Client, replicas []Sender) []Sender {
	wrapped := make([]Sender, len(replicas))
	for i, replica := range replicas {
		wrapped[i] = WithClient(client, replica)
	}

	return wrapped
}
