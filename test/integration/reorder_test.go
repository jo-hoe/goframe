package integration

import "testing"

func TestReorder(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	idA := uploadTestImage(t, client, cfg.ServerURL)
	t.Cleanup(func() { deleteImage(t, client, cfg.ServerURL, idA) })

	idB := uploadTestImage(t, client, cfg.ServerURL)
	t.Cleanup(func() { deleteImage(t, client, cfg.ServerURL, idB) })

	// After two uploads: A is inserted after the current image, B after A.
	// Verify B comes after A in the initial order.
	items := listImages(t, client, cfg.ServerURL)
	posA := orderOf(items, idA)
	posB := orderOf(items, idB)
	if posA < 0 || posB < 0 {
		t.Fatalf("uploaded images not found in list: A=%d B=%d", posA, posB)
	}
	if posA >= posB {
		t.Fatalf("expected A (%s) before B (%s), got A=%d B=%d", idA, idB, posA, posB)
	}
	t.Logf("initial order: A=%d B=%d", posA, posB)

	// Move B up — it should now be before A.
	moveImage(t, client, cfg.ServerURL, idB, "up")

	items = listImages(t, client, cfg.ServerURL)
	posA = orderOf(items, idA)
	posB = orderOf(items, idB)
	t.Logf("after move up: A=%d B=%d", posA, posB)
	if posB >= posA {
		t.Errorf("expected B before A after move up, got A=%d B=%d", posA, posB)
	}
}
