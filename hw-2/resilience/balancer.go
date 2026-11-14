package resilience

type Balancer interface {
	Pick(from, replicas int) (int, error)
	Report(replica int, outcome Outcome)
}

var _ Balancer = RoundRobin{}

type RoundRobin struct{}

func (RoundRobin) Pick(from, replicas int) (int, error) {
	if replicas == 0 {
		return 0, ErrNoReplicas
	}

	return from % replicas, nil
}

func (RoundRobin) Report(int, Outcome) {}
