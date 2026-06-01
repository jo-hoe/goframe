package integration

import "testing"

func TestDelete(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	id := uploadTestImage(t, client, cfg.ServerURL)

	deleteImage(t, client, cfg.ServerURL, id)

	items := listImages(t, client, cfg.ServerURL)
	if containsID(items, id) {
		t.Errorf("deleted image %s still present in list", id)
	}
}
