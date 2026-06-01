package integration

import "testing"

func TestUpload(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	id := uploadTestImage(t, client, cfg.ServerURL)
	t.Logf("uploaded image id: %s", id)

	t.Cleanup(func() { deleteImage(t, client, cfg.ServerURL, id) })
}
