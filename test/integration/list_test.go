package integration

import "testing"

func TestList(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	id := uploadTestImage(t, client, cfg.ServerURL)
	t.Cleanup(func() { deleteImage(t, client, cfg.ServerURL, id) })

	items := listImages(t, client, cfg.ServerURL)
	if !containsID(items, id) {
		t.Errorf("uploaded image %s not found in list (got %d items)", id, len(items))
	}
}
