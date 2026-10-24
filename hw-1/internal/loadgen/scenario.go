package loadgen

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
)

type Op string

const (
	OpList          Op = "list"
	OpCreate        Op = "create"
	OpGet           Op = "get"
	OpUpdate        Op = "update"
	OpGetMissing    Op = "get_missing"
	OpCreateInvalid Op = "create_invalid"
)

const missingIDOffset = 1_000_000

type Request struct {
	Target string
	Op     Op
	Method string
	Path   string
	Body   string
}

type Scenario struct {
	targets []string
	badRate float64

	mu    sync.Mutex
	rnd   *rand.Rand
	next  int
	maxID map[string]int64
}

func NewScenario(targets []string, badRate float64, seed uint64) *Scenario {
	return &Scenario{
		targets: targets,
		badRate: badRate,
		rnd:     rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		maxID:   make(map[string]int64, len(targets)),
	}
}

func (s *Scenario) Next() Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	target := s.targets[s.next%len(s.targets)]
	s.next++
	maxID := s.maxID[target]

	if s.rnd.Float64() < s.badRate {
		if s.rnd.IntN(2) == 0 {
			return s.request(target, OpGetMissing, http.MethodGet, fmt.Sprintf("/items/%d", maxID+missingIDOffset), "")
		}

		return s.request(target, OpCreateInvalid, http.MethodPost, "/items", `{"name":`)
	}
	if maxID == 0 {
		return s.request(target, OpCreate, http.MethodPost, "/items", s.itemBody())
	}

	switch p := s.rnd.Float64(); {
	case p < 0.4:
		return s.request(target, OpGet, http.MethodGet, fmt.Sprintf("/items/%d", 1+s.rnd.Int64N(maxID)), "")
	case p < 0.5:
		return s.request(target, OpList, http.MethodGet, "/items", "")
	case p < 0.75:
		return s.request(target, OpCreate, http.MethodPost, "/items", s.itemBody())
	default:
		return s.request(target, OpUpdate, http.MethodPut, fmt.Sprintf("/items/%d", 1+s.rnd.Int64N(maxID)), s.itemBody())
	}
}

func (s *Scenario) Created(target string, id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id > s.maxID[target] {
		s.maxID[target] = id
	}
}

func (s *Scenario) request(target string, op Op, method, path, body string) Request {
	return Request{Target: target, Op: op, Method: method, Path: path, Body: body}
}

func (s *Scenario) itemBody() string {
	return fmt.Sprintf(`{"name":"item-%d","value":"%d"}`, s.rnd.IntN(1000), s.rnd.IntN(1_000_000))
}
