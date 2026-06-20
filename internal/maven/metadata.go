package maven

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultRepository   = "https://repo1.maven.org/maven2"
	defaultUserAgent    = "mvnp/0.1 (+https://github.com/mvnp)"
	defaultRequestGap   = 300 * time.Millisecond
	defaultRateLimitWait = 60 * time.Second
	metadataCacheDir    = ".mvnp/cache/metadata"
)

type metadataDocument struct {
	XMLName    xml.Name `xml:"metadata"`
	Versioning struct {
		Versions struct {
			Version []string `xml:"version"`
		} `xml:"versions"`
	} `xml:"versioning"`
}

// VersionLister fetches available versions for a Maven artifact.
type VersionLister interface {
	ListVersions(groupID, artifactID string) ([]string, error)
}

// RepositoryClient fetches artifact metadata from a Maven repository.
type RepositoryClient struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

func NewRepositoryClient(baseURL string) *RepositoryClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultRepository
	}
	return &RepositoryClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		UserAgent: defaultUserAgent,
	}
}

func (c *RepositoryClient) ListVersions(groupID, artifactID string) ([]string, error) {
	metaURL := c.metadataURL(groupID, artifactID)
	req, err := http.NewRequest(http.MethodGet, metaURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("Accept", "application/xml")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata for %s:%s: %w", groupID, artifactID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s:%s", ErrArtifactNotFound, groupID, artifactID)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata request failed (%d)", resp.StatusCode)
	}

	var doc metadataDocument
	if err := xml.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode metadata for %s:%s: %w", groupID, artifactID, err)
	}

	versions := doc.Versioning.Versions.Version
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for %s:%s", groupID, artifactID)
	}
	return versions, nil
}

func (c *RepositoryClient) userAgent() string {
	if strings.TrimSpace(c.UserAgent) == "" {
		return defaultUserAgent
	}
	return c.UserAgent
}

func (c *RepositoryClient) metadataURL(groupID, artifactID string) string {
	segments := strings.Split(groupID, ".")
	segments = append(segments, artifactID, "maven-metadata.xml")
	return c.BaseURL + "/" + path.Join(segments...)
}

type versionCacheEntry struct {
	versions []string
	err      error
}

// CachingRepository adds memory/disk cache and polite rate limiting for metadata lookups.
type CachingRepository struct {
	inner          *RepositoryClient
	mem            map[string]versionCacheEntry
	mu             sync.Mutex
	requestGap     time.Duration
	lastRequest    time.Time
	rateLimitedUntil time.Time
	cacheRoot      string
}

func NewCachingRepository(baseURL, cacheRoot string) *CachingRepository {
	if strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = metadataCacheDir
	}
	return &CachingRepository{
		inner:      NewRepositoryClient(baseURL),
		mem:        make(map[string]versionCacheEntry),
		requestGap: defaultRequestGap,
		cacheRoot:  cacheRoot,
	}
}

func (c *CachingRepository) ListVersions(groupID, artifactID string) ([]string, error) {
	key := ArtifactCoordinate(groupID, artifactID)

	c.mu.Lock()
	if entry, ok := c.mem[key]; ok {
		c.mu.Unlock()
		return entry.versions, entry.err
	}
	c.mu.Unlock()

	if versions, err := c.loadDiskCache(key); err == nil && len(versions) > 0 {
		c.storeMemory(key, versions, nil)
		return versions, nil
	}

	c.waitForSlot()

	c.mu.Lock()
	if time.Now().Before(c.rateLimitedUntil) {
		err := ErrRateLimited
		c.mem[key] = versionCacheEntry{err: err}
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	versions, err := c.inner.ListVersions(groupID, artifactID)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			c.mu.Lock()
			c.rateLimitedUntil = time.Now().Add(defaultRateLimitWait)
			c.mu.Unlock()
		}
		c.storeMemory(key, nil, err)
		return nil, err
	}

	c.saveDiskCache(key, versions)
	c.storeMemory(key, versions, nil)
	return versions, nil
}

func (c *CachingRepository) storeMemory(key string, versions []string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mem[key] = versionCacheEntry{versions: versions, err: err}
}

func (c *CachingRepository) waitForSlot() {
	c.mu.Lock()
	if c.lastRequest.IsZero() {
		c.lastRequest = time.Now()
		c.mu.Unlock()
		return
	}
	if wait := c.requestGap - time.Since(c.lastRequest); wait > 0 {
		c.mu.Unlock()
		time.Sleep(wait)
		c.mu.Lock()
	}
	c.lastRequest = time.Now()
	c.mu.Unlock()
}

func (c *CachingRepository) diskPath(key string) string {
	safe := strings.NewReplacer(":", "_", "/", "_").Replace(key)
	return filepath.Join(c.cacheRoot, safe+".json")
}

func (c *CachingRepository) loadDiskCache(key string) ([]string, error) {
	data, err := os.ReadFile(c.diskPath(key))
	if err != nil {
		return nil, err
	}
	var versions []string
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func (c *CachingRepository) saveDiskCache(key string, versions []string) {
	data, err := json.Marshal(versions)
	if err != nil {
		return
	}
	_ = os.MkdirAll(c.cacheRoot, 0o755)
	_ = os.WriteFile(c.diskPath(key), data, 0o644)
}

// RepositoryURLFromSettings is a placeholder for future settings.xml support.
func RepositoryURLFromSettings(settingsPath string) string {
	_ = settingsPath
	return defaultRepository
}

func ArtifactCoordinate(groupID, artifactID string) string {
	return fmt.Sprintf("%s:%s", groupID, artifactID)
}

func ParseArtifactCoordinate(raw string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid coordinate %q, expected groupId:artifactId", raw)
	}
	return parts[0], parts[1], nil
}

func EncodePathSegment(segment string) string {
	return url.PathEscape(segment)
}
