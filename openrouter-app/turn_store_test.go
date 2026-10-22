package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTurnStoreAcquireLeaderAndFollower(t *testing.T) {
	store := newTurnStore()
	sessionID := "sess-1"
	turnID := "turn-a"

	first := store.acquire(sessionID, turnID)
	if first.role != turnRoleLeader {
		t.Fatalf("first role = %v, want leader", first.role)
	}
	if first.record.status != turnInProgress {
		t.Fatalf("status = %q, want in_progress", first.record.status)
	}

	second := store.acquire(sessionID, turnID)
	if second.role != turnRoleFollower {
		t.Fatalf("second role = %v, want follower", second.role)
	}

	body := []byte(`{"choices":[{"message":{"content":"visible assistant"}}]}`)
	store.completeTurn(sessionID, turnID, "raw assistant", "visible assistant", body, 200)

	third := store.acquire(sessionID, turnID)
	if third.role != turnRoleReplayCompleted {
		t.Fatalf("third role = %v, want replay_completed", third.role)
	}
}

func TestTurnWaitStateFollowerWaitsForCompletion(t *testing.T) {
	wait := newTurnWaitState()
	done := make(chan bool, 1)

	go func() {
		done <- wait.wait(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)
	body := []byte(`{"choices":[{"message":{"content":"Live"}}]}`)
	wait.markDone("Live", "Live", body, 200, false)

	if ok := <-done; !ok {
		t.Fatal("wait returned false")
	}

	snapshot, statusCode, failed, ok := wait.snapshot()
	if !ok || failed || statusCode != 200 {
		t.Fatalf("unexpected snapshot ok=%v failed=%v status=%d", ok, failed, statusCode)
	}
	if string(snapshot) != string(body) {
		t.Fatalf("unexpected body %q", snapshot)
	}
}

func TestWriteTurnResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	wait := newTurnWaitState()
	body := []byte(`{"choices":[{"message":{"content":"Hello world"}}]}`)
	wait.markDone("Hello world", "Hello world", body, 200, false)

	writeTurnResponse(rec, &chatTurnRecord{wait: wait})

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
}

func TestTurnStoreFailedPartialResume(t *testing.T) {
	store := newTurnStore()
	sessionID := "sess-2"
	turnID := "turn-b"

	store.acquire(sessionID, turnID)
	store.failTurn(sessionID, turnID, "partial raw", "partial visible", nil, http.StatusBadGateway)

	resume := store.acquire(sessionID, turnID)
	if resume.role != turnRoleResumeLeader {
		t.Fatalf("resume role = %v, want resume_leader", resume.role)
	}

	follower := store.acquire(sessionID, turnID)
	if follower.role != turnRoleFollower {
		t.Fatalf("follower role = %v, want follower", follower.role)
	}
}

func TestReplaceAssistantContent(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"content":"before"}}]}`)
	updated, err := replaceAssistantContent(raw, "after")
	if err != nil {
		t.Fatalf("replaceAssistantContent: %v", err)
	}
	var completion openRouterCompletion
	if err := json.Unmarshal(updated, &completion); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if completion.Choices[0].Message.Content != "after" {
		t.Fatalf("content = %q, want after", completion.Choices[0].Message.Content)
	}
}
