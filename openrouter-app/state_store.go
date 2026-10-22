package main

import (
	"sync"
	"time"
)

type agentStateStore struct {
	mu    sync.RWMutex
	items map[string]*AgentSessionState
}

func newAgentStateStore() *agentStateStore {
	store := &agentStateStore{items: make(map[string]*AgentSessionState)}
	go store.cleanupLoop()
	return store
}

func (s *agentStateStore) get(sessionID string) (*AgentSessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.items[sessionID]
	return state, ok
}

func (s *agentStateStore) getOrCreate(sessionID string) *AgentSessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, ok := s.items[sessionID]; ok {
		return state
	}
	state := newAgentSessionState()
	s.items[sessionID] = state
	return state
}

func (s *agentStateStore) set(sessionID string, state *AgentSessionState) {
	state.UpdatedAt = time.Now()
	s.mu.Lock()
	s.items[sessionID] = state
	s.mu.Unlock()
}

func (s *agentStateStore) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-25 * time.Hour)
		s.mu.Lock()
		for id, state := range s.items {
			if state.UpdatedAt.Before(cutoff) {
				delete(s.items, id)
			}
		}
		s.mu.Unlock()
	}
}
