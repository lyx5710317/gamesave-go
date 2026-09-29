package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// A second-device report: one device wrote test.txt, the other received it,
// then the writer added test2.txt before both devices had been left idle for
// another sync interval. Both files came from the same device, so the second
// arrival must not be classified as independent edits on both sides. Unlike
// the ordinary one-file test, this deliberately does not wait for a watcher
// debounce or a settle window between the two writes.
func TestRapidAdditionsFromOneDeviceDoNotConflict(t *testing.T) {
	writer, receiver, gameID := pairAndTrack(t, "RapidAdditions", nil)

	// Establish a verifiable empty common state before the first write. An
	// unpaired or never-synced game is a different conflict case.
	syncTo(writer, gameID, receiver.NodeID())
	syncTo(receiver, gameID, writer.NodeID())
	if !testutil.WaitFor(10*time.Second, func() bool {
		return writer.Daemon.Store.GetAgreedHash(gameID, receiver.NodeID()) != "" &&
			receiver.Daemon.Store.GetAgreedHash(gameID, writer.NodeID()) != ""
	}) {
		t.Fatal("empty initial state never became an agreed merge-base")
	}

	writer.WriteSave("test.txt", "first")
	if status, _ := syncTo(receiver, gameID, writer.NodeID()); status == "conflict" || status == "error" {
		t.Fatalf("receiving the first file returned %q", status)
	}
	if !testutil.WaitFor(30*time.Second, func() bool {
		return receiver.ReadSave("test.txt") == "first"
	}) {
		t.Fatal("the first file never reached the receiver")
	}

	// Immediately write again, while the first transfer's background events
	// may still be settling. Only the writer changes the save in this test.
	writer.WriteSave("test2.txt", "")
	if status, _ := syncTo(receiver, gameID, writer.NodeID()); status == "conflict" || status == "error" {
		c, _ := conflictOn(receiver, gameID)
		t.Fatalf("a second one-sided file addition returned %q: %+v; receiver base=%q writer base=%q",
			status, c.DiffFiles,
			receiver.Daemon.Store.GetAgreedHash(gameID, writer.NodeID()),
			writer.Daemon.Store.GetAgreedHash(gameID, receiver.NodeID()))
	}
	if !testutil.WaitFor(30*time.Second, func() bool {
		info, err := os.Stat(filepath.Join(receiver.SaveDir, "test2.txt"))
		return err == nil && info.Size() == 0
	}) {
		t.Fatal("the empty second file never reached the receiver")
	}
	if c, ok := conflictOn(receiver, gameID); ok {
		t.Fatalf("receiver retained a conflict after two one-sided additions: %+v", c.DiffFiles)
	}
	if c, ok := conflictOn(writer, gameID); ok {
		t.Fatalf("writer retained a conflict after two one-sided additions: %+v", c.DiffFiles)
	}
}
