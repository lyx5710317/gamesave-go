package pairing

import "testing"

func TestConfirmConsumesLocallyInitiatedPairing(t *testing.T) {
	m := New()
	m.RecordSent("lan:127.0.0.1:3001")
	if !m.ValidateConfirm("peer", "127.0.0.1", 3001) {
		t.Fatal("matching LAN confirmation was rejected")
	}
	if m.ValidateConfirm("peer", "127.0.0.1", 3001) {
		t.Fatal("the LAN confirmation was accepted a second time")
	}
	m.RecordIncoming(IncomingRequest{PeerID: "pending"})
	if m.ValidateConfirm("pending", "127.0.0.1", 3001) {
		t.Fatal("a pending request must still be approved locally")
	}
}

func TestRelayConfirmationMatchesRequestedPeer(t *testing.T) {
	m := New()
	m.RecordSent("relay:peer-a")
	if m.ValidateConfirm("peer-b", "relay", 3001) {
		t.Fatal("confirmation from a different peer must not consume this request")
	}
	if !m.ValidateConfirm("peer-a", "relay", 3001) {
		t.Fatal("confirmation from the requested relay peer was rejected")
	}
}
