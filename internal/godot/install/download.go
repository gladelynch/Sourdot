package install

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ProgressFunc is called periodically during a download with cumulative
// bytes downloaded and the total size (0 if the server didn't report a
// Content-Length).
type ProgressFunc func(downloaded, total int64)

// Download streams url to a new file at destPath, calling onProgress at a
// throttled interval (~every 100ms, plus a final call) so callers
// (core.VersionManager, emitting core.Event{Type:"download_progress"})
// don't flood the Wails event bridge.
func Download(ctx context.Context, url, destPath string, onProgress ProgressFunc) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d for %s", resp.StatusCode, url)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	pr := &progressReader{r: resp.Body, total: resp.ContentLength, onProgress: onProgress}
	if _, err := io.Copy(out, pr); err != nil {
		return err
	}
	if onProgress != nil {
		onProgress(pr.downloaded, pr.total) // final call, guaranteed 100%
	}
	return nil
}

type progressReader struct {
	r          io.Reader
	total      int64
	downloaded int64
	onProgress ProgressFunc
	lastEmit   time.Time
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	p.downloaded += int64(n)
	if p.onProgress != nil && time.Since(p.lastEmit) > 100*time.Millisecond {
		p.onProgress(p.downloaded, p.total)
		p.lastEmit = time.Now()
	}
	return n, err
}
