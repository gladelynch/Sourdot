package install

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// VerifyChecksum downloads checksumsURL (a release's published
// SHA512-SUMS.txt) and checks that filePath's SHA-512 digest matches the
// entry for assetName.
//
// A network hiccup fetching the checksums file, or the file simply not
// containing an entry for assetName, is reported as (false, nil) --
// "verification skipped", not a failure -- since Godot's release format
// doesn't guarantee this file's presence or contents. Callers should
// surface that distinction to the user rather than silently treating a
// skip as a pass. An actual digest mismatch returns (false, non-nil error)
// and should abort the install.
func VerifyChecksum(ctx context.Context, checksumsURL, assetName, filePath string) (verified bool, err error) {
	if checksumsURL == "" {
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return false, nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, nil
	}

	want := findChecksumLine(string(body), assetName)
	if want == "" {
		return false, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(got, want) {
		return false, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, want, got)
	}
	return true, nil
}

// findChecksumLine parses SHA512-SUMS.txt's "<hex digest>  <filename>"
// lines (standard sha512sum output format) and returns the digest for
// name, or "" if not present.
func findChecksumLine(body, name string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0]
		}
	}
	return ""
}
