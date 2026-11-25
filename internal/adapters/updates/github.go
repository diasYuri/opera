// Package updates downloads and verifies Opera release artifacts from GitHub.
package updates

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/diasYuri/opera/internal/modules/versions"
)

const Repository = "diasYuri/opera"

const (
	maxReleaseResponse = 4 << 20
	maxArchiveBytes    = 128 << 20
	maxExpandedBytes   = 512 << 20
	maxBinaryBytes     = 200 << 20
)

type Release struct {
	Tag     string `json:"tag"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

type Client struct{ http *http.Client }

func NewClient() *Client {
	return &Client{http: &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			host := strings.ToLower(request.URL.Hostname())
			githubHost := host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")
			if len(via) >= 5 || request.URL.Scheme != "https" || !githubHost {
				return errors.New("release download used an unsafe redirect")
			}
			return nil
		},
	}}
}

func (c *Client) Latest(ctx context.Context) (Release, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+Repository+"/releases?per_page=1", nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "opera-version-check")
	response, err := c.http.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("query Opera releases: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("query Opera releases: GitHub returned HTTP %d", response.StatusCode)
	}
	releaseData, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseResponse+1))
	if err != nil {
		return Release{}, fmt.Errorf("read Opera releases: %w", err)
	}
	if int64(len(releaseData)) > maxReleaseResponse {
		return Release{}, errors.New("release list exceeded its size limit")
	}
	var releases []struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
	}
	if err := json.Unmarshal(releaseData, &releases); err != nil {
		return Release{}, fmt.Errorf("decode Opera releases: %w", err)
	}
	if len(releases) == 0 || releases[0].Draft || !strings.HasPrefix(releases[0].TagName, "v") || !versions.Validate(releases[0].TagName) {
		return Release{}, errors.New("no published semantic-version release is available")
	}
	item := releases[0]
	return Release{Tag: item.TagName, Version: strings.TrimPrefix(item.TagName, "v"), URL: item.HTMLURL}, nil
}

// Binary downloads the current platform archive and verifies its published
// SHA-256 checksum plus the package's embedded platform/version metadata.
func (c *Client) Binary(ctx context.Context, release Release) ([]byte, error) {
	if !versions.Validate(release.Version) || release.Tag != "v"+release.Version {
		return nil, errors.New("release tag is invalid")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("self-update is supported on macOS and Linux, not %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return nil, fmt.Errorf("self-update is unsupported on architecture %s", runtime.GOARCH)
	}
	base := "https://github.com/" + Repository + "/releases/download/" + release.Tag
	name := fmt.Sprintf("opera_%s_%s_%s.tar.gz", release.Version, runtime.GOOS, runtime.GOARCH)
	archive, err := c.get(ctx, base+"/"+name, maxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download Opera %s: %w", release.Version, err)
	}
	sums, err := c.get(ctx, base+"/SHA256SUMS", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("download release checksums: %w", err)
	}
	expected, err := checksumFor(sums, name)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(archive)
	if !strings.EqualFold(expected, hex.EncodeToString(digest[:])) {
		return nil, errors.New("downloaded Opera archive failed SHA-256 verification")
	}
	binary, metadata, err := extractPackage(archive, "opera_"+release.Version+"_"+runtime.GOOS+"_"+runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	if metadata.Version != release.Version || metadata.GOOS != runtime.GOOS || metadata.GOARCH != runtime.GOARCH {
		return nil, errors.New("release archive metadata does not match the requested release and platform")
	}
	return binary, nil
}

func (c *Client) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "opera-version-check")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("release file exceeded its size limit")
	}
	return data, nil
}

func checksumFor(data []byte, name string) (string, error) {
	var digest string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if digest != "" || len(fields[0]) != sha256.Size*2 {
			return "", errors.New("release checksums contain an invalid or duplicate archive entry")
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return "", errors.New("release checksums contain an invalid archive digest")
		}
		digest = fields[0]
	}
	if digest == "" {
		return "", errors.New("release checksums do not include the platform archive")
	}
	return digest, nil
}

type packageMetadata struct {
	Version string
	GOOS    string
	GOARCH  string
}

func extractPackage(data []byte, root string) ([]byte, packageMetadata, error) {
	compressed := bytes.NewReader(data)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, packageMetadata{}, fmt.Errorf("open Opera archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	var binary []byte
	var metadata packageMetadata
	var total int64
	seen := make(map[string]bool)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, packageMetadata{}, fmt.Errorf("read Opera archive: %w", err)
		}
		name := path.Clean(header.Name)
		if !fsValidTarPath(header.Name) || !strings.HasPrefix(name, root+"/") || seen[name] {
			return nil, packageMetadata{}, errors.New("release archive contains an unexpected or duplicate path")
		}
		seen[name] = true
		if header.Typeflag != tar.TypeReg {
			return nil, packageMetadata{}, errors.New("release archive contains a non-regular file")
		}
		if header.Size < 0 || header.Size > maxExpandedBytes || total > maxExpandedBytes-header.Size {
			return nil, packageMetadata{}, errors.New("release archive exceeds its expanded size limit")
		}
		total += header.Size
		switch name {
		case root + "/opera":
			if header.Size > maxBinaryBytes {
				return nil, packageMetadata{}, errors.New("release executable exceeds its size limit")
			}
			binary, err = io.ReadAll(io.LimitReader(reader, maxBinaryBytes+1))
			if err != nil || int64(len(binary)) != header.Size {
				return nil, packageMetadata{}, errors.New("release executable in the archive is incomplete")
			}
		case root + "/version.json":
			if header.Size > 64<<10 {
				return nil, packageMetadata{}, errors.New("release version metadata exceeds its size limit")
			}
			contents, readErr := io.ReadAll(io.LimitReader(reader, (64<<10)+1))
			if readErr != nil || int64(len(contents)) != header.Size {
				return nil, packageMetadata{}, errors.New("release version metadata is incomplete")
			}
			if err := json.Unmarshal(contents, &metadata); err != nil {
				return nil, packageMetadata{}, fmt.Errorf("decode Opera version metadata: %w", err)
			}
		default:
			if _, err := io.Copy(io.Discard, reader); err != nil {
				return nil, packageMetadata{}, err
			}
		}
	}
	if len(binary) == 0 || metadata.Version == "" {
		return nil, packageMetadata{}, errors.New("release archive is missing its executable or version metadata")
	}
	return binary, metadata, nil
}

func fsValidTarPath(name string) bool {
	return name != "" && !path.IsAbs(name) && path.Clean(name) == name && name != "." && !strings.Contains(name, "\\")
}
