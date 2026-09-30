package api

import (
	"testing"
	"time"
)

func TestServerStopDrainsBackgroundConflictWork(t *testing.T) {
	ts := startTestServer(t)
	ctx, accepted := ts.server.beginBackgroundTask()
	if !accepted {
		t.Fatal("running server rejected background work")
	}
	release := make(chan struct{})
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		defer ts.server.backgroundJobs.Done()
		<-ctx.Done()
		<-release
	}()
	stopped := make(chan struct{})
	go func() {
		ts.server.Stop()
		close(stopped)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not cancel conflict work")
	}
	if _, accepted := ts.server.beginBackgroundTask(); accepted {
		t.Fatal("stopping server admitted new conflict work")
	}
	select {
	case <-stopped:
		t.Fatal("server returned before the background worker drained")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("background worker did not stop")
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish draining background work")
	}
}
