package main

import (
	"fmt"
	"time"
)

type Manager struct {
	current int
	workers []*Worker
}

func NewManager(proxies []string) (*Manager, error) {
	workers := make([]*Worker, len(proxies))

	for i, proxy := range proxies {
		client, err := NewCWithProxy(fmt.Sprintf("sock5://%s", proxy))
		if err != nil {
			return nil, err
		}

		client.SetRateLimiter(time.Second, 2)

		w, err := NewWorker(client)
		if err != nil {
			return nil, err
		}

		workers[i] = w
	}

	return &Manager{
		workers: workers,
	}, nil
}

func (s *Manager) GetPhone(id string) (string, error) {
	s.current = s.current % len(s.workers)
	fmt.Printf("worker index: %d", s.current)
	phone, err := s.workers[s.current].GetPhone(id)

	s.current++

	return phone, err
}
