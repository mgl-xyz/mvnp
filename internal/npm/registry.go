package npm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultRegistry      = "https://registry.npmjs.org"
	defaultUserAgent     = "nvnp/0.1 (+https://github.com/mvnp)"
	defaultRequestGap    = 200 * time.Millisecond
	defaultRateLimitWait = 60 * time.Second
	metadataCacheDir     = ".nvnp/cache/registry"
)

type registryDocument struct {
	DistTags map[string]string   `json:"dist-tags"`
	Versions map[string]struct{} `json:"versions"`
}

// VersionLister fetches available versions for an npm package.
type VersionLister interface {
	ListVersions(name string) ([]string, error)
}

// RegistryClient queries the npm registry.
type RegistryClient struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

func NewRegistryClient(baseURL string) *RegistryClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultRegistry
	}
	return &RegistryClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		UserAgent: defaultUserAgent,
	}
}

func (c *RegistryClient) ListVersions(name string) ([]string, error) {
	packageURL := c.packageURL(name)
	req, err := http.NewRequest(http.MethodGet, packageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrPackageNotFound, name)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry request failed (%d) for %s", resp.StatusCode, name)
	}

	var doc registryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode registry response for %s: %w", name, err)
	}

	versions := make([]string, 0, len(doc.Versions))
	for version := range doc.Versions {
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for %s", name)
	}
	return versions, nil
}

func (c *RegistryClient) userAgent() string {
	if strings.TrimSpace(c.UserAgent) == "" {
		return defaultUserAgent
	}
	return c.UserAgent
}

func (c *RegistryClient) packageURL(name string) string {
	escaped := url.PathEscape(name)
	return c.BaseURL + "/" + escaped
}

type versionCacheEntry struct {
	versions []string
	err      error
}

// CachingRegistry adds memory/disk cache and polite rate limiting.
type CachingRegistry struct {
	inner            *RegistryClient
	mem              map[string]versionCacheEntry
	mu               sync.Mutex
	requestGap       time.Duration
	lastRequest      time.Time
	rateLimitedUntil time.Time
	cacheRoot        string
}

func NewCachingRegistry(baseURL, cacheRoot string) *CachingRegistry {
	if strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = metadataCacheDir
	}
	return &CachingRegistry{
		inner:      NewRegistryClient(baseURL),
		mem:        make(map[string]versionCacheEntry),
		requestGap: defaultRequestGap,
		cacheRoot:  cacheRoot,
	}
}

func (c *CachingRegistry) ListVersions(name string) ([]string, error) {
	key := strings.TrimSpace(name)

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

	versions, err := c.inner.ListVersions(name)
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

func (c *CachingRegistry) storeMemory(key string, versions []string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mem[key] = versionCacheEntry{versions: versions, err: err}
}

func (c *CachingRegistry) waitForSlot() {
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

func (c *CachingRegistry) diskPath(key string) string {
	safe := strings.NewReplacer("@", "_at_", "/", "_", ":", "_").Replace(key)
	return filepath.Join(c.cacheRoot, safe+".json")
}

func (c *CachingRegistry) loadDiskCache(key string) ([]string, error) {
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

func (c *CachingRegistry) saveDiskCache(key string, versions []string) {
	data, err := json.Marshal(versions)
	if err != nil {
		return
	}
	_ = os.MkdirAll(c.cacheRoot, 0o755)
	_ = os.WriteFile(c.diskPath(key), data, 0o644)
}
